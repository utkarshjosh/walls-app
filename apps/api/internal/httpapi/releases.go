package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/utkarshjoshi/walls/apps/api/internal/records"
)

// Builds of the app: what the website offers for download (the first half of
// this file) and how the owner puts them there (the second half).
//
// The files are kept on this server's own disk, in ReleasesDir, whatever the
// catalog uses for storage. They are few and small, and a download should not
// depend on anything but this server.

const (
	// maxBuildBytes is the largest build accepted. Note that Cloudflare's free
	// plan refuses uploads over 100 MB before they reach this server.
	maxBuildBytes = 400 << 20

	downloadsCountedPerIPPerHour = 30
	maxReleaseNotes              = 2000

	// A build's address has its id in it, so its content never changes. A
	// day, rather than for ever, so a build that had to be withdrawn stops
	// being handed out by a cache in front soon after.
	cacheBuild = "public, max-age=86400, no-transform"
)

// platform is one kind of computer a build is made for.
type platform struct {
	ID    string
	Label string
	OS    string // "mac", "windows" or "linux": what /v1/download/<os> picks from
}

// platforms is every platform a build can be uploaded for, each OS's
// preferred build first. Only the Mac ones exist today; the rest are here so
// that adding a Windows or Linux build later is an upload, not a change here.
var platforms = []platform{
	{"mac-universal", "macOS, Apple silicon and Intel", "mac"},
	{"mac-arm64", "macOS, Apple silicon", "mac"},
	{"mac-x64", "macOS, Intel", "mac"},
	{"windows-x64", "Windows, 64-bit", "windows"},
	{"windows-arm64", "Windows on ARM", "windows"},
	{"linux-x64", "Linux, 64-bit", "linux"},
	{"linux-arm64", "Linux on ARM", "linux"},
}

func platformLabel(id string) string {
	for _, p := range platforms {
		if p.ID == id {
			return p.Label
		}
	}
	return ""
}

// buildTypes is the kinds of file a build may be, by the end of its name.
var buildTypes = map[string]string{
	".dmg":      "application/x-apple-diskimage",
	".pkg":      "application/octet-stream",
	".zip":      "application/zip",
	".exe":      "application/vnd.microsoft.portable-executable",
	".msi":      "application/x-msi",
	".appimage": "application/octet-stream",
	".deb":      "application/vnd.debian.binary-package",
	".rpm":      "application/x-rpm",
	".gz":       "application/gzip", // .tar.gz
}

var (
	versionPattern  = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z]+(\.[0-9A-Za-z]+)*)?$`)
	filenamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,119}$`)
)

// ReleasesEnabled reports whether builds can be kept and offered here.
func (s *Server) ReleasesEnabled() bool { return s.Records != nil && s.ReleasesDir != "" }

func (s *Server) buildPath(b records.Build) string {
	return filepath.Join(s.ReleasesDir, strconv.FormatInt(b.ID, 10)+"-"+b.Filename)
}

func (s *Server) buildFileURL(b records.Build) string {
	return s.PublicURL + "/v1/releases/files/" + strconv.FormatInt(b.ID, 10) + "/" + b.Filename
}

// --- what the website reads ---------------------------------------------------

type publicBuildJSON struct {
	Platform string `json:"platform"`
	Label    string `json:"label"`
	Filename string `json:"filename"`
	Bytes    int64  `json:"bytes"`
	SHA256   string `json:"sha256"`
	URL      string `json:"url"` // counts the download, then sends the browser to the file
}

func noRelease(w http.ResponseWriter) {
	writeError(w, http.StatusNotFound, "no_release", "No build has been published yet.")
}

// latest returns the release on offer, answering 404 itself when there is none.
func (s *Server) latest(w http.ResponseWriter, r *http.Request) (records.Release, bool) {
	if !s.ReleasesEnabled() {
		noRelease(w)
		return records.Release{}, false
	}
	rel, err := s.Records.LatestRelease(r.Context())
	if errors.Is(err, records.ErrNotFound) {
		noRelease(w)
		return rel, false
	}
	if err != nil {
		s.fail(w, r, err)
		return rel, false
	}
	return rel, true
}

// handleLatestRelease describes the newest published release and its builds.
func (s *Server) handleLatestRelease(w http.ResponseWriter, r *http.Request) {
	rel, ok := s.latest(w, r)
	if !ok {
		return
	}
	builds := make([]publicBuildJSON, len(rel.Builds))
	// In the order of the platforms list, so a page that just shows them all
	// shows each system's preferred build first.
	i := 0
	for _, p := range platforms {
		for _, b := range rel.Builds {
			if b.Platform == p.ID {
				builds[i] = publicBuildJSON{Platform: b.Platform, Label: p.Label, Filename: b.Filename, Bytes: b.Bytes,
					SHA256: b.SHA256, URL: s.PublicURL + "/v1/download/" + b.Platform}
				i++
			}
		}
	}
	w.Header().Set("Cache-Control", cacheLists)
	writeJSON(w, http.StatusOK, map[string]any{
		"version": rel.Version, "channel": rel.Channel, "notes": rel.Notes, "publishedAt": iso(rel.PublishedAt), "builds": builds[:i],
	})
}

// handleDownload counts a download and sends the browser to the file. The
// name is a platform ("mac-arm64"), or just a system ("mac") for whichever
// build of the latest release suits it best.
func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	rel, ok := s.latest(w, r)
	if !ok {
		return
	}
	want := r.PathValue("platform")
	var found *records.Build
	for _, p := range platforms {
		if p.ID != want && p.OS != want {
			continue
		}
		for i := range rel.Builds {
			if rel.Builds[i].Platform == p.ID {
				found = &rel.Builds[i]
				break
			}
		}
		if found != nil {
			break
		}
	}
	if found == nil {
		writeError(w, http.StatusNotFound, "no_build", "There is no build for that system yet.")
		return
	}
	// One person fetching the same build over and over is one download, more or less.
	if r.Method == http.MethodGet && s.downloadByIP.allow(clientKey(r), s.now()) {
		s.count(r, records.CountDownload, rel.Version+"/"+found.Platform)
	}
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, s.buildFileURL(*found), http.StatusFound)
}

// handleBuildFile serves one build's file. Only builds of a published release
// can be fetched.
func (s *Server) handleBuildFile(w http.ResponseWriter, r *http.Request) {
	missing := func() { writeError(w, http.StatusNotFound, "not_found", "There is no such download.") }
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 || !s.ReleasesEnabled() {
		missing()
		return
	}
	b, rel, err := s.Records.Build(r.Context(), id)
	if errors.Is(err, records.ErrNotFound) || (err == nil && (!rel.Published() || b.Filename != r.PathValue("name"))) {
		missing()
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	f, err := os.Open(s.buildPath(b))
	if err != nil {
		s.Log.Error("build file missing from disk", "build", b.ID, "err", err.Error())
		missing()
		return
	}
	defer f.Close()
	// A large build on a slow link needs longer than the server-wide timeout.
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(30 * time.Minute))

	h := w.Header()
	h.Set("Content-Type", buildTypes[strings.ToLower(filepath.Ext(b.Filename))])
	h.Set("Content-Disposition", `attachment; filename="`+b.Filename+`"`)
	h.Set("Cache-Control", cacheBuild)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("ETag", `"`+b.SHA256[:16]+`"`)
	http.ServeContent(w, r, "", b.CreatedAt, f)
}

// --- what the owner does ------------------------------------------------------

type adminBuildJSON struct {
	ID        int64  `json:"id"`
	Platform  string `json:"platform"`
	Label     string `json:"label"`
	Filename  string `json:"filename"`
	Bytes     int64  `json:"bytes"`
	SHA256    string `json:"sha256"`
	URL       string `json:"url"`
	CreatedAt string `json:"createdAt"`
	Month     int64  `json:"downloads30"`
	AllTime   int64  `json:"downloadsAll"`
}

type adminReleaseJSON struct {
	ID          int64            `json:"id"`
	Version     string           `json:"version"`
	Channel     string           `json:"channel"`
	Notes       string           `json:"notes"`
	CreatedAt   string           `json:"createdAt"`
	PublishedAt *string          `json:"publishedAt"` // null while it is a draft
	Latest      bool             `json:"latest"`      // the one the website offers
	Builds      []adminBuildJSON `json:"builds"`
}

func (s *Server) adminBuild(b records.Build, version string, month, all map[string]int64) adminBuildJSON {
	key := version + "/" + b.Platform
	return adminBuildJSON{ID: b.ID, Platform: b.Platform, Label: platformLabel(b.Platform), Filename: b.Filename, Bytes: b.Bytes,
		SHA256: b.SHA256, URL: s.buildFileURL(b), CreatedAt: iso(b.CreatedAt), Month: month[key], AllTime: all[key]}
}

func releasesOff(w http.ResponseWriter) {
	writeError(w, http.StatusServiceUnavailable, "releases_off", "This server has nowhere to keep builds.")
}

// releaseID reads the {id} of a release or a build from the path.
func releaseID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeError(w, http.StatusNotFound, "not_found", "There is nothing with that id.")
		return 0, false
	}
	return id, true
}

func (s *Server) handleAdminReleases(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	list, err := s.Records.Releases(ctx)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	month, err := s.subjectCounts(r, records.CountDownload, records.Day(s.now().AddDate(0, 0, -(activityDays-1))))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	all, err := s.subjectCounts(r, records.CountDownload, "")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var latestID int64
	if rel, err := s.Records.LatestRelease(ctx); err == nil {
		latestID = rel.ID
	} else if !errors.Is(err, records.ErrNotFound) {
		s.fail(w, r, err)
		return
	}
	out := make([]adminReleaseJSON, len(list))
	for i, rel := range list {
		j := adminReleaseJSON{ID: rel.ID, Version: rel.Version, Channel: rel.Channel, Notes: rel.Notes,
			CreatedAt: iso(rel.CreatedAt), Latest: rel.ID == latestID, Builds: make([]adminBuildJSON, len(rel.Builds))}
		if rel.Published() {
			at := iso(rel.PublishedAt)
			j.PublishedAt = &at
		}
		for k, b := range rel.Builds {
			j.Builds[k] = s.adminBuild(b, rel.Version, month, all)
		}
		out[i] = j
	}
	type option struct {
		ID    string `json:"id"`
		Label string `json:"label"`
	}
	options := make([]option, len(platforms))
	for i, p := range platforms {
		options[i] = option{p.ID, p.Label}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"releases": out, "platforms": options, "enabled": s.ReleasesEnabled(), "maxBuildBytes": maxBuildBytes,
	})
}

func checkChannel(w http.ResponseWriter, channel string) bool {
	if channel != records.ChannelBeta && channel != records.ChannelStable {
		writeError(w, http.StatusBadRequest, "invalid_channel", `channel must be "beta" or "stable".`)
		return false
	}
	return true
}

func checkNotes(w http.ResponseWriter, notes string) bool {
	if len(notes) > maxReleaseNotes {
		writeError(w, http.StatusBadRequest, "invalid_notes", fmt.Sprintf("Notes can be at most %d characters.", maxReleaseNotes))
		return false
	}
	return true
}

func (s *Server) handleAdminCreateRelease(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Version string `json:"version"`
		Channel string `json:"channel"`
		Notes   string `json:"notes"`
	}
	if !readJSON(w, r, &body, 16<<10) {
		return
	}
	version := strings.TrimPrefix(strings.TrimSpace(body.Version), "v")
	if len(version) > 40 || !versionPattern.MatchString(version) {
		writeError(w, http.StatusBadRequest, "invalid_version", `A version looks like "0.1.0" or "0.2.0-beta.1".`)
		return
	}
	if body.Channel == "" {
		body.Channel = records.ChannelBeta
	}
	notes := strings.TrimSpace(body.Notes)
	if !checkChannel(w, body.Channel) || !checkNotes(w, notes) {
		return
	}
	rel, err := s.Records.CreateRelease(r.Context(), version, body.Channel, notes, s.now())
	if errors.Is(err, records.ErrExists) {
		writeError(w, http.StatusConflict, "version_taken", "There is already a release with that version.")
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r, "Started a release", rel.Version)
	writeJSON(w, http.StatusCreated, map[string]any{"id": rel.ID, "version": rel.Version})
}

// handleAdminUpdateRelease changes a release's notes or channel, publishes it
// or takes it back. Publishing makes it the one the website offers.
func (s *Server) handleAdminUpdateRelease(w http.ResponseWriter, r *http.Request) {
	id, ok := releaseID(w, r)
	if !ok {
		return
	}
	var body struct {
		Channel   *string `json:"channel"`
		Notes     *string `json:"notes"`
		Published *bool   `json:"published"`
	}
	if !readJSON(w, r, &body, 16<<10) {
		return
	}
	ctx := r.Context()
	rel, err := s.Records.Release(ctx, id)
	if errors.Is(err, records.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "There is no release with that id.")
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if body.Channel != nil || body.Notes != nil {
		channel, notes := rel.Channel, rel.Notes
		if body.Channel != nil {
			channel = *body.Channel
		}
		if body.Notes != nil {
			notes = strings.TrimSpace(*body.Notes)
		}
		if !checkChannel(w, channel) || !checkNotes(w, notes) {
			return
		}
		if err := s.Records.UpdateRelease(ctx, id, channel, notes); err != nil {
			s.fail(w, r, err)
			return
		}
	}
	if body.Published != nil && *body.Published != rel.Published() {
		if *body.Published && len(rel.Builds) == 0 {
			writeError(w, http.StatusConflict, "no_builds", "Add at least one build before publishing.")
			return
		}
		if err := s.Records.SetReleasePublished(ctx, id, *body.Published, s.now()); err != nil {
			s.fail(w, r, err)
			return
		}
		if *body.Published {
			s.audit(r, "Published a release", rel.Version)
		} else {
			s.audit(r, "Took a release back", rel.Version)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// removeBuildFile deletes a build's file. A file that will not go is logged
// and left: the build is already gone from the list.
func (s *Server) removeBuildFile(b records.Build) {
	if err := os.Remove(s.buildPath(b)); err != nil && !errors.Is(err, os.ErrNotExist) {
		s.Log.Warn("build file not removed", "build", b.ID, "err", err.Error())
	}
}

func (s *Server) handleAdminDeleteRelease(w http.ResponseWriter, r *http.Request) {
	id, ok := releaseID(w, r)
	if !ok {
		return
	}
	rel, err := s.Records.Release(r.Context(), id)
	if err == nil {
		var builds []records.Build
		if builds, err = s.Records.DeleteRelease(r.Context(), id); err == nil {
			for _, b := range builds {
				s.removeBuildFile(b)
			}
		}
	}
	if errors.Is(err, records.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "There is no release with that id.")
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r, "Removed a release", rel.Version)
	w.WriteHeader(http.StatusNoContent)
}

// handleAdminUploadBuild stores the request body as the build of a release
// for one platform (?platform=) under a file name (?filename=). Uploading
// again for the same platform replaces the earlier file.
func (s *Server) handleAdminUploadBuild(w http.ResponseWriter, r *http.Request) {
	if !s.ReleasesEnabled() {
		releasesOff(w)
		return
	}
	id, ok := releaseID(w, r)
	if !ok {
		return
	}
	plat, name := r.URL.Query().Get("platform"), r.URL.Query().Get("filename")
	if platformLabel(plat) == "" {
		writeError(w, http.StatusBadRequest, "invalid_platform", "Choose which system the build is for.")
		return
	}
	if !filenamePattern.MatchString(name) || buildTypes[strings.ToLower(filepath.Ext(name))] == "" {
		writeError(w, http.StatusBadRequest, "invalid_filename",
			"The file name may use letters, digits, dots, dashes and underscores, and must end in .dmg, .pkg, .zip, .exe, .msi, .AppImage, .deb, .rpm or .tar.gz.")
		return
	}
	ctx := r.Context()
	rel, err := s.Records.Release(ctx, id)
	if errors.Is(err, records.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "There is no release with that id.")
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}

	// An upload takes longer than the server-wide limits allow for a request.
	rc := http.NewResponseController(w)
	_ = rc.SetReadDeadline(time.Now().Add(30 * time.Minute))
	_ = rc.SetWriteDeadline(time.Now().Add(31 * time.Minute))

	if err := os.MkdirAll(s.ReleasesDir, 0o755); err != nil {
		s.fail(w, r, err)
		return
	}
	// Written beside its final place, so moving it there cannot half-happen.
	tmp, err := os.CreateTemp(s.ReleasesDir, "upload-*.part")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	defer os.Remove(tmp.Name()) // a no-op once it has been moved into place
	defer tmp.Close()

	sum := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmp, sum), http.MaxBytesReader(w, r.Body, maxBuildBytes))
	var tooBig *http.MaxBytesError
	switch {
	case errors.As(err, &tooBig):
		writeError(w, http.StatusRequestEntityTooLarge, "too_large", fmt.Sprintf("A build can be at most %d MB.", maxBuildBytes>>20))
		return
	case err != nil:
		writeError(w, http.StatusBadRequest, "invalid_body", "The upload was cut short. Try again.")
		return
	case n == 0:
		writeError(w, http.StatusBadRequest, "invalid_body", "The upload was empty.")
		return
	}
	if err := tmp.Sync(); err != nil {
		s.fail(w, r, err)
		return
	}
	if err := tmp.Close(); err != nil {
		s.fail(w, r, err)
		return
	}

	b, replaced, err := s.Records.PutBuild(ctx, rel.ID, plat, name, n, hex.EncodeToString(sum.Sum(nil)), s.now())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := os.Rename(tmp.Name(), s.buildPath(b)); err != nil {
		_, _ = s.Records.DeleteBuild(ctx, b.ID)
		s.fail(w, r, err)
		return
	}
	_ = os.Chmod(s.buildPath(b), 0o644)
	if replaced != nil {
		s.removeBuildFile(*replaced)
	}
	s.audit(r, "Uploaded a build", rel.Version+" for "+platformLabel(plat))
	writeJSON(w, http.StatusCreated, s.adminBuild(b, rel.Version, nil, nil))
}

func (s *Server) handleAdminDeleteBuild(w http.ResponseWriter, r *http.Request) {
	id, ok := releaseID(w, r)
	if !ok {
		return
	}
	b, err := s.Records.DeleteBuild(r.Context(), id)
	if errors.Is(err, records.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "There is no build with that id.")
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.removeBuildFile(b)
	s.audit(r, "Removed a build", platformLabel(b.Platform))
	w.WriteHeader(http.StatusNoContent)
}

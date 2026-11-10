package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/utkarshjoshi/walls/apps/api/internal/catalog"
	"github.com/utkarshjoshi/walls/apps/api/internal/imaging"
)

// FileResult is one row of the import report.
type FileResult struct {
	File   string
	Status string // added | updated | unchanged | skipped | failed
	ID     string
	Detail string
	More   []string // further lines of detail (every reason a scene failed its check)
}

// Report is the outcome of importing a folder.
type Report struct {
	Results  []FileResult
	Imported int // files now in the catalog (added, updated or unchanged)
}

// fileMeta is the JSON shape of sidecars and manifest entries. Pointers tell
// "not mentioned" apart from "set to empty".
type fileMeta struct {
	Title     *string   `json:"title"`
	Category  *string   `json:"category"`
	Tags      *[]string `json:"tags"`
	Author    *string   `json:"author"`
	License   *string   `json:"license"`
	SourceURL *string   `json:"sourceUrl"`
}

type manifest struct {
	Defaults fileMeta            `json:"defaults"`
	Items    map[string]fileMeta `json:"items"`
}

// over returns m with every field that o mentions replaced by o's value.
func (m fileMeta) over(o fileMeta) fileMeta {
	if o.Title != nil {
		m.Title = o.Title
	}
	if o.Category != nil {
		m.Category = o.Category
	}
	if o.Tags != nil {
		m.Tags = o.Tags
	}
	if o.Author != nil {
		m.Author = o.Author
	}
	if o.License != nil {
		m.License = o.License
	}
	if o.SourceURL != nil {
		m.SourceURL = o.SourceURL
	}
	return m
}

func readJSON(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("%s is not valid JSON: %v", filepath.Base(path), err)
	}
	return nil
}

var posterExts = []string{".jpg", ".jpeg", ".png", ".webp"}

// sniffFile identifies a file from its first bytes.
func sniffFile(path string) (imaging.Format, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return imaging.Format{}, false, err
	}
	defer f.Close()
	head := make([]byte, 64)
	n, err := io.ReadFull(f, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return imaging.Format{}, false, err
	}
	format, ok := imaging.Sniff(head[:n])
	return format, ok, nil
}

// IngestDir imports every image and video directly inside dir, and every live
// scene: a sub-folder holding a scene.html (or dir itself, if it is one).
// Other sub-folders are not visited. Scenes must pass the scene gate; a
// scene that fails is skipped with its reasons. Problems with one file never
// stop the others. The error return is only for a folder that cannot be read
// at all.
func (p *Pipeline) IngestDir(ctx context.Context, dir string) (Report, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return Report{}, fmt.Errorf("cannot read %s: %w", dir, err)
	}
	if IsSceneDir(dir) { // the folder is itself one scene
		res := p.ingestScene(ctx, dir, filepath.Base(filepath.Clean(dir))+"/")
		rep := Report{Results: []FileResult{res}}
		if res.Status != "skipped" && res.Status != "failed" {
			rep.Imported = 1
		}
		return rep, nil
	}

	var man manifest
	manifestErr := readJSON(filepath.Join(dir, "manifest.json"), &man)
	if errors.Is(manifestErr, fs.ErrNotExist) {
		manifestErr = nil
	}
	if manifestErr != nil {
		// A broken manifest would silently drop licences; refuse instead.
		return Report{}, manifestErr
	}

	// First pass: what is each file really?
	type candidate struct {
		name   string
		format imaging.Format
		known  bool
		err    error
	}
	var files []candidate
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || strings.HasPrefix(name, ".") || strings.EqualFold(filepath.Ext(name), ".json") {
			continue
		}
		if info, err := e.Info(); err != nil || !info.Mode().IsRegular() {
			continue
		}
		format, ok, err := sniffFile(filepath.Join(dir, name))
		files = append(files, candidate{name: name, format: format, known: ok, err: err})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].name < files[j].name })

	// Images that share a base name with a video are that video's poster,
	// not wallpapers of their own.
	base := func(name string) string { return strings.TrimSuffix(name, filepath.Ext(name)) }
	posterFor := map[string]string{} // video file name → poster file name
	isPoster := map[string]bool{}
	for _, v := range files {
		if !v.known || v.format.Kind != "video" {
			continue
		}
		for _, c := range files {
			if c.known && c.format.Kind == "image" && base(c.name) == base(v.name) {
				posterFor[v.name] = c.name
				isPoster[c.name] = true
				break
			}
		}
	}

	var rep Report
	for _, f := range files {
		if isPoster[f.name] {
			continue
		}
		res := FileResult{File: f.name}
		id, status, detail, err := p.ingestFile(ctx, dir, f.name, f.format, f.known, f.err, posterFor[f.name], man)
		res.ID, res.Status, res.Detail = id, status, detail
		if err != nil {
			res.Status, res.Detail = "failed", err.Error()
			var skip skipError
			if errors.As(err, &skip) || errors.Is(err, imaging.ErrUnsupported) || errors.Is(err, imaging.ErrTooBig) ||
				errors.Is(err, imaging.ErrTooManyPixels) || errors.Is(err, imaging.ErrCorrupt) {
				res.Status = "skipped"
			}
		} else {
			rep.Imported++
		}
		rep.Results = append(rep.Results, res)
	}

	// Live scenes: one per sub-folder that holds a scene.html.
	for _, e := range entries {
		sub := filepath.Join(dir, e.Name())
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") || !IsSceneDir(sub) {
			continue
		}
		res := p.ingestScene(ctx, sub, e.Name()+"/")
		if res.Status != "skipped" && res.Status != "failed" {
			rep.Imported++
		}
		rep.Results = append(rep.Results, res)
	}
	return rep, nil
}

// skipError marks a file refused for a reason the owner can fix (as opposed
// to an I/O or storage failure).
type skipError struct{ reason string }

func (e skipError) Error() string { return e.reason }

func skipf(format string, args ...any) error { return skipError{fmt.Sprintf(format, args...)} }

func (p *Pipeline) ingestFile(ctx context.Context, dir, name string, format imaging.Format, known bool, sniffErr error,
	posterName string, man manifest) (id, status, detail string, err error) {
	if sniffErr != nil {
		return "", "", "", sniffErr
	}
	if !known {
		return "", "", "", skipf("unsupported file type (accepted: JPEG, PNG, WebP images; MP4, MOV, WebM videos)")
	}
	path := filepath.Join(dir, name)
	stem := strings.TrimSuffix(name, filepath.Ext(name))

	// Metadata: folder defaults, then the manifest entry, then the sidecar.
	fm := man.Defaults.over(man.Items[name])
	var side fileMeta
	switch err := readJSON(filepath.Join(dir, stem+".json"), &side); {
	case err == nil:
		fm = fm.over(side)
	case !errors.Is(err, fs.ErrNotExist):
		return "", "", "", skipf("%v", err)
	}
	meta, err := resolveMeta(fm, stem, format.Kind)
	if err != nil {
		return "", "", "", err
	}

	info, err := os.Stat(path)
	if err != nil {
		return "", "", "", err
	}
	asset := Asset{Ext: format.Ext, ContentType: format.ContentType, Meta: meta, Origin: "ingest"}

	if format.Kind == "image" {
		if info.Size() > imaging.MaxImageBytes {
			return "", "", "", fmt.Errorf("%w: %.1f MiB, limit is 25 MiB", imaging.ErrTooBig, float64(info.Size())/(1<<20))
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return "", "", "", err
		}
		img, _, err := imaging.DecodeImage(data)
		if err != nil {
			return "", "", "", err
		}
		asset.Kind, asset.Data, asset.Image = catalog.KindImage, data, img
	} else {
		if info.Size() > imaging.MaxVideoBytes {
			return "", "", "", fmt.Errorf("%w: %.1f MiB, limit is 200 MiB", imaging.ErrTooBig, float64(info.Size())/(1<<20))
		}
		if posterName == "" {
			return "", "", "", skipf("video needs a poster image beside it with the same name (for example %s.jpg)", stem)
		}
		posterData, err := os.ReadFile(filepath.Join(dir, posterName))
		if err != nil {
			return "", "", "", err
		}
		img, pf, err := imaging.DecodeImage(posterData)
		if err != nil {
			return "", "", "", fmt.Errorf("poster %s: %w", posterName, err)
		}
		asset.Kind, asset.Path, asset.Image = catalog.KindVideo, path, img
		// A JPEG poster that is already upright is stored as-is; anything
		// else is re-encoded.
		if pf == imaging.FormatJPEG && imaging.Upright(posterData) {
			asset.PosterJPEG = posterData
		}
	}

	res, err := p.Publish(ctx, asset)
	if err != nil {
		if errors.Is(err, ErrOverCap) {
			return "", "", "", skipf("%v", err)
		}
		return "", "", "", err
	}
	b := res.Item
	return res.ID, res.Status, fmt.Sprintf("%s · %s · %d×%d · %s", b.Title, b.Kind, b.Width, b.Height, HumanBytes(b.Bytes)), nil
}

// resolveMeta checks and normalises the merged metadata for one file.
func resolveMeta(fm fileMeta, stem, kind string) (Meta, error) {
	get := func(s *string) string {
		if s == nil {
			return ""
		}
		return strings.TrimSpace(*s)
	}
	m := Meta{
		Title: get(fm.Title), Author: get(fm.Author), License: get(fm.License), SourceURL: get(fm.SourceURL),
	}
	// Attribution is never guessed.
	var missing []string
	if m.License == "" {
		missing = append(missing, "license")
	}
	if m.Author == "" {
		missing = append(missing, "author")
	}
	if len(missing) > 0 {
		return m, skipf("missing %s (add it to %s.json or manifest.json)", strings.Join(missing, " and "), stem)
	}
	if m.Title == "" {
		m.Title = catalog.Humanize(stem)
	}
	if m.Title == "" {
		return m, skipf("could not work out a title; set one in %s.json", stem)
	}

	m.Category = catalog.Slugify(get(fm.Category))
	if m.Category == "" {
		m.Category = "other"
		if kind == "video" {
			m.Category = "live"
		}
	}

	m.Tags = []string{}
	if fm.Tags != nil {
		seen := map[string]bool{}
		for _, t := range *fm.Tags {
			t = strings.ToLower(strings.TrimSpace(t))
			if t != "" && !seen[t] {
				seen[t] = true
				m.Tags = append(m.Tags, t)
			}
		}
	}

	if m.SourceURL != "" {
		u, err := url.Parse(m.SourceURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return m, skipf("sourceUrl must be an http(s) link")
		}
	}
	return m, nil
}

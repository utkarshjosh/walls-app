package httpapi

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/csv"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/utkarshjoshi/walls/apps/api/internal/catalog"
	"github.com/utkarshjoshi/walls/apps/api/internal/records"
)

// The admin portal's API, under /v1/admin. One person uses it: the owner,
// with one password set on the server (WALLS_ADMIN_PASSWORD). There are no
// user accounts. With no password set, every address here answers 404.

const (
	sessionCookie = "walls_admin"
	sessionTTL    = 14 * 24 * time.Hour

	// Wrong passwords allowed before sign-in is refused for a while: per
	// connection, and in total. The total is what holds if someone gets
	// around the per-connection count; it is high enough that the owner
	// mistyping never reaches it, and low enough that guessing a twelve
	// character password stays hopeless.
	loginWindow          = 15 * time.Minute
	loginFailuresPerIP   = 8
	loginFailuresOverall = 300

	activityDays  = 30
	maxPeopleSent = 5000
	maxNoteLength = 500
)

// AdminEnabled reports whether the admin portal is switched on.
func (s *Server) AdminEnabled() bool { return s.Records != nil && s.AdminPassword != "" }

// sessionHash is what is stored for a signed-in browser. The password is
// mixed in, so changing the password signs every browser out.
func (s *Server) sessionHash(token string) string {
	sum := sha256.Sum256([]byte("walls admin session\n" + s.AdminPassword + "\n" + token))
	return hex.EncodeToString(sum[:])
}

func isLocalHost(hostport string) bool {
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	return host == "localhost" || strings.HasSuffix(host, ".localhost") || host == "127.0.0.1" || host == "::1"
}

func (s *Server) setSessionCookie(w http.ResponseWriter, r *http.Request, token string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/v1/admin", // sent to the admin API and nowhere else
		MaxAge:   maxAge,
		HttpOnly: true, // script on the page cannot read it
		// Never sent on a request that another site started.
		SameSite: http.SameSiteStrictMode,
		// Only over https, except on this machine during development, where
		// there is no https and some browsers refuse a Secure cookie.
		Secure: !isLocalHost(r.Host),
	})
}

func (s *Server) signedIn(r *http.Request) bool {
	c, err := r.Cookie(sessionCookie)
	if err != nil || c.Value == "" || len(c.Value) > 128 {
		return false
	}
	ok, err := s.Records.SessionValid(r.Context(), s.sessionHash(c.Value), s.now())
	return err == nil && ok
}

// sameSite reports whether a request that changes something was started by
// the admin page itself. A browser says where a request came from in Origin
// and Sec-Fetch-Site; a request from another site is refused even if the
// cookie somehow came with it.
func sameSite(r *http.Request) bool {
	if v := r.Header.Get("Sec-Fetch-Site"); v != "" && v != "same-origin" && v != "none" {
		return false
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		u, err := url.Parse(origin)
		if err != nil || u.Host == "" || !strings.EqualFold(u.Host, r.Host) {
			return false
		}
	}
	return true
}

func adminOff(w http.ResponseWriter) {
	writeError(w, http.StatusNotFound, "admin_off", "The admin portal is switched off on this server.")
}

// guard wraps an admin handler: the portal must be on, a change must come
// from the admin page, and the browser must be signed in.
func (s *Server) guard(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if !s.AdminEnabled() {
			adminOff(w)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead && !sameSite(r) {
			writeError(w, http.StatusForbidden, "cross_site", "This request did not come from the admin page.")
			return
		}
		if !s.signedIn(r) {
			writeError(w, http.StatusUnauthorized, "signed_out", "Sign in to continue.")
			return
		}
		next(w, r)
	}
}

// audit writes a line to the admin log. It never fails the request.
func (s *Server) audit(r *http.Request, action, detail string) {
	if err := s.Records.Log(r.Context(), s.now(), action, detail); err != nil {
		s.Log.Warn("admin log not written", "action", action, "err", err.Error())
	}
}

func people(n int) string {
	if n == 1 {
		return "1 person"
	}
	return strconv.Itoa(n) + " people"
}

// --- signing in ---------------------------------------------------------------

func (s *Server) handleAdminSession(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	enabled := s.AdminEnabled()
	writeJSON(w, http.StatusOK, map[string]bool{"enabled": enabled, "signedIn": enabled && s.signedIn(r)})
}

func (s *Server) handleAdminLogin(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !s.AdminEnabled() {
		adminOff(w)
		return
	}
	if !sameSite(r) {
		writeError(w, http.StatusForbidden, "cross_site", "This request did not come from the admin page.")
		return
	}
	var body struct {
		Password string `json:"password"`
	}
	if !readJSON(w, r, &body, 4<<10) {
		return
	}
	now, key := s.now(), clientKey(r)
	if s.loginByIP.full(key, now) || s.loginAll.full("all", now) {
		w.Header().Set("Retry-After", strconv.Itoa(int(loginWindow.Seconds())))
		writeError(w, http.StatusTooManyRequests, "too_many", "Too many wrong passwords. Wait fifteen minutes and try again.")
		return
	}
	// Compare fixed-length digests in constant time, so how long the answer
	// takes says nothing about how much of a guess was right.
	given, want := sha256.Sum256([]byte(body.Password)), sha256.Sum256([]byte(s.AdminPassword))
	if subtle.ConstantTimeCompare(given[:], want[:]) != 1 {
		s.loginByIP.add(key, now)
		s.loginAll.add("all", now)
		s.Log.Warn("admin sign-in refused: wrong password")
		writeError(w, http.StatusUnauthorized, "wrong_password", "That is not the password.")
		return
	}

	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		s.fail(w, r, err)
		return
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	if err := s.Records.CreateSession(r.Context(), s.sessionHash(token), now, now.Add(sessionTTL)); err != nil {
		s.fail(w, r, err)
		return
	}
	s.setSessionCookie(w, r, token, int(sessionTTL.Seconds()))
	s.audit(r, "Signed in", "")
	writeJSON(w, http.StatusOK, map[string]bool{"signedIn": true})
}

func (s *Server) handleAdminLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		if err := s.Records.DeleteSession(r.Context(), s.sessionHash(c.Value)); err != nil {
			s.fail(w, r, err)
			return
		}
	}
	s.setSessionCookie(w, r, "", -1)
	w.WriteHeader(http.StatusNoContent)
}

// --- activity -----------------------------------------------------------------

type dayJSON struct {
	Day      string `json:"day"`
	Visits   int64  `json:"visits"`
	Signups  int64  `json:"signups"`
	Discover int64  `json:"discover"`
	Files    int64  `json:"files"`
}

type topJSON struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Kind  string `json:"kind"`
	Thumb string `json:"thumb"`
	Count int64  `json:"count"`
}

type eventJSON struct {
	At     string `json:"at"`
	Action string `json:"action"`
	Detail string `json:"detail"`
}

func iso(t time.Time) string { return t.UTC().Format("2006-01-02T15:04:05Z") }

func (s *Server) handleAdminOverview(w http.ResponseWriter, r *http.Request) {
	ctx, now := r.Context(), s.now()
	from := records.Day(now.AddDate(0, 0, -(activityDays - 1)))

	series := map[string]map[string]int64{}
	for _, kind := range []string{records.CountVisit, records.CountDiscover, records.CountFile} {
		m, err := s.Records.CountsByDay(ctx, kind, from)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		series[kind] = m
	}
	signups, err := s.Records.SignupsByDay(ctx, from)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	days := make([]dayJSON, 0, activityDays)
	for i := activityDays - 1; i >= 0; i-- {
		d := records.Day(now.AddDate(0, 0, -i))
		days = append(days, dayJSON{Day: d, Visits: series[records.CountVisit][d], Signups: signups[d],
			Discover: series[records.CountDiscover][d], Files: series[records.CountFile][d]})
	}

	totals, err := s.Records.SignupTotals(ctx)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	counts, err := s.Records.CountsBySubject(ctx, records.CountFile, from)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	top := []topJSON{}
	for _, c := range counts {
		if len(top) == 8 {
			break
		}
		it, err := s.DB.GetItem(ctx, c.Subject)
		if errors.Is(err, catalog.ErrNotFound) {
			continue // no longer in the catalog
		}
		if err != nil {
			s.fail(w, r, err)
			return
		}
		top = append(top, topJSON{ID: it.ID, Title: it.Title, Kind: it.Kind, Thumb: s.fileURL(it.ID, "thumb"), Count: c.N})
	}
	events, err := s.Records.RecentEvents(ctx, 20)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	log := make([]eventJSON, len(events))
	for i, e := range events {
		log[i] = eventJSON{At: iso(e.At), Action: e.Action, Detail: e.Detail}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"days":          days,
		"signups":       map[string]int{"total": totals.Total, "invited": totals.Invited},
		"topWallpapers": top,
		"events":        log,
	})
}

// --- the list of people -------------------------------------------------------

type personJSON struct {
	ID        int64   `json:"id"`
	Email     string  `json:"email"`
	Source    string  `json:"source"`
	Country   string  `json:"country"`
	Status    string  `json:"status"`
	Note      string  `json:"note"`
	CreatedAt string  `json:"createdAt"`
	InvitedAt *string `json:"invitedAt"`
}

func person(p records.Signup) personJSON {
	out := personJSON{ID: p.ID, Email: p.Email, Source: p.Source, Country: p.Country, Status: p.Status,
		Note: p.Note, CreatedAt: iso(p.CreatedAt)}
	if !p.InvitedAt.IsZero() {
		at := iso(p.InvitedAt)
		out.InvitedAt = &at
	}
	return out
}

// statusFilter reads ?status=, answering 400 itself when it is not one of the two.
func statusFilter(w http.ResponseWriter, r *http.Request) (string, bool) {
	switch v := r.URL.Query().Get("status"); v {
	case "", records.StatusNew, records.StatusInvited:
		return v, true
	}
	writeError(w, http.StatusBadRequest, "invalid_status", `status must be "new" or "invited".`)
	return "", false
}

func (s *Server) handleAdminPeople(w http.ResponseWriter, r *http.Request) {
	status, ok := statusFilter(w, r)
	if !ok {
		return
	}
	search := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(search) > 200 {
		writeError(w, http.StatusBadRequest, "invalid_query", "The search text is too long.")
		return
	}
	list, err := s.Records.ListSignups(r.Context(), records.SignupQuery{Search: search, Status: status, Limit: maxPeopleSent + 1})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	more := len(list) > maxPeopleSent
	if more {
		list = list[:maxPeopleSent]
	}
	totals, err := s.Records.SignupTotals(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := make([]personJSON, len(list))
	for i, p := range list {
		out[i] = person(p)
	}
	writeJSON(w, http.StatusOK, map[string]any{"people": out, "more": more, "total": totals.Total, "invited": totals.Invited})
}

func (s *Server) handleAdminAddPerson(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email string `json:"email"`
		Note  string `json:"note"`
	}
	if !readJSON(w, r, &body, 8<<10) {
		return
	}
	email, ok := records.NormalizeEmail(body.Email)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_email", "That does not look like an email address.")
		return
	}
	note := strings.TrimSpace(body.Note)
	if len(note) > maxNoteLength {
		writeError(w, http.StatusBadRequest, "invalid_note", fmt.Sprintf("A note can be at most %d characters.", maxNoteLength))
		return
	}
	added, err := s.Records.AddSignup(r.Context(), email, records.SourceManual, "", note, s.now())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if !added {
		writeError(w, http.StatusConflict, "already_listed", "That address is already on the list.")
		return
	}
	s.audit(r, "Added a person", "Typed in by hand")
	writeJSON(w, http.StatusCreated, map[string]bool{"added": true})
}

func (s *Server) handleAdminMark(w http.ResponseWriter, r *http.Request) {
	var body struct {
		IDs    []int64 `json:"ids"`
		Status string  `json:"status"`
	}
	if !readJSON(w, r, &body, 256<<10) {
		return
	}
	if body.Status != records.StatusNew && body.Status != records.StatusInvited {
		writeError(w, http.StatusBadRequest, "invalid_status", `status must be "new" or "invited".`)
		return
	}
	if len(body.IDs) == 0 || len(body.IDs) > maxPeopleSent {
		writeError(w, http.StatusBadRequest, "invalid_ids", fmt.Sprintf("Send between 1 and %d ids.", maxPeopleSent))
		return
	}
	n, err := s.Records.SetStatus(r.Context(), body.IDs, body.Status, s.now())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if n > 0 {
		if body.Status == records.StatusInvited {
			s.audit(r, "Marked as sent", people(n))
		} else {
			s.audit(r, "Marked as not sent", people(n))
		}
	}
	writeJSON(w, http.StatusOK, map[string]int{"changed": n})
}

func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeError(w, http.StatusNotFound, "not_found", "Nobody on the list has that id.")
		return 0, false
	}
	return id, true
}

func (s *Server) handleAdminNote(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body struct {
		Note *string `json:"note"`
	}
	if !readJSON(w, r, &body, 8<<10) {
		return
	}
	if body.Note == nil {
		writeError(w, http.StatusBadRequest, "invalid_note", "Send the note to store.")
		return
	}
	note := strings.TrimSpace(*body.Note)
	if len(note) > maxNoteLength {
		writeError(w, http.StatusBadRequest, "invalid_note", fmt.Sprintf("A note can be at most %d characters.", maxNoteLength))
		return
	}
	err := s.Records.SetNote(r.Context(), id, note)
	if errors.Is(err, records.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "Nobody on the list has that id.")
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"note": note})
}

func (s *Server) handleAdminRemovePerson(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	err := s.Records.DeleteSignup(r.Context(), id)
	if errors.Is(err, records.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "Nobody on the list has that id.")
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r, "Removed a person", "Their address is gone from the list")
	w.WriteHeader(http.StatusNoContent)
}

// csvCell stops a spreadsheet from running a cell as a formula. An address or
// a note that starts with one of these characters is written as text.
func csvCell(v string) string {
	if v != "" && strings.ContainsRune("=+-@\t\r", rune(v[0])) {
		return "'" + v
	}
	return v
}

// WriteSignupsCSV writes the list as a spreadsheet file, oldest first.
func WriteSignupsCSV(w *csv.Writer, list []records.Signup) error {
	if err := w.Write([]string{"email", "signed_up", "source", "country", "status", "sent", "note"}); err != nil {
		return err
	}
	for i := len(list) - 1; i >= 0; i-- {
		p := list[i]
		sent := ""
		if !p.InvitedAt.IsZero() {
			sent = iso(p.InvitedAt)
		}
		if err := w.Write([]string{csvCell(p.Email), iso(p.CreatedAt), p.Source, p.Country, p.Status, sent, csvCell(p.Note)}); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}

func (s *Server) handleAdminPeopleCSV(w http.ResponseWriter, r *http.Request) {
	status, ok := statusFilter(w, r)
	if !ok {
		return
	}
	list, err := s.Records.ListSignups(r.Context(), records.SignupQuery{Status: status})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r, "Downloaded the list", people(len(list)))
	h := w.Header()
	h.Set("Content-Type", "text/csv; charset=utf-8")
	h.Set("Content-Disposition", `attachment; filename="signups-`+records.Day(s.now())+`.csv"`)
	h.Set("X-Content-Type-Options", "nosniff")
	if err := WriteSignupsCSV(csv.NewWriter(w), list); err != nil {
		s.Log.Error("csv not fully written", "err", err.Error())
	}
}

// --- wallpapers ---------------------------------------------------------------

type adminItemJSON struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Kind      string `json:"kind"`
	Category  string `json:"category"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	Bytes     int64  `json:"storedBytes"`
	Author    string `json:"author"`
	License   string `json:"license"`
	Origin    string `json:"origin"`
	CreatedAt string `json:"createdAt"`
	Thumb     string `json:"thumb"`
	Week      int64  `json:"fetches7"`
	Month     int64  `json:"fetches30"`
	AllTime   int64  `json:"fetchesAll"`
}

type adminPackJSON struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Count int    `json:"count"`
	Month int64  `json:"opens30"`
}

func (s *Server) subjectCounts(r *http.Request, kind, fromDay string) (map[string]int64, error) {
	list, err := s.Records.CountsBySubject(r.Context(), kind, fromDay)
	if err != nil {
		return nil, err
	}
	out := make(map[string]int64, len(list))
	for _, c := range list {
		out[c.Subject] = c.N
	}
	return out, nil
}

// handleAdminWallpapers lists everything in the catalog with how often each
// full file was fetched. It only reads: the catalog is changed with the
// `walls` commands and a deploy, which replaces the server's copy.
func (s *Server) handleAdminWallpapers(w http.ResponseWriter, r *http.Request) {
	ctx, now := r.Context(), s.now()
	week := records.Day(now.AddDate(0, 0, -6))
	month := records.Day(now.AddDate(0, 0, -(activityDays - 1)))

	var counts [3]map[string]int64
	for i, from := range []string{week, month, ""} {
		m, err := s.subjectCounts(r, records.CountFile, from)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		counts[i] = m
	}
	packOpens, err := s.subjectCounts(r, records.CountPack, month)
	if err != nil {
		s.fail(w, r, err)
		return
	}

	items := []adminItemJSON{}
	var stored int64
	for cursor := ""; ; {
		page, err := s.DB.ListItems(ctx, catalog.Query{Cursor: cursor, Limit: 500})
		if err != nil {
			s.fail(w, r, err)
			return
		}
		for _, it := range page.Items {
			stored += it.StoredBytes
			items = append(items, adminItemJSON{
				ID: it.ID, Title: it.Title, Kind: it.Kind, Category: it.Category, Width: it.Width, Height: it.Height,
				Bytes: it.StoredBytes, Author: it.Author, License: it.License, Origin: it.Origin, CreatedAt: iso(it.CreatedAt),
				Thumb: s.fileURL(it.ID, "thumb"), Week: counts[0][it.ID], Month: counts[1][it.ID], AllTime: counts[2][it.ID],
			})
		}
		if cursor = page.NextCursor; cursor == "" {
			break
		}
	}
	list, err := s.DB.Packs(ctx)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	packs := make([]adminPackJSON, len(list))
	for i, p := range list {
		packs[i] = adminPackJSON{ID: p.ID, Name: p.Name, Count: p.Count, Month: packOpens[p.ID]}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": items, "packs": packs, "storedBytes": stored, "capBytes": s.StorageCap,
	})
}

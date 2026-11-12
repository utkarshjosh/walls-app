package httpapi

import (
	"encoding/csv"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/utkarshjoshi/walls/apps/api/internal/catalog"
	"github.com/utkarshjoshi/walls/apps/api/internal/records"
)

// The admin portal's view of the people who have signed in to the app: who
// they are, what they did, and the three things the owner can do to one of
// them (keep a note, block, remove).

const (
	maxUsersSent      = 5000
	userEventsShown   = 200
	userNoteMaxLength = maxNoteLength
)

type userJSON struct {
	ID         int64  `json:"id"`
	Email      string `json:"email"`
	Country    string `json:"country"`
	Status     string `json:"status"`
	Note       string `json:"note"`
	CreatedAt  string `json:"createdAt"`
	LastSeenAt string `json:"lastSeenAt"`
	Events     int64  `json:"events"`
}

func userRow(u records.UserRow) userJSON {
	return userJSON{ID: u.ID, Email: u.Email, Country: u.Country, Status: u.Status, Note: u.Note,
		CreatedAt: iso(u.CreatedAt), LastSeenAt: iso(u.LastSeenAt), Events: u.Events}
}

// userStatusFilter reads ?status=, answering 400 itself when it is not one of the two.
func userStatusFilter(w http.ResponseWriter, r *http.Request) (string, bool) {
	switch v := r.URL.Query().Get("status"); v {
	case "", records.UserActive, records.UserBlocked:
		return v, true
	}
	writeError(w, http.StatusBadRequest, "invalid_status", `status must be "active" or "blocked".`)
	return "", false
}

func (s *Server) handleAdminUsers(w http.ResponseWriter, r *http.Request) {
	status, ok := userStatusFilter(w, r)
	if !ok {
		return
	}
	search := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(search) > 200 {
		writeError(w, http.StatusBadRequest, "invalid_query", "The search text is too long.")
		return
	}
	ctx := r.Context()
	list, err := s.Records.ListUsers(ctx, records.UserQuery{Search: search, Status: status, Limit: maxUsersSent + 1})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	more := len(list) > maxUsersSent
	if more {
		list = list[:maxUsersSent]
	}
	totals, err := s.Records.UserTotals(ctx)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := make([]userJSON, len(list))
	for i, u := range list {
		out[i] = userRow(u)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"users": out, "more": more, "total": totals.Total, "blocked": totals.Blocked, "signIn": s.SignInEnabled(),
	})
}

type userEventJSON struct {
	At      string `json:"at"`
	Kind    string `json:"kind"`
	Subject string `json:"subject"`
	Detail  string `json:"detail"`
	Title   string `json:"title"` // the wallpaper's or collection's name, when the subject is one
	Thumb   string `json:"thumb"` // a small picture of the wallpaper, when the subject is one
}

// handleAdminUser shows one person and the most recent things they did.
func (s *Server) handleAdminUser(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	u, err := s.Records.UserByID(ctx, id)
	if errors.Is(err, records.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "No user has that id.")
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	counts, err := s.Records.UserEventCounts(ctx, id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	events, err := s.Records.UserEvents(ctx, id, userEventsShown)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var total int64
	for _, n := range counts {
		total += n
	}

	// Names for what the events point at, each looked up once.
	packs := map[string]string{}
	if list, err := s.DB.Packs(ctx); err == nil {
		for _, p := range list {
			packs[p.ID] = p.Name
		}
	}
	type named struct{ title, thumb string }
	items := map[string]named{}
	out := make([]userEventJSON, len(events))
	for i, e := range events {
		j := userEventJSON{At: iso(e.At), Kind: e.Kind, Subject: e.Subject, Detail: e.Detail}
		switch e.Kind {
		case records.EventPackOpen, records.EventPackSave:
			j.Title = packs[e.Subject]
		case records.EventView, records.EventSave, records.EventSet, records.EventRemove:
			n, seen := items[e.Subject]
			if !seen && idPattern.MatchString(e.Subject) {
				if it, err := s.DB.GetItem(ctx, e.Subject); err == nil {
					n = named{it.Title, s.fileURL(it.ID, "thumb")}
				} else if !errors.Is(err, catalog.ErrNotFound) {
					s.fail(w, r, err)
					return
				}
				items[e.Subject] = n
			}
			j.Title, j.Thumb = n.title, n.thumb
		}
		out[i] = j
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user":   userRow(records.UserRow{User: u, Events: total}),
		"counts": counts,
		"events": out,
		"more":   total > int64(len(events)),
	})
}

// handleAdminUpdateUser keeps a note on a person, or blocks or unblocks them.
func (s *Server) handleAdminUpdateUser(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body struct {
		Note   *string `json:"note"`
		Status *string `json:"status"`
	}
	if !readJSON(w, r, &body, 8<<10) {
		return
	}
	if body.Note == nil && body.Status == nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Send a note or a status to store.")
		return
	}
	var note string
	if body.Note != nil {
		if note = strings.TrimSpace(*body.Note); len(note) > userNoteMaxLength {
			writeError(w, http.StatusBadRequest, "invalid_note", fmt.Sprintf("A note can be at most %d characters.", userNoteMaxLength))
			return
		}
	}
	if body.Status != nil && *body.Status != records.UserActive && *body.Status != records.UserBlocked {
		writeError(w, http.StatusBadRequest, "invalid_status", `status must be "active" or "blocked".`)
		return
	}
	ctx := r.Context()
	before, err := s.Records.UserByID(ctx, id)
	if err == nil && body.Note != nil {
		err = s.Records.SetUserNote(ctx, id, note)
	}
	if err == nil && body.Status != nil && *body.Status != before.Status {
		if err = s.Records.SetUserStatus(ctx, id, *body.Status); err == nil {
			if *body.Status == records.UserBlocked {
				s.audit(r, "Blocked a user", "They are signed out and cannot sign in")
			} else {
				s.audit(r, "Unblocked a user", "They can sign in again")
			}
		}
	}
	if errors.Is(err, records.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "No user has that id.")
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAdminRemoveUser(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	err := s.Records.DeleteUser(r.Context(), id)
	if errors.Is(err, records.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "No user has that id.")
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r, "Removed a user", "Their account and everything recorded about them is gone")
	w.WriteHeader(http.StatusNoContent)
}

// WriteUsersCSV writes the users as a spreadsheet file, oldest first.
func WriteUsersCSV(w *csv.Writer, list []records.UserRow) error {
	if err := w.Write([]string{"email", "joined", "last_seen", "country", "status", "events", "note"}); err != nil {
		return err
	}
	for i := len(list) - 1; i >= 0; i-- {
		u := list[i]
		if err := w.Write([]string{csvCell(u.Email), iso(u.CreatedAt), iso(u.LastSeenAt), u.Country, u.Status,
			fmt.Sprint(u.Events), csvCell(u.Note)}); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}

func (s *Server) handleAdminUsersCSV(w http.ResponseWriter, r *http.Request) {
	status, ok := userStatusFilter(w, r)
	if !ok {
		return
	}
	list, err := s.Records.ListUsers(r.Context(), records.UserQuery{Status: status})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r, "Downloaded the users", people(len(list)))
	h := w.Header()
	h.Set("Content-Type", "text/csv; charset=utf-8")
	h.Set("Content-Disposition", `attachment; filename="users-`+records.Day(s.now())+`.csv"`)
	h.Set("X-Content-Type-Options", "nosniff")
	if err := WriteUsersCSV(csv.NewWriter(w), list); err != nil {
		s.Log.Error("csv not fully written", "err", err.Error())
	}
}

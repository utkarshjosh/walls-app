package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/utkarshjoshi/walls/apps/api/internal/records"
)

// What the landing page may write: a tick each time the page is opened. It
// does not need to know who is asking. This file also holds the helpers every
// handler that reads a request shares.

const visitsPerIPPerHour = 30

func (s *Server) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// readJSON decodes a small JSON body into v, answering the request itself and
// returning false when the body is not usable. Insisting on the JSON content
// type also means a form on another site cannot post here: a browser will not
// send that type across sites without asking first, and the answer is no.
func readJSON(w http.ResponseWriter, r *http.Request, v any, limit int64) bool {
	if mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); mt != "application/json" {
		writeError(w, http.StatusUnsupportedMediaType, "not_json", "Send the request as application/json.")
		return false
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, limit))
	if err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeError(w, http.StatusRequestEntityTooLarge, "too_large", "The request is too large.")
		} else {
			writeError(w, http.StatusBadRequest, "invalid_body", "The request could not be read.")
		}
		return false
	}
	if err := json.Unmarshal(body, v); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "The request is not valid JSON.")
		return false
	}
	return true
}

// country is the visitor's country as Cloudflare reports it, or "". It is the
// only thing about where someone is that is kept; their address never is.
func country(r *http.Request) string {
	c := strings.ToUpper(strings.TrimSpace(r.Header.Get("CF-IPCountry")))
	if len(c) != 2 || c == "XX" || c == "T1" {
		return ""
	}
	for _, ch := range c {
		if ch < 'A' || ch > 'Z' {
			return ""
		}
	}
	return c
}

// count adds one to a daily count. A count is never worth failing a request
// for, so errors are logged and dropped.
func (s *Server) count(r *http.Request, kind, subject string) {
	if s.Records == nil {
		return
	}
	// Finish even if the client hangs up as soon as it has its answer.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 2*time.Second)
	defer cancel()
	if err := s.Records.Count(ctx, s.now(), kind, subject); err != nil {
		s.Log.Warn("count not recorded", "kind", kind, "err", err.Error())
	}
}

// handleVisit counts one opening of the landing page. The page sends it once
// per browser tab. Nothing is read from the request and nothing about the
// visitor is kept: the day's number goes up by one.
func (s *Server) handleVisit(w http.ResponseWriter, r *http.Request) {
	if s.Records != nil && s.visitByIP.allow(clientKey(r), s.now()) {
		s.count(r, records.CountVisit, "")
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}

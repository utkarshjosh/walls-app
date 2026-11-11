package httpapi

import (
	"embed"
	"net/http"
)

// The admin portal's pages: three small files with no build step, carried
// inside the program so the portal is deployed with it.
//
//go:embed adminui/index.html adminui/admin.css adminui/admin.js
var adminFiles embed.FS

var adminTypes = map[string]string{
	"index.html": "text/html; charset=utf-8",
	"admin.css":  "text/css; charset=utf-8",
	"admin.js":   "text/javascript; charset=utf-8",
}

// handleAdminPage serves the portal's pages. They hold no data: everything
// shown comes from /v1/admin, which checks the sign-in on every request.
func (s *Server) handleAdminPage(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("file")
	if name == "" {
		name = "index.html"
	}
	ctype, ok := adminTypes[name]
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "There is nothing at this address.")
		return
	}
	body, err := adminFiles.ReadFile("adminui/" + name)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	h := w.Header()
	h.Set("Content-Type", ctype)
	h.Set("Cache-Control", "no-cache")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("X-Robots-Tag", "noindex, nofollow")
	h.Set("Referrer-Policy", "no-referrer")
	// The page may load only its own files and talk only to this server, and
	// no other site may put it in a frame.
	h.Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self' data:; font-src 'self'; connect-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
	_, _ = w.Write(body)
}

package httpapi

import (
	"net/http"
	"runtime/debug"
	"slices"
	"time"
)

// statusWriter remembers the response status for the request log.
type statusWriter struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (w *statusWriter) WriteHeader(code int) {
	if !w.wrote {
		w.status, w.wrote = code, true
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if !w.wrote {
		w.status, w.wrote = http.StatusOK, true
	}
	return w.ResponseWriter.Write(b)
}

// Unwrap lets http.ResponseController reach the real writer (for deadlines
// and flushing).
func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// logger writes one line per request: method, path, status, duration.
func (s *Server) logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		s.Log.Info("request", "method", r.Method, "path", r.URL.Path, "status", sw.status,
			"duration", time.Since(start).Round(10*time.Microsecond).String())
	})
}

// recoverer turns a panic in a handler into a logged 500 instead of a dropped
// connection.
func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			v := recover()
			if v == nil {
				return
			}
			if v == http.ErrAbortHandler { // deliberate abort; let net/http handle it
				panic(v)
			}
			s.Log.Error("panic", "method", r.Method, "path", r.URL.Path, "panic", v, "stack", string(debug.Stack()))
			writeError(w, http.StatusInternalServerError, "internal", "Something went wrong on the server.")
		}()
		next.ServeHTTP(w, r)
	})
}

// cors answers browsers' cross-origin checks. Only origins from the
// configured list are echoed back; anything else gets no CORS headers, which
// makes the browser refuse to share the response with that page.
func (s *Server) cors(next http.Handler) http.Handler {
	allowAny := slices.Contains(s.CORSOrigins, "*")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Add("Vary", "Origin")
		origin := r.Header.Get("Origin")
		allowed := origin != "" && (allowAny || slices.Contains(s.CORSOrigins, origin))
		if allowed {
			h.Set("Access-Control-Allow-Origin", origin)
			h.Set("Access-Control-Expose-Headers", "Content-Length, Content-Range, Accept-Ranges")
		}
		if r.Method != http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}
		// OPTIONS: a preflight check, or a plain "what can I do here?".
		h.Set("Allow", "GET, HEAD, OPTIONS")
		if allowed && r.Header.Get("Access-Control-Request-Method") != "" {
			h.Add("Vary", "Access-Control-Request-Method")
			h.Add("Vary", "Access-Control-Request-Headers")
			h.Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
			if req := r.Header.Get("Access-Control-Request-Headers"); req != "" {
				h.Set("Access-Control-Allow-Headers", req)
			} else {
				h.Set("Access-Control-Allow-Headers", "Range, Content-Type, Accept")
			}
			h.Set("Access-Control-Max-Age", "600")
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

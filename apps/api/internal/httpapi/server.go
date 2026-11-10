// Package httpapi is the read-only JSON API the app's Discover section calls,
// plus the endpoint that serves (or redirects to) the image and video files.
package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/utkarshjoshi/walls/apps/api/internal/catalog"
	"github.com/utkarshjoshi/walls/apps/api/internal/ingest"
	"github.com/utkarshjoshi/walls/apps/api/internal/storage"
)

const (
	// grantTTL is how long a presigned download link stays valid.
	grantTTL = 15 * time.Minute

	cacheLists     = "public, max-age=60"
	cacheImmutable = "public, max-age=31536000, immutable"
	cacheRedirect  = "private, max-age=600"

	defaultLimit = 30
	maxLimit     = 60
)

// Server holds what the handlers need.
type Server struct {
	DB          *catalog.DB
	Store       storage.Store
	StorageName string   // "local" or "s3", reported by /v1/health
	PublicURL   string   // optional absolute prefix for file URLs, no trailing slash
	CORSOrigins []string // allowed browser origins; "*" allows any
	Log         *slog.Logger
}

// Handler returns the full HTTP handler: routes wrapped in CORS, request
// logging and panic recovery.
func (s *Server) Handler() http.Handler {
	if s.Log == nil {
		s.Log = slog.New(slog.DiscardHandler)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/health", s.handleHealth)
	mux.HandleFunc("GET /v1/categories", s.handleCategories)
	mux.HandleFunc("GET /v1/wallpapers", s.handleList)
	mux.HandleFunc("GET /v1/wallpapers/{id}", s.handleItem)
	mux.HandleFunc("GET /v1/packs", s.handlePacks)
	mux.HandleFunc("GET /v1/packs/{id}", s.handlePack)
	mux.HandleFunc("GET /v1/files/{id}/{variant}", s.handleFile)
	// Everything else, including wrong methods on the routes above (the mux
	// falls through to this pattern rather than answering in plain text).
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD, OPTIONS")
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "This API is read-only; use GET or HEAD.")
			return
		}
		writeError(w, http.StatusNotFound, "not_found", "There is nothing at this address.")
	})
	return s.recoverer(s.logger(s.cors(mux)))
}

// --- JSON shapes (the contract with the UI) ----------------------------------

type itemJSON struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Kind      string    `json:"kind"`
	Width     int       `json:"width"`
	Height    int       `json:"height"`
	Bytes     int64     `json:"bytes"`
	Category  string    `json:"category"`
	Tags      []string  `json:"tags"`
	Author    string    `json:"author"`
	License   string    `json:"license"`
	SourceURL *string   `json:"sourceUrl"`
	Color     string    `json:"color"`
	Layout    *string   `json:"layout"` // scenes: "canvas" or "per-display"; null for images and videos
	CreatedAt string    `json:"createdAt"`
	Files     filesJSON `json:"files"`
}

type filesJSON struct {
	Thumb    string  `json:"thumb"`
	Preview  string  `json:"preview"`
	Original string  `json:"original"`
	Poster   *string `json:"poster"`
}

type packJSON struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Count       int         `json:"count"`
	Covers      []string    `json:"covers"`
	Items       *[]itemJSON `json:"items,omitempty"` // only on GET /v1/packs/{id}
}

type healthJSON struct {
	Status  string `json:"status"`
	Storage string `json:"storage"`
	Items   int    `json:"items"`
}

type listJSON struct {
	Items      []itemJSON `json:"items"`
	NextCursor *string    `json:"nextCursor"`
}

type categoryJSON struct {
	Slug  string `json:"slug"`
	Name  string `json:"name"`
	Count int    `json:"count"`
}

func (s *Server) fileURL(id, variant string) string {
	return s.PublicURL + "/v1/files/" + id + "/" + variant
}

func (s *Server) item(it catalog.Item) itemJSON {
	out := itemJSON{
		ID: it.ID, Title: it.Title, Kind: it.Kind, Width: it.Width, Height: it.Height, Bytes: it.Bytes,
		Category: it.Category, Tags: it.Tags, Author: it.Author, License: it.License, Color: it.Color,
		CreatedAt: it.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		Files: filesJSON{
			Thumb:    s.fileURL(it.ID, "thumb"),
			Preview:  s.fileURL(it.ID, "preview"),
			Original: s.fileURL(it.ID, "original"),
		},
	}
	if out.Tags == nil {
		out.Tags = []string{}
	}
	if it.SourceURL != "" {
		out.SourceURL = &it.SourceURL
	}
	if it.HasPoster {
		p := s.fileURL(it.ID, "poster")
		out.Files.Poster = &p
	}
	if it.Kind == catalog.KindScene {
		// A scene always has a layout for the UI to act on. One stored
		// without it draws one continuous picture, as every early scene did.
		layout := it.Layout
		if layout == "" {
			layout = "canvas"
		}
		out.Layout = &layout
	}
	return out
}

func (s *Server) items(list []catalog.Item) []itemJSON {
	out := make([]itemJSON, len(list))
	for i, it := range list {
		out[i] = s.item(it)
	}
	return out
}

func (s *Server) pack(p catalog.Pack) packJSON {
	out := packJSON{ID: p.ID, Name: p.Name, Description: p.Description, Count: p.Count, Covers: []string{}}
	for _, id := range p.CoverIDs {
		out.Covers = append(out.Covers, s.fileURL(id, "thumb"))
	}
	return out
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

// writeError sends the one error shape every failure uses.
func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, status, map[string]string{"error": code, "message": message})
}

func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error) {
	s.Log.Error("request failed", "method", r.Method, "path", r.URL.Path, "err", err.Error())
	writeError(w, http.StatusInternalServerError, "internal", "Something went wrong on the server.")
}

// --- Handlers -----------------------------------------------------------------

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	n, err := s.DB.CountItems(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, healthJSON{Status: "ok", Storage: s.StorageName, Items: n})
}

func (s *Server) handleCategories(w http.ResponseWriter, r *http.Request) {
	cats, err := s.DB.Categories(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := make([]categoryJSON, len(cats))
	for i, c := range cats {
		out[i] = categoryJSON{Slug: c.Slug, Name: c.Name, Count: c.Count}
	}
	w.Header().Set("Cache-Control", cacheLists)
	writeJSON(w, http.StatusOK, map[string]any{"categories": out})
}

func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	qv := r.URL.Query()
	q := catalog.Query{
		Kind:     qv.Get("kind"),
		Category: strings.TrimSpace(qv.Get("category")),
		Q:        strings.TrimSpace(qv.Get("q")),
		Cursor:   qv.Get("cursor"),
		Limit:    defaultLimit,
	}
	switch q.Kind {
	case "", "image", "live", "video", "scene":
	default:
		writeError(w, http.StatusBadRequest, "invalid_kind", `kind must be "image" or "live".`)
		return
	}
	switch qv.Get("wide") {
	case "", "0", "false":
	case "1", "true":
		q.Wide = true
	default:
		writeError(w, http.StatusBadRequest, "invalid_wide", "wide must be 1 or left out.")
		return
	}
	if len(q.Q) > 200 {
		writeError(w, http.StatusBadRequest, "invalid_query", "The search text is too long.")
		return
	}
	if raw := qv.Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			writeError(w, http.StatusBadRequest, "invalid_limit", fmt.Sprintf("limit must be a whole number from 1 to %d.", maxLimit))
			return
		}
		q.Limit = min(n, maxLimit) // asking for more than the maximum just gets the maximum
	}

	page, err := s.DB.ListItems(r.Context(), q)
	if errors.Is(err, catalog.ErrBadCursor) {
		writeError(w, http.StatusBadRequest, "invalid_cursor", "The cursor is not valid; start again without one.")
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var next *string
	if page.NextCursor != "" {
		next = &page.NextCursor
	}
	w.Header().Set("Cache-Control", cacheLists)
	writeJSON(w, http.StatusOK, listJSON{Items: s.items(page.Items), NextCursor: next})
}

var idPattern = regexp.MustCompile(`^[0-9a-f]{12}$`)

// lookup fetches an item by path id, answering 404 itself when there is none.
func (s *Server) lookup(w http.ResponseWriter, r *http.Request) (catalog.Item, bool) {
	id := r.PathValue("id")
	if idPattern.MatchString(id) {
		it, err := s.DB.GetItem(r.Context(), id)
		if err == nil {
			return it, true
		}
		if !errors.Is(err, catalog.ErrNotFound) {
			s.fail(w, r, err)
			return it, false
		}
	}
	writeError(w, http.StatusNotFound, "not_found", "No wallpaper has that id.")
	return catalog.Item{}, false
}

func (s *Server) handleItem(w http.ResponseWriter, r *http.Request) {
	it, ok := s.lookup(w, r)
	if !ok {
		return
	}
	w.Header().Set("Cache-Control", cacheLists)
	writeJSON(w, http.StatusOK, s.item(it))
}

func (s *Server) handlePacks(w http.ResponseWriter, r *http.Request) {
	packs, err := s.DB.Packs(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := make([]packJSON, len(packs))
	for i, p := range packs {
		out[i] = s.pack(p)
	}
	w.Header().Set("Cache-Control", cacheLists)
	writeJSON(w, http.StatusOK, map[string]any{"packs": out})
}

func (s *Server) handlePack(w http.ResponseWriter, r *http.Request) {
	p, items, err := s.DB.GetPack(r.Context(), r.PathValue("id"))
	if errors.Is(err, catalog.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "No pack has that id.")
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := s.pack(p)
	list := s.items(items)
	out.Items = &list
	w.Header().Set("Cache-Control", cacheLists)
	writeJSON(w, http.StatusOK, out)
}

// handleFile serves one stored file of an item. A store that can sign URLs
// (the bucket) gets a redirect so bytes flow straight from it to the client;
// otherwise (local disk) the file is streamed from here, with Range support
// so video can seek.
func (s *Server) handleFile(w http.ResponseWriter, r *http.Request) {
	it, ok := s.lookup(w, r)
	if !ok {
		return
	}
	original, preview, thumb, poster := ingest.Keys(it.ID, it.OriginalExt)
	var key string
	switch r.PathValue("variant") {
	case "thumb":
		key = thumb
	case "preview":
		key = preview
	case "original":
		key = original
	case "poster":
		if it.HasPoster {
			key = poster
		}
	}
	if key == "" {
		writeError(w, http.StatusNotFound, "not_found", "This wallpaper has no such file; use thumb, preview, original or poster.")
		return
	}
	ctx := r.Context()

	url, err := s.Store.GrantDownload(ctx, key, grantTTL)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if url != "" {
		w.Header().Set("Cache-Control", cacheRedirect)
		if r.Method == http.MethodHead {
			// A signed link is only valid for GET, so a redirect would be no
			// use to a HEAD request; answer with the file's details instead.
			info, err := s.Store.Head(ctx, key)
			if errors.Is(err, storage.ErrNotFound) {
				writeError(w, http.StatusNotFound, "not_found", "The file for this wallpaper is missing from storage.")
				return
			}
			if err != nil {
				s.fail(w, r, err)
				return
			}
			w.Header().Set("Content-Type", info.ContentType)
			w.Header().Set("Content-Length", strconv.FormatInt(info.Size, 10))
			w.WriteHeader(http.StatusOK)
			return
		}
		http.Redirect(w, r, url, http.StatusFound)
		return
	}

	f, info, err := s.Store.Open(ctx, key)
	if errors.Is(err, storage.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "The file for this wallpaper is missing from storage.")
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	defer f.Close()

	// Large videos on slow links need longer than the server-wide write
	// timeout; this handler alone gets a generous one.
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(30 * time.Minute))

	h := w.Header()
	h.Set("Content-Type", info.ContentType)
	h.Set("Cache-Control", cacheImmutable)
	h.Set("X-Content-Type-Options", "nosniff")
	// The id is a hash of the original's bytes, so id+variant never changes.
	h.Set("ETag", `"`+it.ID+"-"+r.PathValue("variant")+`"`)
	http.ServeContent(w, r, "", info.ModTime, f)
}

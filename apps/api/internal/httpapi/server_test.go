package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/utkarshjoshi/walls/apps/api/internal/catalog"
	"github.com/utkarshjoshi/walls/apps/api/internal/ingest"
	"github.com/utkarshjoshi/walls/apps/api/internal/storage"
)

type fixture struct {
	srv      *Server
	handler  http.Handler
	pipeline *ingest.Pipeline
	ids      map[string]string // title → id
	video    []byte
}

func picture(w, h int, tint uint8) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, color.RGBA{uint8(255 * x / w), tint, uint8(255 * y / h), 255})
		}
	}
	return img
}

func jpegBytes(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	db, err := catalog.Open(filepath.Join(dir, "catalog.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	store, err := storage.NewLocal(filepath.Join(dir, "objects"))
	if err != nil {
		t.Fatal(err)
	}
	p := &ingest.Pipeline{DB: db, Store: store}
	f := &fixture{pipeline: p, ids: map[string]string{}}
	db.SetCategory(ctx, "gradients", "Gradients")
	db.SetCategory(ctx, "live", "Live")
	created := time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)

	publish := func(a ingest.Asset) {
		t.Helper()
		a.CreatedAt = created
		a.Origin = "seed"
		a.Meta.Author, a.Meta.License = "Still Originals", "Still Original"
		res, err := p.Publish(ctx, a)
		if err != nil {
			t.Fatal(err)
		}
		f.ids[a.Meta.Title] = res.ID
	}

	img := picture(2400, 1500, 30)
	publish(ingest.Asset{Data: jpegBytes(t, img), Kind: catalog.KindImage, Ext: "jpg", ContentType: "image/jpeg", Image: img,
		Meta: ingest.Meta{Title: "Low Tide", Category: "gradients", Tags: []string{"warm", "calm"}}, Rank: 100})

	pano := picture(3200, 1000, 90)
	publish(ingest.Asset{Data: jpegBytes(t, pano), Kind: catalog.KindImage, Ext: "jpg", ContentType: "image/jpeg", Image: pano,
		Meta: ingest.Meta{Title: "Slow Dawn", Category: "gradients", Tags: []string{"wide"}, SourceURL: "https://example.com/dawn"}, Rank: 200})

	// A "video": 300 kB of patterned bytes, enough to exercise Range requests.
	f.video = make([]byte, 300_000)
	copy(f.video, "\x00\x00\x00\x18ftypmp42")
	for i := 16; i < len(f.video); i++ {
		f.video[i] = byte(i * 7)
	}
	posterImg := picture(1280, 720, 150)
	publish(ingest.Asset{Data: f.video, Kind: catalog.KindVideo, Ext: "mp4", ContentType: "video/mp4", Image: posterImg,
		Meta: ingest.Meta{Title: "Slow Waves", Category: "live", Tags: []string{}}, Rank: 300})

	sceneImg := picture(960, 600, 200)
	publish(ingest.Asset{Data: []byte("<!doctype html><html><body>scene</body></html>"), Kind: catalog.KindScene, Ext: "html",
		ContentType: "text/html; charset=utf-8", Image: sceneImg, PosterJPEG: jpegBytes(t, sceneImg), Color: "#a8676a",
		Meta: ingest.Meta{Title: "Tidal", Category: "live", Tags: []string{"abstract"}}, Rank: 400})

	db.PutPack(ctx, catalog.Pack{ID: "quiet-gradients", Name: "Quiet gradients", Description: "Soft blends.", Position: 1},
		[]string{f.ids["Low Tide"], f.ids["Slow Dawn"]})
	db.PutPack(ctx, catalog.Pack{ID: "live", Name: "Live", Description: "Moving scenes.", Position: 2},
		[]string{f.ids["Tidal"], f.ids["Slow Waves"]})

	f.srv = &Server{DB: db, Store: store, StorageName: "local",
		CORSOrigins: []string{"http://localhost:5173", "tauri://localhost"}}
	f.handler = f.srv.Handler()
	return f
}

func (f *fixture) do(method, target string, headers ...string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, nil)
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	rec := httptest.NewRecorder()
	f.handler.ServeHTTP(rec, req)
	return rec
}

func decode(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	if ct := rec.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q, want JSON", ct)
	}
	var v map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("response is not a JSON object: %v\n%s", err, rec.Body.String())
	}
	return v
}

func keys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

var itemKeys = []string{"author", "bytes", "category", "color", "createdAt", "files", "height", "id", "kind", "license",
	"sourceUrl", "tags", "title", "width"}

// checkItem verifies an item object has exactly the contract's fields and types.
func checkItem(t *testing.T, it map[string]any) {
	t.Helper()
	if !slices.Equal(keys(it), itemKeys) {
		t.Fatalf("item keys = %v, want %v", keys(it), itemKeys)
	}
	id := it["id"].(string)
	files := it["files"].(map[string]any)
	if !slices.Equal(keys(files), []string{"original", "poster", "preview", "thumb"}) {
		t.Fatalf("files keys = %v", keys(files))
	}
	for _, v := range []string{"thumb", "preview", "original"} {
		if files[v] != "/v1/files/"+id+"/"+v {
			t.Errorf("files.%s = %v", v, files[v])
		}
	}
	if _, ok := it["tags"].([]any); !ok {
		t.Errorf("tags must be an array, got %T", it["tags"])
	}
	switch it["kind"] {
	case "image":
		if files["poster"] != nil {
			t.Errorf("images have a null poster, got %v", files["poster"])
		}
	case "video", "scene":
		if files["poster"] != "/v1/files/"+id+"/poster" {
			t.Errorf("poster = %v", files["poster"])
		}
	default:
		t.Errorf("kind = %v", it["kind"])
	}
	for _, n := range []string{"width", "height", "bytes"} {
		if v, ok := it[n].(float64); !ok || v <= 0 || v != float64(int64(v)) {
			t.Errorf("%s = %v, want a positive whole number", n, it[n])
		}
	}
	if s, ok := it["sourceUrl"].(string); it["sourceUrl"] != nil && (!ok || s == "") {
		t.Errorf("sourceUrl = %v, want a string or null", it["sourceUrl"])
	}
	if c, _ := it["color"].(string); len(c) != 7 || c[0] != '#' {
		t.Errorf("color = %v", it["color"])
	}
	if _, err := time.Parse("2006-01-02T15:04:05Z", it["createdAt"].(string)); err != nil {
		t.Errorf("createdAt = %v", it["createdAt"])
	}
}

func TestHealthAndCategories(t *testing.T) {
	f := newFixture(t)

	rec := f.do("GET", "/v1/health")
	if rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != `{"status":"ok","storage":"local","items":4}` {
		t.Fatalf("health: %d %s", rec.Code, rec.Body.String())
	}

	rec = f.do("GET", "/v1/categories")
	want := `{"categories":[{"slug":"gradients","name":"Gradients","count":2},{"slug":"live","name":"Live","count":2}]}`
	if rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != want {
		t.Fatalf("categories: %d %s", rec.Code, rec.Body.String())
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "public, max-age=60" {
		t.Errorf("categories Cache-Control = %q", cc)
	}
}

func TestListAndItem(t *testing.T) {
	f := newFixture(t)

	rec := f.do("GET", "/v1/wallpapers")
	if rec.Code != 200 {
		t.Fatalf("list: %d %s", rec.Code, rec.Body.String())
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "public, max-age=60" {
		t.Errorf("list Cache-Control = %q", cc)
	}
	body := decode(t, rec)
	if !slices.Equal(keys(body), []string{"items", "nextCursor"}) {
		t.Fatalf("list keys = %v", keys(body))
	}
	if body["nextCursor"] != nil {
		t.Errorf("nextCursor on the last page must be null, got %v", body["nextCursor"])
	}
	items := body["items"].([]any)
	var titles []string
	for _, raw := range items {
		it := raw.(map[string]any)
		checkItem(t, it)
		titles = append(titles, it["title"].(string))
	}
	if !slices.Equal(titles, []string{"Low Tide", "Slow Dawn", "Slow Waves", "Tidal"}) {
		t.Errorf("order = %v", titles)
	}

	first := items[0].(map[string]any)
	if first["title"] != "Low Tide" || first["kind"] != "image" || first["width"] != 2400.0 || first["height"] != 1500.0 ||
		first["category"] != "gradients" || first["author"] != "Still Originals" || first["license"] != "Still Original" ||
		first["sourceUrl"] != nil || first["createdAt"] != "2026-11-10T00:00:00Z" ||
		fmt.Sprint(first["tags"]) != "[warm calm]" {
		t.Errorf("first item = %v", first)
	}
	if items[1].(map[string]any)["sourceUrl"] != "https://example.com/dawn" {
		t.Errorf("sourceUrl = %v", items[1].(map[string]any)["sourceUrl"])
	}
	if tags := items[2].(map[string]any)["tags"].([]any); len(tags) != 0 {
		t.Errorf("empty tags should be [], got %v", tags)
	}
	if items[3].(map[string]any)["color"] != "#a8676a" {
		t.Errorf("scene colour = %v", items[3].(map[string]any)["color"])
	}

	titlesFor := func(query string) []string {
		t.Helper()
		rec := f.do("GET", "/v1/wallpapers?"+query)
		if rec.Code != 200 {
			t.Fatalf("%s: %d %s", query, rec.Code, rec.Body.String())
		}
		out := []string{}
		for _, raw := range decode(t, rec)["items"].([]any) {
			out = append(out, raw.(map[string]any)["title"].(string))
		}
		return out
	}
	for query, want := range map[string][]string{
		"kind=image":             {"Low Tide", "Slow Dawn"},
		"kind=live":              {"Slow Waves", "Tidal"},
		"category=live":          {"Slow Waves", "Tidal"},
		"category=nope":          {},
		"wide=1":                 {"Slow Dawn"},
		"q=SLOW":                 {"Slow Dawn", "Slow Waves"},
		"q=abstract":             {"Tidal"},
		"q=gradients&kind=image": {"Low Tide", "Slow Dawn"},
		"kind=live&q=waves":      {"Slow Waves"},
		"limit=60":               {"Low Tide", "Slow Dawn", "Slow Waves", "Tidal"},
		"limit=500":              {"Low Tide", "Slow Dawn", "Slow Waves", "Tidal"},
	} {
		if got := titlesFor(query); !slices.Equal(got, want) {
			t.Errorf("?%s = %v, want %v", query, got, want)
		}
	}

	// Paging: limit=3 then the cursor.
	rec = f.do("GET", "/v1/wallpapers?limit=3")
	page1 := decode(t, rec)
	cursor, _ := page1["nextCursor"].(string)
	if len(page1["items"].([]any)) != 3 || cursor == "" {
		t.Fatalf("page 1: %d items, cursor %v", len(page1["items"].([]any)), page1["nextCursor"])
	}
	if got := titlesFor("limit=3&cursor=" + cursor); !slices.Equal(got, []string{"Tidal"}) {
		t.Errorf("page 2 = %v", got)
	}

	// Bad parameters.
	for query, code := range map[string]string{
		"kind=gif": "invalid_kind", "limit=0": "invalid_limit", "limit=abc": "invalid_limit", "limit=-3": "invalid_limit",
		"cursor=%21%21": "invalid_cursor", "wide=maybe": "invalid_wide", "q=" + strings.Repeat("x", 300): "invalid_query",
	} {
		rec := f.do("GET", "/v1/wallpapers?"+query)
		body := decode(t, rec)
		if rec.Code != 400 || body["error"] != code || body["message"] == "" || len(body) != 2 {
			t.Errorf("?%s: %d %v, want 400 %s", query[:min(len(query), 20)], rec.Code, body, code)
		}
	}

	// Single item.
	rec = f.do("GET", "/v1/wallpapers/"+f.ids["Tidal"])
	it := decode(t, rec)
	checkItem(t, it)
	if rec.Code != 200 || it["title"] != "Tidal" || it["kind"] != "scene" || it["width"] != 960.0 {
		t.Errorf("item: %d %v", rec.Code, it)
	}
}

func TestNotFoundShapes(t *testing.T) {
	f := newFixture(t)
	id := f.ids["Low Tide"]
	for _, target := range []string{
		"/v1/wallpapers/000000000000", // well-formed, unknown
		"/v1/wallpapers/nope",
		"/v1/wallpapers/..%2f..%2fetc",
		"/v1/packs/nope",
		"/v1/files/000000000000/thumb",
		"/v1/files/" + id + "/huge",   // unknown variant
		"/v1/files/" + id + "/poster", // images have no poster
		"/v1/files/" + id,
		"/v1/nothing",
		"/",
		"/v1/wallpapers/",
	} {
		rec := f.do("GET", target)
		body := decode(t, rec)
		if rec.Code != 404 || body["error"] != "not_found" || len(body) != 2 {
			t.Errorf("GET %s: %d %v, want 404 not_found", target, rec.Code, body)
		}
		if msg, _ := body["message"].(string); msg == "" {
			t.Errorf("GET %s: empty message", target)
		}
	}

	for _, method := range []string{"POST", "PUT", "DELETE", "PATCH"} {
		rec := f.do(method, "/v1/wallpapers")
		body := decode(t, rec)
		if rec.Code != 405 || body["error"] != "method_not_allowed" || rec.Header().Get("Allow") == "" {
			t.Errorf("%s: %d %v", method, rec.Code, body)
		}
	}
}

func TestPacks(t *testing.T) {
	f := newFixture(t)
	rec := f.do("GET", "/v1/packs")
	want := fmt.Sprintf(`{"packs":[{"id":"quiet-gradients","name":"Quiet gradients","description":"Soft blends.","count":2,`+
		`"covers":["/v1/files/%s/thumb","/v1/files/%s/thumb"]},{"id":"live","name":"Live","description":"Moving scenes.","count":2,`+
		`"covers":["/v1/files/%s/thumb","/v1/files/%s/thumb"]}]}`,
		f.ids["Low Tide"], f.ids["Slow Dawn"], f.ids["Tidal"], f.ids["Slow Waves"])
	if rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != want {
		t.Fatalf("packs: %d\n got %s\nwant %s", rec.Code, rec.Body.String(), want)
	}

	rec = f.do("GET", "/v1/packs/live")
	body := decode(t, rec)
	if !slices.Equal(keys(body), []string{"count", "covers", "description", "id", "items", "name"}) {
		t.Fatalf("pack keys = %v", keys(body))
	}
	items := body["items"].([]any)
	if rec.Code != 200 || body["count"] != 2.0 || len(items) != 2 || items[0].(map[string]any)["title"] != "Tidal" {
		t.Fatalf("pack detail: %d %v", rec.Code, body)
	}
	for _, raw := range items {
		checkItem(t, raw.(map[string]any))
	}
}

func TestFilesLocal(t *testing.T) {
	f := newFixture(t)
	video := f.ids["Slow Waves"]
	target := "/v1/files/" + video + "/original"

	// Whole file.
	rec := f.do("GET", target)
	if rec.Code != 200 || !bytes.Equal(rec.Body.Bytes(), f.video) {
		t.Fatalf("GET original: %d, %d bytes", rec.Code, rec.Body.Len())
	}
	h := rec.Header()
	if h.Get("Content-Type") != "video/mp4" || h.Get("Cache-Control") != "public, max-age=31536000, immutable" ||
		h.Get("Accept-Ranges") != "bytes" || h.Get("Content-Length") != "300000" || h.Get("ETag") == "" {
		t.Errorf("headers: %v", h)
	}

	// A range from the middle, as a video player seeking would ask.
	rec = f.do("GET", target, "Range", "bytes=100000-100999")
	if rec.Code != 206 || rec.Header().Get("Content-Range") != "bytes 100000-100999/300000" ||
		!bytes.Equal(rec.Body.Bytes(), f.video[100000:101000]) {
		t.Fatalf("range: %d %q, %d bytes", rec.Code, rec.Header().Get("Content-Range"), rec.Body.Len())
	}
	// Open-ended and suffix ranges.
	rec = f.do("GET", target, "Range", "bytes=299990-")
	if rec.Code != 206 || !bytes.Equal(rec.Body.Bytes(), f.video[299990:]) {
		t.Errorf("open-ended range: %d, %d bytes", rec.Code, rec.Body.Len())
	}
	rec = f.do("GET", target, "Range", "bytes=-5")
	if rec.Code != 206 || !bytes.Equal(rec.Body.Bytes(), f.video[299995:]) {
		t.Errorf("suffix range: %d, %d bytes", rec.Code, rec.Body.Len())
	}
	// A range past the end.
	if rec = f.do("GET", target, "Range", "bytes=400000-"); rec.Code != http.StatusRequestedRangeNotSatisfiable {
		t.Errorf("out-of-range: %d", rec.Code)
	}

	// HEAD: headers, no body.
	rec = f.do("HEAD", target)
	if rec.Code != 200 || rec.Body.Len() != 0 || rec.Header().Get("Content-Length") != "300000" || rec.Header().Get("Content-Type") != "video/mp4" {
		t.Errorf("HEAD: %d, %d bytes, %v", rec.Code, rec.Body.Len(), rec.Header())
	}

	// Revalidation with the ETag costs no bytes.
	etag := rec.Header().Get("ETag")
	if rec = f.do("GET", target, "If-None-Match", etag); rec.Code != http.StatusNotModified {
		t.Errorf("If-None-Match: %d", rec.Code)
	}

	// Every variant of every item is served with the right type.
	for title, id := range f.ids {
		for _, variant := range []string{"thumb", "preview", "poster"} {
			rec := f.do("GET", "/v1/files/"+id+"/"+variant)
			if variant == "poster" && (title == "Low Tide" || title == "Slow Dawn") {
				if rec.Code != 404 {
					t.Errorf("%s poster: %d, want 404", title, rec.Code)
				}
				continue
			}
			if rec.Code != 200 || rec.Header().Get("Content-Type") != "image/jpeg" {
				t.Errorf("%s %s: %d %s", title, variant, rec.Code, rec.Header().Get("Content-Type"))
				continue
			}
			cfg, err := jpeg.DecodeConfig(rec.Body)
			if err != nil {
				t.Errorf("%s %s is not a JPEG: %v", title, variant, err)
			}
			if variant == "thumb" && cfg.Width > 640 {
				t.Errorf("%s thumb is %d px wide", title, cfg.Width)
			}
		}
	}
	rec = f.do("GET", "/v1/files/"+f.ids["Tidal"]+"/original")
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "text/html; charset=utf-8" ||
		rec.Header().Get("X-Content-Type-Options") != "nosniff" || !strings.Contains(rec.Body.String(), "scene") {
		t.Errorf("scene original: %d %v", rec.Code, rec.Header())
	}

	// A catalog row whose object has gone missing is a clean 404.
	original, _, _, _ := ingest.Keys(video, "mp4")
	f.srv.Store.Delete(context.Background(), original)
	rec = f.do("GET", target)
	if body := decode(t, rec); rec.Code != 404 || body["error"] != "not_found" {
		t.Errorf("missing object: %d %v", rec.Code, body)
	}
}

// signingStore behaves like a bucket: it hands out download URLs.
type signingStore struct {
	storage.Store
	ttl time.Duration
}

func (s *signingStore) GrantDownload(ctx context.Context, key string, ttl time.Duration) (string, error) {
	s.ttl = ttl
	return "https://bucket.example/walls-assets/" + key + "?X-Amz-Signature=abc", nil
}

func TestFilesRedirectWhenStoreSigns(t *testing.T) {
	f := newFixture(t)
	signer := &signingStore{Store: f.srv.Store}
	f.srv.Store, f.srv.StorageName = signer, "s3"
	f.handler = f.srv.Handler()
	id := f.ids["Slow Waves"]

	rec := f.do("GET", "/v1/files/"+id+"/original")
	wantURL := "https://bucket.example/walls-assets/catalog/" + id + "/original.mp4?X-Amz-Signature=abc"
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != wantURL {
		t.Fatalf("redirect: %d %q", rec.Code, rec.Header().Get("Location"))
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "private, max-age=600" {
		t.Errorf("redirect Cache-Control = %q", cc)
	}
	if signer.ttl != 15*time.Minute {
		t.Errorf("grant TTL = %v, want 15m", signer.ttl)
	}

	// HEAD cannot follow a GET-signed link, so it is answered directly.
	rec = f.do("HEAD", "/v1/files/"+id+"/thumb")
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "image/jpeg" || rec.Header().Get("Content-Length") == "" ||
		rec.Header().Get("Location") != "" {
		t.Errorf("HEAD with a signing store: %d %v", rec.Code, rec.Header())
	}

	// Unknown ids never reach the store.
	if rec = f.do("GET", "/v1/files/000000000000/thumb"); rec.Code != 404 {
		t.Errorf("unknown id: %d", rec.Code)
	}
	if rec = f.do("GET", "/v1/health"); !strings.Contains(rec.Body.String(), `"storage":"s3"`) {
		t.Errorf("health: %s", rec.Body.String())
	}
}

func TestCORS(t *testing.T) {
	f := newFixture(t)

	// Preflight from an allowed origin.
	rec := f.do("OPTIONS", "/v1/wallpapers", "Origin", "http://localhost:5173",
		"Access-Control-Request-Method", "GET", "Access-Control-Request-Headers", "range")
	h := rec.Header()
	if rec.Code != http.StatusNoContent || h.Get("Access-Control-Allow-Origin") != "http://localhost:5173" ||
		h.Get("Access-Control-Allow-Methods") != "GET, HEAD, OPTIONS" || h.Get("Access-Control-Allow-Headers") != "range" ||
		h.Get("Access-Control-Max-Age") == "" || !strings.Contains(strings.Join(h.Values("Vary"), ","), "Origin") {
		t.Errorf("preflight: %d %v", rec.Code, h)
	}

	// The desktop shell's origin.
	rec = f.do("OPTIONS", "/v1/files/"+f.ids["Low Tide"]+"/thumb", "Origin", "tauri://localhost", "Access-Control-Request-Method", "GET")
	if rec.Code != http.StatusNoContent || rec.Header().Get("Access-Control-Allow-Origin") != "tauri://localhost" {
		t.Errorf("tauri preflight: %d %v", rec.Code, rec.Header())
	}

	// Preflight from somewhere else gets no permission.
	rec = f.do("OPTIONS", "/v1/wallpapers", "Origin", "https://evil.example", "Access-Control-Request-Method", "GET")
	if rec.Header().Get("Access-Control-Allow-Origin") != "" || rec.Header().Get("Access-Control-Allow-Methods") != "" {
		t.Errorf("foreign preflight was allowed: %v", rec.Header())
	}

	// Ordinary requests: allowed origins are echoed, with the exposed headers.
	rec = f.do("GET", "/v1/files/"+f.ids["Slow Waves"]+"/original", "Origin", "http://localhost:5173", "Range", "bytes=0-9")
	h = rec.Header()
	if rec.Code != 206 || h.Get("Access-Control-Allow-Origin") != "http://localhost:5173" ||
		h.Get("Access-Control-Expose-Headers") != "Content-Length, Content-Range, Accept-Ranges" {
		t.Errorf("CORS on file request: %d %v", rec.Code, h)
	}
	rec = f.do("GET", "/v1/wallpapers", "Origin", "https://evil.example")
	if rec.Code != 200 || rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Errorf("foreign origin: %d %v", rec.Code, rec.Header())
	}
	// Errors carry CORS headers too, so the UI can read them.
	rec = f.do("GET", "/v1/wallpapers/nope", "Origin", "http://localhost:5173")
	if rec.Code != 404 || rec.Header().Get("Access-Control-Allow-Origin") != "http://localhost:5173" {
		t.Errorf("CORS on 404: %d %v", rec.Code, rec.Header())
	}

	// "*" allows anyone.
	f.srv.CORSOrigins = []string{"*"}
	f.handler = f.srv.Handler()
	rec = f.do("GET", "/v1/health", "Origin", "https://anywhere.example")
	if rec.Header().Get("Access-Control-Allow-Origin") != "https://anywhere.example" {
		t.Errorf("wildcard origin: %v", rec.Header())
	}
}

func TestPublicURLMakesFileLinksAbsolute(t *testing.T) {
	f := newFixture(t)
	f.srv.PublicURL = "https://walls.example.com"
	f.handler = f.srv.Handler()
	id := f.ids["Tidal"]

	it := decode(t, f.do("GET", "/v1/wallpapers/"+id))
	files := it["files"].(map[string]any)
	for _, v := range []string{"thumb", "preview", "original", "poster"} {
		if files[v] != "https://walls.example.com/v1/files/"+id+"/"+v {
			t.Errorf("files.%s = %v", v, files[v])
		}
	}
	packs := decode(t, f.do("GET", "/v1/packs"))["packs"].([]any)
	cover := packs[0].(map[string]any)["covers"].([]any)[0].(string)
	if !strings.HasPrefix(cover, "https://walls.example.com/v1/files/") {
		t.Errorf("cover = %s", cover)
	}
}

func TestPanicBecomesJSON500(t *testing.T) {
	s := &Server{}
	s.Handler() // sets the discard logger
	h := s.recoverer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") }))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/x", nil))
	var body map[string]string
	json.Unmarshal(rec.Body.Bytes(), &body)
	if rec.Code != 500 || body["error"] != "internal" || body["message"] == "" {
		t.Fatalf("panic response: %d %s", rec.Code, rec.Body.String())
	}
}

// TestOverRealHTTP runs the handler behind a real listener, the way the UI
// will reach it, to catch anything the in-memory recorder hides.
func TestOverRealHTTP(t *testing.T) {
	f := newFixture(t)
	ts := httptest.NewServer(f.handler)
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/v1/files/"+f.ids["Slow Waves"]+"/original", nil)
	req.Header.Set("Range", "bytes=1000-1999")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 206 || !bytes.Equal(got, f.video[1000:2000]) || resp.Header.Get("Content-Range") != "bytes 1000-1999/300000" {
		t.Fatalf("range over HTTP: %d, %d bytes, %q", resp.StatusCode, len(got), resp.Header.Get("Content-Range"))
	}

	resp, err = http.Head(ts.URL + "/v1/wallpapers")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("HEAD on a JSON route: %d", resp.StatusCode)
	}
}

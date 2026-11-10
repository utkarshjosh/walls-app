package seed

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/utkarshjoshi/walls/apps/api/internal/catalog"
	"github.com/utkarshjoshi/walls/apps/api/internal/httpapi"
	"github.com/utkarshjoshi/walls/apps/api/internal/ingest"
	"github.com/utkarshjoshi/walls/apps/api/internal/scenecheck"
	"github.com/utkarshjoshi/walls/apps/api/internal/storage"
)

func newPipeline(t *testing.T) *ingest.Pipeline {
	t.Helper()
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
	return &ingest.Pipeline{DB: db, Store: store}
}

// poster returns a 1920×1200 JPEG, the size the scene standard requires.
var posters sync.Map

func poster(t *testing.T, tint uint8) []byte {
	t.Helper()
	if data, ok := posters.Load(tint); ok {
		return data.([]byte)
	}
	img := image.NewRGBA(image.Rect(0, 0, 1920, 1200))
	for y := 0; y < 1200; y++ {
		for x := 0; x < 1920; x++ {
			img.SetRGBA(x, y, color.RGBA{uint8(x / 8), tint, uint8(y / 5), 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	posters.Store(tint, buf.Bytes())
	return buf.Bytes()
}

// sceneHTML is a minimal scene that meets the standard. The marker makes
// each scene's bytes, and so its id, different.
func sceneHTML(marker string) []byte {
	return []byte("<!doctype html>\n<html>\n<head>\n<meta charset=\"utf-8\">\n" + scenecheck.PolicyLine + "\n<title>" + marker + "</title>\n" +
		"<style>html,body{margin:0;background:#1c2230}</style>\n</head>\n<body>\n<canvas id=\"c\"></canvas>\n<script>\n" +
		"(function () {\n  var paused = /paused=1/.test(location.hash); // " + marker + "\n" +
		"  window.addEventListener('hashchange', function () { paused = /paused=1/.test(location.hash); });\n})();\n" +
		"</script>\n</body>\n</html>\n")
}

const firstPartyMeta = `{"standard":1,"title":"Tidal","tags":["abstract","warm"],"color":"#A8676A","author":"Still Originals","license":"Still Original","sourceUrl":null,"layout":"canvas"}`
const adaptedMeta = `{"standard":1,"title":"Slow Sea","tags":["sea"],"author":"Ana Ruiz","license":"CC BY 4.0","sourceUrl":"https://example.com/slow-sea","layout":"per-display"}`

func writeScene(t *testing.T, root, slug string, files map[string][]byte) {
	t.Helper()
	dir := filepath.Join(root, slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func allItems(t *testing.T, p *ingest.Pipeline) []catalog.Item {
	t.Helper()
	page, err := p.DB.ListItems(context.Background(), catalog.Query{Limit: 1000})
	if err != nil {
		t.Fatal(err)
	}
	return page.Items
}

func TestSeed(t *testing.T) {
	ctx := context.Background()
	p := newPipeline(t)
	scenes := t.TempDir()
	tidalHTML := sceneHTML("tidal")
	writeScene(t, scenes, "tidal", map[string][]byte{
		"scene.html": tidalHTML, "poster.jpg": poster(t, 80), "meta.json": []byte(firstPartyMeta),
	})
	writeScene(t, scenes, "slow-sea", map[string][]byte{
		"scene.html": sceneHTML("slow sea"), "poster.jpg": poster(t, 160), "meta.json": []byte(adaptedMeta),
	})
	// Scenes that fail the gate: each is skipped with its reasons, and the
	// run carries on.
	writeScene(t, scenes, "bad-calls-out", map[string][]byte{
		"scene.html": bytes.Replace(sceneHTML("x"), []byte("(function () {"), []byte("(function () { fetch('/collect');"), 1),
		"poster.jpg": poster(t, 80), "meta.json": []byte(firstPartyMeta),
	})
	writeScene(t, scenes, "bad-old-style", map[string][]byte{ // what scenes looked like before the standard
		"scene.html": bytes.Replace(sceneHTML("y"), []byte(scenecheck.Policy), []byte("default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'"), 1),
		"poster.jpg": poster(t, 80), "meta.json": []byte(`{"title":"Old","tags":["x"],"color":"#112233"}`),
	})
	writeScene(t, scenes, "bad-no-poster", map[string][]byte{
		"scene.html": sceneHTML("z"), "meta.json": []byte(firstPartyMeta),
	})

	opt := Options{ScenesDir: scenes, Scale: 1.0 / 24} // 160×100 renders keep the test quick
	var log strings.Builder
	opt.Logf = func(format string, args ...any) { fmt.Fprintf(&log, format+"\n", args...) }
	stats, err := Run(ctx, p, opt)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Added != 32 || stats.Scenes != 2 || len(stats.ScenesSkipped) != 3 || stats.Removed != 0 {
		t.Fatalf("first run stats: %+v", stats)
	}
	for i, want := range []struct{ name, reason string }{
		{"bad-calls-out", "forbidden: fetch"},
		{"bad-no-poster", "poster.jpg: is missing"},
		{"bad-old-style", "the policy text differs"},
	} {
		got := stats.ScenesSkipped[i]
		if got.Name != want.name || !strings.Contains(strings.Join(got.Reasons, "\n"), want.reason) {
			t.Errorf("skipped scene %d = %+v, want %s with a reason mentioning %q", i, got, want.name, want.reason)
		}
		if !strings.Contains(log.String(), want.reason) {
			t.Errorf("the log does not say why %s was skipped (%q)", want.name, want.reason)
		}
	}
	// Every reason is reported, not just the first.
	if n := len(stats.ScenesSkipped[2].Reasons); n != 5 { // policy text + standard + author + licence + layout
		t.Errorf("bad-old-style has %d reasons, want 5: %v", n, stats.ScenesSkipped[2].Reasons)
	}

	items := allItems(t, p)
	if len(items) != 32 {
		t.Fatalf("catalog has %d items, want 30 pictures + 2 scenes", len(items))
	}
	byKind := map[string]int{}
	wide := 0
	for _, it := range items {
		byKind[it.Kind]++
		if !it.CreatedAt.Equal(Date) {
			t.Errorf("%s: created %v, want the fixed seed date", it.Title, it.CreatedAt)
		}
		if it.Kind == catalog.KindImage {
			if it.Author != Author || it.License != License {
				t.Errorf("%s: credited to %q / %q", it.Title, it.Author, it.License)
			}
			if it.Width*10 >= it.Height*26 {
				wide++
			}
		}
	}
	if byKind[catalog.KindImage] != 30 || byKind[catalog.KindScene] != 2 || wide != 6 {
		t.Errorf("kinds %v, %d panoramas; want 30 images, 2 scenes, 6 panoramas", byKind, wide)
	}

	// Feed order alternates categories.
	for i := 1; i < len(items); i++ {
		if items[i].Category == items[i-1].Category {
			t.Errorf("feed positions %d and %d share category %q", i-1, i, items[i].Category)
		}
	}

	cats, _ := p.DB.Categories(ctx)
	got := map[string]string{}
	total := 0
	for _, c := range cats {
		got[c.Slug] = c.Name
		total += c.Count
	}
	want := map[string]string{"gradients": "Gradients", "horizons": "Horizons", "dunes": "Dunes", "contours": "Contours", "aurora": "Aurora", "live": "Live"}
	if len(got) != len(want) || total != 32 {
		t.Errorf("categories = %v (total %d)", got, total)
	}
	for slug, name := range want {
		if got[slug] != name {
			t.Errorf("category %s is named %q, want %q", slug, got[slug], name)
		}
	}

	// Scenes take their title, credits, tags, colour and source from meta.json.
	live, _ := p.DB.ListItems(ctx, catalog.Query{Kind: "live"})
	if len(live.Items) != 2 {
		t.Fatalf("live items = %d", len(live.Items))
	}
	var tidal, sea catalog.Item
	for _, it := range live.Items {
		switch it.Title {
		case "Tidal":
			tidal = it
		case "Slow Sea":
			sea = it
		}
	}
	if tidal.ID == "" || sea.ID == "" {
		t.Fatalf("scene titles: %q, %q", live.Items[0].Title, live.Items[1].Title)
	}
	if tidal.Kind != catalog.KindScene || tidal.Category != "live" || tidal.Color != "#a8676a" || !tidal.HasPoster ||
		tidal.Width != 1920 || tidal.Height != 1200 || tidal.OriginalExt != "html" || !slices.Equal(tidal.Tags, []string{"abstract", "warm"}) ||
		tidal.Author != "Still Originals" || tidal.License != "Still Original" || tidal.SourceURL != "" || tidal.Layout != "canvas" {
		t.Errorf("tidal = %+v", tidal)
	}
	if sea.Author != "Ana Ruiz" || sea.License != "CC BY 4.0" || sea.SourceURL != "https://example.com/slow-sea" ||
		!slices.Equal(sea.Tags, []string{"sea"}) || !strings.HasPrefix(sea.Color, "#") || len(sea.Color) != 7 || sea.Layout != "per-display" {
		t.Errorf("an adapted scene must keep its own credits; got %+v", sea)
	}

	// The stored original is the source file byte for byte, and the id is its hash.
	original, preview, thumb, posterKey := ingest.Keys(tidal.ID, "html")
	f, info, err := p.Store.Open(ctx, original)
	if err != nil {
		t.Fatal(err)
	}
	html, _ := io.ReadAll(f)
	f.Close()
	sum := sha256.Sum256(tidalHTML)
	if info.ContentType != "text/html; charset=utf-8" || !bytes.Equal(html, tidalHTML) || tidal.ID != hex.EncodeToString(sum[:])[:12] ||
		tidal.Bytes != int64(len(tidalHTML)) {
		t.Errorf("scene original: type %q, %d bytes, id %s", info.ContentType, len(html), tidal.ID)
	}
	for _, key := range []string{preview, thumb, posterKey} {
		if info, err := p.Store.Head(ctx, key); err != nil || info.ContentType != "image/jpeg" {
			t.Errorf("%s: %v %+v", key, err, info)
		}
	}

	// Packs.
	packs, _ := p.DB.Packs(ctx)
	counts := map[string]int{}
	for _, pk := range packs {
		counts[pk.ID] = pk.Count
		if pk.Description == "" || strings.Count(pk.Description, ".") != 1 || len(pk.CoverIDs) == 0 || len(pk.CoverIDs) > 3 {
			t.Errorf("pack %s: description %q, covers %v", pk.ID, pk.Description, pk.CoverIDs)
		}
	}
	if counts["quiet-gradients"] != 8 || counts["two-displays"] != 6 || counts["live"] != 2 || counts["after-dark"] < 8 {
		t.Errorf("pack counts = %v", counts)
	}
	_, wideItems, _ := p.DB.GetPack(ctx, "two-displays")
	for _, it := range wideItems {
		if it.Width*10 < it.Height*26 {
			t.Errorf("two-displays contains %q which is not a panorama", it.Title)
		}
	}
	_, gradients, _ := p.DB.GetPack(ctx, "quiet-gradients")
	for _, it := range gradients {
		if it.Category != "gradients" {
			t.Errorf("quiet-gradients contains %q from %q", it.Title, it.Category)
		}
	}

	// Second run: nothing changes, same ids.
	before := allItems(t, p)
	stats, err = Run(ctx, p, opt)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Added != 0 || stats.Updated != 0 || stats.Unchanged != 32 || stats.Removed != 0 {
		t.Errorf("second run stats: %+v", stats)
	}
	after := allItems(t, p)
	if !slices.EqualFunc(before, after, func(a, b catalog.Item) bool { return a.ID == b.ID && a.Rank == b.Rank }) {
		t.Error("re-seeding changed ids or order")
	}

	// Changing only meta.json: scene.html is the same bytes, so the id is the
	// same, and the next seed must still pick up the new details. Here the
	// scene turns out to be one that draws itself whole on each display.
	api := (&httpapi.Server{DB: p.DB, Store: p.Store, StorageName: "local"}).Handler()
	layoutOverAPI := func(id string) any {
		t.Helper()
		rec := httptest.NewRecorder()
		api.ServeHTTP(rec, httptest.NewRequest("GET", "/v1/wallpapers/"+id, nil))
		var item map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &item); err != nil || rec.Code != 200 {
			t.Fatalf("GET item %s: %d %s", id, rec.Code, rec.Body.String())
		}
		layout, present := item["layout"]
		if !present {
			t.Fatalf("item %s has no \"layout\" key: %s", id, rec.Body.String())
		}
		return layout
	}
	if got := layoutOverAPI(tidal.ID); got != "canvas" {
		t.Fatalf("before the change the API reports layout %v, want canvas", got)
	}
	writeScene(t, scenes, "tidal", map[string][]byte{
		"meta.json": []byte(strings.Replace(firstPartyMeta, `"layout":"canvas"`, `"layout":"per-display"`, 1)),
	})
	stats, err = Run(ctx, p, opt)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Added != 0 || stats.Updated != 1 || stats.Unchanged != 31 || stats.Removed != 0 {
		t.Errorf("after changing only a layout: %+v", stats)
	}
	if now, err := p.DB.GetItem(ctx, tidal.ID); err != nil || now.Layout != "per-display" || now.Title != "Tidal" || now.Rank != tidal.Rank {
		t.Errorf("tidal after the change: %+v %v", now, err)
	}
	if got := layoutOverAPI(tidal.ID); got != "per-display" {
		t.Errorf("after re-seeding the API reports layout %v, want per-display", got)
	}
	if got := layoutOverAPI(items[0].ID); got != nil {
		t.Errorf("a picture's layout over the API = %v, want null", got)
	}
	// The other meta.json fields follow the same path.
	writeScene(t, scenes, "tidal", map[string][]byte{
		"meta.json": []byte(`{"standard":1,"title":"Tidal Two","tags":["sea"],"color":"#102030","author":"Ana Ruiz","license":"CC BY 4.0","sourceUrl":"https://example.com/t","layout":"per-display"}`),
	})
	if stats, err = Run(ctx, p, opt); err != nil || stats.Updated != 1 || stats.Added != 0 {
		t.Fatalf("after changing the other details: %+v %v", stats, err)
	}
	if now, _ := p.DB.GetItem(ctx, tidal.ID); now.Title != "Tidal Two" || now.Author != "Ana Ruiz" || now.License != "CC BY 4.0" ||
		now.SourceURL != "https://example.com/t" || now.Color != "#102030" || !slices.Equal(now.Tags, []string{"sea"}) {
		t.Errorf("tidal after changing its details: %+v", now)
	}
	if page, _ := p.DB.ListItems(ctx, catalog.Query{Q: "tidal two"}); len(page.Items) != 1 {
		t.Errorf("search does not find the scene under its new title")
	}

	// A missing scenes folder (seed run from the wrong place) leaves the
	// loaded scenes alone.
	stats, err = Run(ctx, p, Options{ScenesDir: filepath.Join(scenes, "nope"), Scale: opt.Scale})
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := p.DB.CountItems(ctx); n != 32 || stats.Scenes != 0 || stats.Removed != 0 {
		t.Errorf("missing scenes folder: %d items, %+v", n, stats)
	}

	// A published scene whose file is changed so that it no longer passes is
	// retired at the next run: what is published must be what passes.
	writeScene(t, scenes, "tidal", map[string][]byte{
		"scene.html": bytes.Replace(tidalHTML, []byte("(function () {"), []byte("(function () { new WebSocket('wss://e.example');"), 1),
	})
	stats, err = Run(ctx, p, opt)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Removed != 1 || stats.Scenes != 1 || len(stats.ScenesSkipped) != 4 {
		t.Errorf("after breaking tidal: %+v", stats)
	}
	if _, err := p.DB.GetItem(ctx, tidal.ID); !errors.Is(err, catalog.ErrNotFound) {
		t.Error("a scene that stopped passing the gate is still published")
	}
	if _, err := p.Store.Head(ctx, original); !errors.Is(err, storage.ErrNotFound) {
		t.Error("the retired scene's files were not removed")
	}
	if pk, items, _ := p.DB.GetPack(ctx, "live"); pk.Count != 1 || items[0].Title != "Slow Sea" {
		t.Errorf("live pack after retiring tidal: %+v", pk)
	}

	// An edited scene that still passes replaces its old version.
	writeScene(t, scenes, "slow-sea", map[string][]byte{"scene.html": sceneHTML("slow sea, second cut")})
	stats, err = Run(ctx, p, opt)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Added != 1 || stats.Removed != 1 || stats.Scenes != 1 {
		t.Errorf("after editing slow-sea: %+v", stats)
	}
	if _, err := p.DB.GetItem(ctx, sea.ID); !errors.Is(err, catalog.ErrNotFound) {
		t.Error("the old version of the edited scene is still in the catalog")
	}

	// With no passing scenes left the Live pack disappears.
	os.RemoveAll(filepath.Join(scenes, "slow-sea"))
	if _, err := Run(ctx, p, opt); err != nil {
		t.Fatal(err)
	}
	if _, _, err := p.DB.GetPack(ctx, "live"); !errors.Is(err, catalog.ErrNotFound) {
		t.Errorf("live pack should be gone, got %v", err)
	}
	if n, _ := p.DB.CountItems(ctx); n != 30 {
		t.Errorf("final count = %d, want 30", n)
	}
}

func TestSeedStopsAtStorageCap(t *testing.T) {
	p := newPipeline(t)
	p.Cap = 20_000 // far too small for the set
	_, err := Run(context.Background(), p, Options{Scale: 1.0 / 24})
	if !errors.Is(err, ingest.ErrOverCap) || !strings.Contains(err.Error(), "WALLS_STORAGE_CAP_BYTES") {
		t.Fatalf("got %v, want a clear storage-cap error", err)
	}
	if total, _ := p.DB.StoredBytes(context.Background(), ""); total > p.Cap {
		t.Errorf("stored %d bytes despite a cap of %d", total, p.Cap)
	}
}

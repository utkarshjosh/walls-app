package seed

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/utkarshjoshi/walls/apps/api/internal/catalog"
	"github.com/utkarshjoshi/walls/apps/api/internal/ingest"
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

func poster(t *testing.T, tint uint8) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 960, 600))
	for y := 0; y < 600; y++ {
		for x := 0; x < 960; x++ {
			img.SetRGBA(x, y, color.RGBA{uint8(x / 4), tint, uint8(y / 3), 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

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

const sceneHTML = "<!doctype html><html><body><canvas></canvas><script>/* %s */</script></body></html>"

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
	writeScene(t, scenes, "tidal", map[string][]byte{
		"scene.html": []byte(strings.Replace(sceneHTML, "%s", "tidal", 1)),
		"poster.jpg": poster(t, 80),
		"meta.json":  []byte(`{"title":"Tidal","tags":["Abstract","warm"],"color":"#A8676A"}`),
	})
	writeScene(t, scenes, "first-light", map[string][]byte{
		"scene.html": []byte(strings.Replace(sceneHTML, "%s", "first light", 1)),
		"poster.jpg": poster(t, 160),
		"meta.json":  []byte(`{"tags":["dawn"]}`), // no title or colour: both are filled in
	})
	// Half-written folders: must be skipped without failing the run.
	writeScene(t, scenes, "no-poster", map[string][]byte{
		"scene.html": []byte(strings.Replace(sceneHTML, "%s", "x", 1)), "meta.json": []byte(`{"title":"X"}`),
	})
	writeScene(t, scenes, "partial-html", map[string][]byte{
		"scene.html": []byte("<!doctype html><html><body><canv"), "poster.jpg": poster(t, 1), "meta.json": []byte(`{"title":"Y"}`),
	})
	writeScene(t, scenes, "bad-meta", map[string][]byte{
		"scene.html": []byte(strings.Replace(sceneHTML, "%s", "z", 1)), "poster.jpg": poster(t, 2), "meta.json": []byte(`{"title":`),
	})

	opt := Options{ScenesDir: scenes, Scale: 1.0 / 24} // 160×100 renders keep the test quick
	stats, err := Run(ctx, p, opt)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Added != 32 || stats.Scenes != 2 || len(stats.ScenesSkipped) != 3 || stats.Removed != 0 {
		t.Fatalf("first run stats: %+v", stats)
	}

	items := allItems(t, p)
	if len(items) != 32 {
		t.Fatalf("catalog has %d items, want 30 pictures + 2 scenes", len(items))
	}
	byKind := map[string]int{}
	wide := 0
	for _, it := range items {
		byKind[it.Kind]++
		if it.Author != Author || it.License != License || !it.CreatedAt.Equal(Date) {
			t.Errorf("%s: attribution/date wrong: %+v", it.Title, it)
		}
		if it.Kind == catalog.KindImage && it.Width*10 >= it.Height*26 {
			wide++
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

	// Scenes: html original with the right content type, poster kept, colour and title rules.
	live, _ := p.DB.ListItems(ctx, catalog.Query{Kind: "live"})
	if len(live.Items) != 2 {
		t.Fatalf("live items = %d", len(live.Items))
	}
	var tidal, first catalog.Item
	for _, it := range live.Items {
		switch it.Title {
		case "Tidal":
			tidal = it
		case "First Light":
			first = it
		}
	}
	if tidal.ID == "" || first.ID == "" {
		t.Fatalf("scene titles: %q, %q", live.Items[0].Title, live.Items[1].Title)
	}
	if tidal.Kind != catalog.KindScene || tidal.Category != "live" || tidal.Color != "#a8676a" || !tidal.HasPoster ||
		tidal.Width != 960 || tidal.Height != 600 || tidal.OriginalExt != "html" || !slices.Equal(tidal.Tags, []string{"abstract", "warm"}) {
		t.Errorf("tidal = %+v", tidal)
	}
	if !strings.HasPrefix(first.Color, "#") || len(first.Color) != 7 {
		t.Errorf("first-light colour should be computed from the poster, got %q", first.Color)
	}
	original, preview, thumb, posterKey := ingest.Keys(tidal.ID, "html")
	f, info, err := p.Store.Open(ctx, original)
	if err != nil {
		t.Fatal(err)
	}
	html, _ := io.ReadAll(f)
	f.Close()
	if info.ContentType != "text/html; charset=utf-8" || !strings.Contains(string(html), "tidal") || tidal.Bytes != int64(len(html)) {
		t.Errorf("scene original: type %q, %d bytes", info.ContentType, len(html))
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

	// A half-written scenes folder must not retire scenes already loaded …
	stats, _ = Run(ctx, p, opt)
	if n, _ := p.DB.CountItems(ctx); n != 32 || stats.Removed != 0 {
		t.Errorf("scenes were pruned while the folder was incomplete: %d items, %+v", n, stats)
	}
	// … nor must a missing folder.
	stats, err = Run(ctx, p, Options{ScenesDir: filepath.Join(scenes, "nope"), Scale: opt.Scale})
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := p.DB.CountItems(ctx); n != 32 || stats.Scenes != 0 {
		t.Errorf("missing scenes folder: %d items, %+v", n, stats)
	}

	// Once the folder is clean, a changed scene replaces its old version and
	// a deleted one is retired; the Live pack follows.
	for _, slug := range []string{"no-poster", "partial-html", "bad-meta", "first-light"} {
		os.RemoveAll(filepath.Join(scenes, slug))
	}
	writeScene(t, scenes, "tidal", map[string][]byte{"scene.html": []byte(strings.Replace(sceneHTML, "%s", "tidal v2", 1))})
	stats, err = Run(ctx, p, opt)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Added != 1 || stats.Removed != 2 || stats.Scenes != 1 {
		t.Errorf("after editing scenes: %+v", stats)
	}
	if _, err := p.DB.GetItem(ctx, tidal.ID); !errors.Is(err, catalog.ErrNotFound) {
		t.Error("the old version of the edited scene is still in the catalog")
	}
	if _, err := p.Store.Head(ctx, original); !errors.Is(err, storage.ErrNotFound) {
		t.Error("the old scene's objects were not removed")
	}
	if pk, items, _ := p.DB.GetPack(ctx, "live"); pk.Count != 1 || items[0].Title != "Tidal" {
		t.Errorf("live pack after edit: %+v", pk)
	}

	// With no scenes left the Live pack disappears.
	os.RemoveAll(filepath.Join(scenes, "tidal"))
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

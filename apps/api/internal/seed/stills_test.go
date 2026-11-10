package seed

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/utkarshjoshi/walls/apps/api/internal/catalog"
)

// picture returns a w×h JPEG whose bytes depend on tint.
func picture(t *testing.T, w, h int, tint uint8) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, color.RGBA{uint8(x * 255 / w), tint, uint8(y * 255 / h), 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func writeStills(t *testing.T, dir string, list map[string]any, files map[string][]byte) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if list != nil {
		data, err := json.Marshal(list)
		if err != nil {
			t.Fatal(err)
		}
		files = map[string][]byte{StillsList: data}
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func stillsList(entries ...map[string]any) map[string]any {
	return map[string]any{
		"author": "Still Originals", "license": "Still Original",
		"categories": map[string]string{"silk": "Silk", "orbit": "Orbit"},
		"stills":     entries,
	}
}

func TestSeedStills(t *testing.T) {
	ctx := context.Background()
	p := newPipeline(t)
	dir := filepath.Join(t.TempDir(), "stills")
	// Small stand-ins with the shapes of the real ones: 16:10 and 3.2:1.
	writeStills(t, dir, nil, map[string][]byte{
		"ink-silk.jpg":   picture(t, 512, 320, 10),
		"pearl-silk.jpg": picture(t, 512, 320, 200),
		"long-limb.jpg":  picture(t, 768, 240, 90),
		"moon-lake.jpg":  picture(t, 512, 320, 60),
	})
	ink := map[string]any{"file": "ink-silk.jpg", "title": "Ink Silk", "category": "silk", "tags": []string{"silk", "blue"},
		"color": "#1B1F4A", "dark": true, "pack": "still-life", "width": 512, "height": 320,
		"render": map[string]any{"scene": "silk", "t": 40}}
	pearl := map[string]any{"file": "pearl-silk.jpg", "title": "Pearl Silk", "category": "silk", "tags": []string{"silk", "light"},
		"pack": "still-life", "width": 512, "height": 320}
	limb := map[string]any{"file": "long-limb.jpg", "title": "Long Limb", "category": "orbit", "tags": []string{"space", "wide"},
		"color": "#0a1524", "dark": true, "pack": "far-places", "width": 768, "height": 240}
	lake := map[string]any{"file": "moon-lake.jpg", "title": "Moon Lake", "category": "lakes", "tags": []string{"night"},
		"dark": true, "pack": "far-places", "width": 512, "height": 320, "author": "Ana Ruiz", "license": "CC BY 4.0"}
	writeStills(t, dir, stillsList(ink, pearl, limb, lake), nil)

	opt := Options{StillsDir: dir, Scale: 1.0 / 24}
	var log strings.Builder
	opt.Logf = func(format string, args ...any) { fmt.Fprintf(&log, format+"\n", args...) }
	stats, err := Run(ctx, p, opt)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Added != 34 || stats.Stills != 4 || stats.Removed != 0 {
		t.Fatalf("first run stats: %+v", stats)
	}

	stills, err := p.DB.ItemsByOrigin(ctx, originStill)
	if err != nil || len(stills) != 4 {
		t.Fatalf("stills in the catalog: %d %v", len(stills), err)
	}
	by := map[string]catalog.Item{}
	for _, it := range stills {
		by[it.Title] = it
	}
	// The size on record is the picture's own: the app refuses a picture
	// that is too small for the display, so this must never be a guess.
	for title, want := range map[string][2]int{"Ink Silk": {512, 320}, "Pearl Silk": {512, 320}, "Long Limb": {768, 240}, "Moon Lake": {512, 320}} {
		if it := by[title]; it.Width != want[0] || it.Height != want[1] {
			t.Errorf("%s is recorded as %d×%d, want %d×%d", title, it.Width, it.Height, want[0], want[1])
		}
	}
	silk := by["Ink Silk"]
	if silk.Kind != catalog.KindImage || silk.Category != "silk" || silk.Color != "#1b1f4a" || silk.HasPoster ||
		silk.OriginalExt != "jpg" || silk.OriginalType != "image/jpeg" || !slices.Equal(silk.Tags, []string{"silk", "blue"}) ||
		silk.Author != Author || silk.License != License || !silk.CreatedAt.Equal(Date) || silk.Layout != "" {
		t.Errorf("ink silk = %+v", silk)
	}
	original, err := os.ReadFile(filepath.Join(dir, "ink-silk.jpg"))
	if err != nil || silk.Bytes != int64(len(original)) {
		t.Errorf("the stored original is %d bytes, the file is %d (%v)", silk.Bytes, len(original), err)
	}
	if c := by["Pearl Silk"].Color; len(c) != 7 || !strings.HasPrefix(c, "#") {
		t.Errorf("a still with no colour should get one from its picture, got %q", c)
	}
	if lake := by["Moon Lake"]; lake.Author != "Ana Ruiz" || lake.License != "CC BY 4.0" || lake.Category != "lakes" {
		t.Errorf("a still's own credit and category must win over the defaults: %+v", lake)
	}

	// A still leads the feed, and no two neighbours share a category.
	items := allItems(t, p)
	if len(items) != 34 || items[0].ID != silk.ID {
		t.Fatalf("%d items, first is %q; want 34 with Ink Silk first", len(items), items[0].Title)
	}
	for i := 1; i < len(items); i++ {
		if items[i].Category == items[i-1].Category {
			t.Errorf("feed positions %d and %d share category %q", i-1, i, items[i].Category)
		}
	}

	names := map[string]string{}
	cats, _ := p.DB.Categories(ctx)
	for _, c := range cats {
		names[c.Slug] = c.Name
	}
	if names["silk"] != "Silk" || names["orbit"] != "Orbit" || names["lakes"] != "Lakes" || names["gradients"] != "Gradients" {
		t.Errorf("categories = %v", names)
	}

	// Packs: two of the stills' own, and the stills join the existing ones they suit.
	packTitles := func(id string) []string {
		t.Helper()
		_, items, err := p.DB.GetPack(ctx, id)
		if err != nil {
			t.Fatalf("pack %s: %v", id, err)
		}
		var out []string
		for _, it := range items {
			if it.Origin == originStill {
				out = append(out, it.Title)
			}
		}
		slices.Sort(out)
		return out
	}
	for id, want := range map[string][]string{
		"still-life":   {"Ink Silk", "Pearl Silk"},
		"far-places":   {"Long Limb", "Moon Lake"},
		"two-displays": {"Long Limb"},
		"after-dark":   {"Ink Silk", "Long Limb", "Moon Lake"},
	} {
		if got := packTitles(id); !slices.Equal(got, want) {
			t.Errorf("stills in pack %s = %v, want %v", id, got, want)
		}
	}
	packs, _ := p.DB.Packs(ctx)
	counts := map[string]int{}
	for i, pk := range packs {
		counts[pk.ID] = pk.Count
		if pk.Description == "" || strings.Count(pk.Description, ".") != 1 {
			t.Errorf("pack %s: description %q", pk.ID, pk.Description)
		}
		if i == 0 && pk.ID != "still-life" {
			t.Errorf("the first pack is %s, want still-life", pk.ID)
		}
	}
	if counts["quiet-gradients"] != 8 || counts["two-displays"] != 7 || counts["still-life"] != 2 || counts["far-places"] != 2 {
		t.Errorf("pack counts = %v", counts)
	}

	// Second run: nothing changes.
	before := allItems(t, p)
	if stats, err = Run(ctx, p, opt); err != nil || stats.Added != 0 || stats.Updated != 0 || stats.Unchanged != 34 || stats.Removed != 0 {
		t.Fatalf("second run: %+v %v", stats, err)
	}
	if !slices.EqualFunc(before, allItems(t, p), func(a, b catalog.Item) bool { return a.ID == b.ID && a.Rank == b.Rank }) {
		t.Error("re-seeding changed ids or order")
	}

	// A missing stills folder leaves the stills alone, in the catalog and in every pack.
	if stats, err = Run(ctx, p, Options{StillsDir: filepath.Join(dir, "nope"), Scale: opt.Scale}); err != nil || stats.Removed != 0 || stats.Stills != 0 {
		t.Fatalf("missing stills folder: %+v %v", stats, err)
	}
	if n, _ := p.DB.CountItems(ctx); n != 34 {
		t.Errorf("missing stills folder: %d items, want 34", n)
	}
	if got := packTitles("after-dark"); len(got) != 3 || len(packTitles("still-life")) != 2 || len(packTitles("two-displays")) != 1 {
		t.Errorf("missing stills folder changed the packs: after-dark has %v", got)
	}

	// Changing only the details keeps the id and updates the item in place.
	ink["title"], ink["tags"], ink["dark"] = "Blue Silk", []string{"satin"}, false
	writeStills(t, dir, stillsList(ink, pearl, limb, lake), nil)
	if stats, err = Run(ctx, p, opt); err != nil || stats.Updated != 1 || stats.Added != 0 || stats.Removed != 0 {
		t.Fatalf("after changing a still's details: %+v %v", stats, err)
	}
	if now, _ := p.DB.GetItem(ctx, silk.ID); now.Title != "Blue Silk" || !slices.Equal(now.Tags, []string{"satin"}) {
		t.Errorf("after changing its details: %+v", now)
	}
	if got := packTitles("after-dark"); !slices.Equal(got, []string{"Long Limb", "Moon Lake"}) {
		t.Errorf("after-dark after a still stopped being dark: %v", got)
	}

	// A still taken off the list is retired, with its files; an emptied pack goes too.
	writeStills(t, dir, stillsList(ink, pearl), nil)
	if stats, err = Run(ctx, p, opt); err != nil || stats.Removed != 2 || stats.Stills != 2 {
		t.Fatalf("after shortening the list: %+v %v", stats, err)
	}
	if _, err := p.DB.GetItem(ctx, by["Long Limb"].ID); !errors.Is(err, catalog.ErrNotFound) {
		t.Error("a still taken off the list is still in the catalog")
	}
	if _, _, err := p.DB.GetPack(ctx, "far-places"); !errors.Is(err, catalog.ErrNotFound) {
		t.Errorf("the emptied pack should be gone, got %v", err)
	}
	if pk, _, _ := p.DB.GetPack(ctx, "two-displays"); pk.Count != 6 {
		t.Errorf("two-displays has %d items after the panorama was retired, want 6", pk.Count)
	}
}

// Mistakes in the list, or a picture that does not match it, stop the run
// with a message that names the still: nothing wrong is published.
func TestSeedStillsRefusesMistakes(t *testing.T) {
	good := func() map[string]any {
		return map[string]any{"file": "ink-silk.jpg", "title": "Ink Silk", "category": "silk", "tags": []string{"silk"},
			"width": 512, "height": 320}
	}
	cases := []struct {
		name   string
		change func(e map[string]any)
		want   string
	}{
		{"size that is not the picture's", func(e map[string]any) { e["width"], e["height"] = 5120, 3200 }, "the picture is 512×320"},
		{"no size", func(e map[string]any) { delete(e, "width") }, "pixel size"},
		{"missing picture", func(e map[string]any) { e["file"] = "gone.jpg" }, "gone.jpg"},
		{"three-word title", func(e map[string]any) { e["title"] = "Ink Blue Silk" }, "one or two words"},
		{"no title", func(e map[string]any) { e["title"] = " " }, "one or two words"},
		{"no tags", func(e map[string]any) { e["tags"] = []string{} }, "no tags"},
		{"upper-case tag", func(e map[string]any) { e["tags"] = []string{"Silk"} }, "lowercase"},
		{"bad colour", func(e map[string]any) { e["color"] = "blue" }, `"color"`},
		{"bad category", func(e map[string]any) { e["category"] = "Fine Silk" }, `"category"`},
		{"unknown pack", func(e map[string]any) { e["pack"] = "nope" }, "does not define"},
		{"unsafe file name", func(e map[string]any) { e["file"] = "../ink-silk.jpg" }, `"file"`},
		{"not a JPEG", func(e map[string]any) { e["file"] = "text.jpg" }, "text.jpg"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := newPipeline(t)
			dir := t.TempDir()
			writeStills(t, dir, nil, map[string][]byte{"ink-silk.jpg": picture(t, 512, 320, 10), "text.jpg": []byte("not a picture")})
			entry := good()
			c.change(entry)
			writeStills(t, dir, stillsList(entry), nil)
			_, err := Run(context.Background(), p, Options{StillsDir: dir, Scale: 1.0 / 24})
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("got %v, want an error mentioning %q", err, c.want)
			}
			if stills, _ := p.DB.ItemsByOrigin(context.Background(), originStill); len(stills) != 0 {
				t.Errorf("%d stills were published despite the mistake", len(stills))
			}
		})
	}

	t.Run("no credit anywhere", func(t *testing.T) {
		dir := t.TempDir()
		writeStills(t, dir, map[string]any{"stills": []map[string]any{good()}}, nil)
		if _, err := ReadStills(dir); err == nil || !strings.Contains(err.Error(), "no author or licence") {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("the same picture twice", func(t *testing.T) {
		dir := t.TempDir()
		second := good()
		second["title"] = "Other Silk"
		writeStills(t, dir, stillsList(good(), second), nil)
		if _, err := ReadStills(dir); err == nil || !strings.Contains(err.Error(), "listed twice") {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("broken JSON", func(t *testing.T) {
		dir := t.TempDir()
		writeStills(t, dir, nil, map[string][]byte{StillsList: []byte("{")})
		if _, err := Run(context.Background(), newPipeline(t), Options{StillsDir: dir, Scale: 1.0 / 24}); err == nil ||
			!strings.Contains(err.Error(), "not valid JSON") {
			t.Fatalf("got %v", err)
		}
	})
}

// The stills kept in the repository are what `walls seed` publishes, so they
// are checked here: every listed picture is present, is the size the list
// says (and a size made for real displays), and the set stays within its
// budget.
func TestShippedStills(t *testing.T) {
	dir := filepath.Join("..", "..", "seed", "stills")
	list, err := ReadStills(dir)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(list.Stills); n < 12 || n > 16 {
		t.Errorf("%d stills are listed, want 12 to 16", n)
	}
	sources := filepath.Join("..", "..", "seed", "scene-sources")
	var total int64
	panoramas := 0
	for _, s := range list.Stills {
		data, err := os.ReadFile(filepath.Join(dir, s.File))
		if err != nil {
			t.Errorf("%s: %v", s.File, err)
			continue
		}
		total += int64(len(data))
		cfg, err := jpeg.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			t.Errorf("%s: not a JPEG: %v", s.File, err)
			continue
		}
		if cfg.Width != s.Width || cfg.Height != s.Height {
			t.Errorf("%s is %d×%d but the list says %d×%d", s.File, cfg.Width, cfg.Height, s.Width, s.Height)
		}
		size := [2]int{cfg.Width, cfg.Height}
		if size != [2]int{5120, 3200} && size != [2]int{7680, 2400} {
			t.Errorf("%s is %d×%d; stills are 5120×3200, or 7680×2400 for a pair of displays", s.File, cfg.Width, cfg.Height)
		}
		if isPanorama(cfg.Width, cfg.Height) {
			panoramas++
		}
		if len(data) > 2_600_000 {
			t.Errorf("%s is %.1f MB; a still stays under about 2.5 MB", s.File, float64(len(data))/1e6)
		}
		if s.Color == "" || s.Pack == "" {
			t.Errorf("%s: a shipped still names its colour and its pack", s.File)
		}
		if _, named := list.Categories[s.Category]; !named {
			t.Errorf("%s: category %q has no display name under \"categories\"", s.File, s.Category)
		}
		// Its recipe must point at a scene and a colourway that exist, so it can be drawn again.
		var entry struct {
			Render struct {
				Scene   string `json:"scene"`
				Variant string `json:"variant"`
			} `json:"render"`
		}
		raw, _ := os.ReadFile(filepath.Join(dir, StillsList))
		var doc struct {
			Stills []json.RawMessage `json:"stills"`
		}
		_ = json.Unmarshal(raw, &doc)
		for _, r := range doc.Stills {
			var probe struct {
				File string `json:"file"`
			}
			if json.Unmarshal(r, &probe) == nil && probe.File == s.File {
				_ = json.Unmarshal(r, &entry)
			}
		}
		var scene struct {
			Variants []string `json:"variants"`
		}
		src, err := os.ReadFile(filepath.Join(sources, entry.Render.Scene, "scene.json"))
		if entry.Render.Scene == "" || err != nil || json.Unmarshal(src, &scene) != nil {
			t.Errorf("%s: its \"render\" names the scene %q, which has no source", s.File, entry.Render.Scene)
		} else if v := entry.Render.Variant; v != "" && !slices.Contains(scene.Variants, v) {
			t.Errorf("%s: the scene %s has no variant %q", s.File, entry.Render.Scene, v)
		}
	}
	if panoramas < 3 {
		t.Errorf("%d panoramas, want at least 3", panoramas)
	}
	if total > 32_000_000 {
		t.Errorf("the stills take %.1f MB in all, want about 30 MB or less", float64(total)/1e6)
	}
	// Nothing in the folder is left unlisted (a picture nobody publishes).
	entries, _ := os.ReadDir(dir)
	listed := map[string]bool{StillsList: true}
	for _, s := range list.Stills {
		listed[s.File] = true
	}
	for _, e := range entries {
		if !listed[e.Name()] && !strings.HasPrefix(e.Name(), ".") {
			t.Errorf("%s is in the stills folder but not in %s", e.Name(), StillsList)
		}
	}
}

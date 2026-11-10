package ingest

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/utkarshjoshi/walls/apps/api/internal/catalog"
	"github.com/utkarshjoshi/walls/apps/api/internal/storage"
)

func newPipeline(t *testing.T) *Pipeline {
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
	return &Pipeline{DB: db, Store: store}
}

func picture(w, h int, tint uint8) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, color.RGBA{uint8(255 * x / w), uint8(255 * y / h), tint, 255})
		}
	}
	return img
}

func jpegBytes(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func pngBytes(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// hugePNG is a header-only PNG claiming w×h pixels.
func hugePNG(w, h uint32) []byte {
	var buf bytes.Buffer
	buf.WriteString("\x89PNG\r\n\x1a\n")
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:], w)
	binary.BigEndian.PutUint32(ihdr[4:], h)
	ihdr[8], ihdr[9] = 8, 2
	binary.Write(&buf, binary.BigEndian, uint32(13))
	chunk := append([]byte("IHDR"), ihdr...)
	buf.Write(chunk)
	binary.Write(&buf, binary.BigEndian, crc32.ChecksumIEEE(chunk))
	return buf.Bytes()
}

func fakeMP4(payload string) []byte {
	return append([]byte("\x00\x00\x00\x18ftypmp42\x00\x00\x00\x00mp42isom"), []byte(strings.Repeat(payload, 50))...)
}

func write(t *testing.T, dir, name string, data []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func byFile(rep Report) map[string]FileResult {
	m := map[string]FileResult{}
	for _, r := range rep.Results {
		m[r.File] = r
	}
	return m
}

func TestIngestDir(t *testing.T) {
	ctx := context.Background()
	p := newPipeline(t)
	dir := t.TempDir()

	// Good image with a sidecar that overrides the folder defaults.
	write(t, dir, "low-tide.jpg", jpegBytes(t, picture(2400, 1500, 10)))
	write(t, dir, "low-tide.json", []byte(`{"title":"Low Tide","category":"Coast Line","tags":["Warm"," calm ","warm"],
		"author":"Ana Ruiz","license":"CC BY 4.0","sourceUrl":"https://example.com/low-tide"}`))
	// Good image using manifest defaults + manifest item; title from the file name.
	write(t, dir, "misty_ridge-02.png", pngBytes(t, picture(800, 500, 60)))
	// Image saved with the wrong extension: content wins.
	write(t, dir, "actually-a-jpeg.png", jpegBytes(t, picture(640, 400, 90)))
	// Video with its poster, and a video without one.
	write(t, dir, "waves.mp4", fakeMP4("waves"))
	write(t, dir, "waves.jpg", jpegBytes(t, picture(1920, 1080, 120)))
	write(t, dir, "orphan.mp4", fakeMP4("orphan"))
	// Refused: wrong type, too many pixels, no licence.
	write(t, dir, "notes.txt", []byte("not an image"))
	write(t, dir, "anim.gif", []byte("GIF89a"+strings.Repeat("\x00", 100)))
	write(t, dir, "gigantic.png", hugePNG(10000, 7000))
	write(t, dir, "nolicense.jpg", jpegBytes(t, picture(640, 400, 200)))
	write(t, dir, "nolicense.json", []byte(`{"author":"","license":""}`)) // explicitly blanked
	write(t, dir, "badlink.jpg", jpegBytes(t, picture(640, 400, 210)))
	write(t, dir, "badlink.json", []byte(`{"sourceUrl":"javascript:alert(1)"}`))
	// Ignored entirely: hidden files and sub-folders.
	write(t, dir, ".DS_Store", []byte("junk"))
	os.Mkdir(filepath.Join(dir, "nested"), 0o755)
	write(t, filepath.Join(dir, "nested"), "deep.jpg", jpegBytes(t, picture(64, 64, 1)))

	write(t, dir, "manifest.json", []byte(`{
		"defaults": {"author":"Studio North","license":"Licensed from Studio North","category":"mountains","tags":["mist"]},
		"items": {
			"misty_ridge-02.png": {"tags":["ridge","Blue"]},
			"waves.mp4": {"title":"Slow Waves"}
		}}`))

	rep, err := p.IngestDir(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	res := byFile(rep)

	wantStatus := map[string]string{
		"low-tide.jpg": "added", "misty_ridge-02.png": "added", "actually-a-jpeg.png": "added", "waves.mp4": "added",
		"orphan.mp4": "skipped", "notes.txt": "skipped", "anim.gif": "skipped", "gigantic.png": "skipped",
		"nolicense.jpg": "skipped", "badlink.jpg": "skipped",
	}
	if len(res) != len(wantStatus) {
		t.Errorf("report has %d rows, want %d: %+v", len(res), len(wantStatus), rep.Results)
	}
	for file, want := range wantStatus {
		if res[file].Status != want {
			t.Errorf("%s: status %q (%s), want %q", file, res[file].Status, res[file].Detail, want)
		}
	}
	if rep.Imported != 4 {
		t.Errorf("imported = %d, want 4", rep.Imported)
	}
	for file, wantReason := range map[string]string{
		"orphan.mp4":    "poster",
		"notes.txt":     "unsupported file type",
		"anim.gif":      "unsupported file type",
		"gigantic.png":  "70.0 megapixels",
		"nolicense.jpg": "missing license and author",
		"badlink.jpg":   "sourceUrl",
	} {
		if !strings.Contains(res[file].Detail, wantReason) {
			t.Errorf("%s: reason %q does not mention %q", file, res[file].Detail, wantReason)
		}
	}
	if _, listed := res["waves.jpg"]; listed {
		t.Error("a video's poster must not be imported as its own wallpaper")
	}

	// Sidecar wins over defaults; tags are cleaned up.
	it, err := p.DB.GetItem(ctx, res["low-tide.jpg"].ID)
	if err != nil {
		t.Fatal(err)
	}
	if it.Title != "Low Tide" || it.Category != "coast-line" || strings.Join(it.Tags, ",") != "warm,calm" ||
		it.Author != "Ana Ruiz" || it.License != "CC BY 4.0" || it.SourceURL != "https://example.com/low-tide" ||
		it.Kind != catalog.KindImage || it.Width != 2400 || it.Height != 1500 || it.HasPoster ||
		it.OriginalExt != "jpg" || it.Origin != "ingest" || len(it.ID) != 12 {
		t.Errorf("low-tide item = %+v", it)
	}

	// Manifest defaults + per-file entry; humanised title.
	it, _ = p.DB.GetItem(ctx, res["misty_ridge-02.png"].ID)
	if it.Title != "Misty Ridge 02" || it.Category != "mountains" || strings.Join(it.Tags, ",") != "ridge,blue" ||
		it.Author != "Studio North" || it.License != "Licensed from Studio North" || it.OriginalExt != "png" ||
		it.OriginalType != "image/png" {
		t.Errorf("misty-ridge item = %+v", it)
	}

	// Extension lies are corrected from the content.
	it, _ = p.DB.GetItem(ctx, res["actually-a-jpeg.png"].ID)
	if it.OriginalExt != "jpg" || it.OriginalType != "image/jpeg" {
		t.Errorf("sniffed type = %s / %s, want jpg / image/jpeg", it.OriginalExt, it.OriginalType)
	}

	// The video: stored as-is, sized by its poster, with all four objects.
	it, _ = p.DB.GetItem(ctx, res["waves.mp4"].ID)
	if it.Kind != catalog.KindVideo || it.Title != "Slow Waves" || !it.HasPoster || it.Width != 1920 || it.Height != 1080 ||
		it.OriginalExt != "mp4" || it.OriginalType != "video/mp4" || it.Bytes != int64(len(fakeMP4("waves"))) {
		t.Errorf("video item = %+v", it)
	}
	for _, key := range ItemKeys(it) {
		info, err := p.Store.Head(ctx, key)
		if err != nil {
			t.Errorf("object %s: %v", key, err)
		}
		if strings.HasSuffix(key, ".jpg") && info.ContentType != "image/jpeg" {
			t.Errorf("object %s has content type %q", key, info.ContentType)
		}
	}
	if len(ItemKeys(it)) != 4 {
		t.Errorf("video should own 4 objects, has %v", ItemKeys(it))
	}

	// Ranks: appended in file order, after any first-party content.
	page, _ := p.DB.ListItems(ctx, catalog.Query{})
	if len(page.Items) != 4 || page.Items[0].Rank <= 1_000_000 {
		t.Errorf("ranks: %d items, first rank %d", len(page.Items), page.Items[0].Rank)
	}

	// Second run: same bytes, same ids, nothing re-uploaded.
	again, err := p.IngestDir(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	for file, r := range byFile(again) {
		if wantStatus[file] == "added" && (r.Status != "unchanged" || r.ID != res[file].ID) {
			t.Errorf("second run %s: %q id %s, want unchanged id %s", file, r.Status, r.ID, res[file].ID)
		}
	}
	if n, _ := p.DB.CountItems(ctx); n != 4 {
		t.Errorf("after second run count = %d, want 4", n)
	}

	// Editing metadata updates the row without changing the id or the rank.
	before, _ := p.DB.GetItem(ctx, res["low-tide.jpg"].ID)
	write(t, dir, "low-tide.json", []byte(`{"title":"Lower Tide","author":"Ana Ruiz","license":"CC BY 4.0"}`))
	third, _ := p.IngestDir(ctx, dir)
	if r := byFile(third)["low-tide.jpg"]; r.Status != "updated" || r.ID != before.ID {
		t.Errorf("after editing the sidecar: %+v", r)
	}
	after, _ := p.DB.GetItem(ctx, before.ID)
	if after.Title != "Lower Tide" || after.Rank != before.Rank || after.SourceURL != "" || !after.CreatedAt.Equal(before.CreatedAt) {
		t.Errorf("updated item = %+v", after)
	}

	// A missing object is restored on the next run.
	original, _, _, _ := Keys(before.ID, "jpg")
	p.Store.Delete(ctx, original)
	fourth, _ := p.IngestDir(ctx, dir)
	if r := byFile(fourth)["low-tide.jpg"]; r.Status != "updated" {
		t.Errorf("after losing an object: %+v", r)
	}
	if _, err := p.Store.Head(ctx, original); err != nil {
		t.Errorf("original was not restored: %v", err)
	}

	// Remove deletes the row and every object.
	if _, err := p.Remove(ctx, it.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := p.DB.GetItem(ctx, it.ID); !errors.Is(err, catalog.ErrNotFound) {
		t.Errorf("row still present after Remove: %v", err)
	}
	for _, key := range ItemKeys(it) {
		if _, err := p.Store.Head(ctx, key); !errors.Is(err, storage.ErrNotFound) {
			t.Errorf("object %s still present after Remove", key)
		}
	}
	if _, err := p.Remove(ctx, "ffffffffffff"); !errors.Is(err, catalog.ErrNotFound) {
		t.Errorf("Remove of a missing id: got %v", err)
	}
}

func TestIngestRefusesBeyondStorageCap(t *testing.T) {
	ctx := context.Background()
	p := newPipeline(t)
	dir := t.TempDir()
	write(t, dir, "manifest.json", []byte(`{"defaults":{"author":"A","license":"L"}}`))
	a, b := jpegBytes(t, picture(1200, 800, 10)), jpegBytes(t, picture(1200, 800, 99))
	write(t, dir, "a.jpg", a)
	write(t, dir, "b.jpg", b)

	// Room for the first item (original + derivatives) but not both.
	p.Cap = int64(len(a))*2 + int64(len(b))/2

	rep, err := p.IngestDir(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	res := byFile(rep)
	if res["a.jpg"].Status != "added" {
		t.Fatalf("a.jpg: %+v", res["a.jpg"])
	}
	if res["b.jpg"].Status != "skipped" || !strings.Contains(res["b.jpg"].Detail, "storage cap reached") ||
		!strings.Contains(res["b.jpg"].Detail, "WALLS_STORAGE_CAP_BYTES") {
		t.Fatalf("b.jpg should be refused with a clear cap message, got %+v", res["b.jpg"])
	}
	if n, _ := p.DB.CountItems(ctx); n != 1 {
		t.Errorf("count = %d, want 1", n)
	}
	total, _ := p.DB.StoredBytes(ctx, "")
	if total > p.Cap {
		t.Errorf("stored %d bytes, over the cap of %d", total, p.Cap)
	}

	// Re-running with the item already stored does not trip the cap.
	rep, _ = p.IngestDir(ctx, dir)
	if r := byFile(rep)["a.jpg"]; r.Status != "unchanged" {
		t.Errorf("re-run under cap: %+v", r)
	}
}

func TestIngestDirProblems(t *testing.T) {
	ctx := context.Background()
	p := newPipeline(t)

	if _, err := p.IngestDir(ctx, filepath.Join(t.TempDir(), "does-not-exist")); err == nil {
		t.Error("a missing folder should be an error")
	}

	// A broken manifest stops the import rather than silently dropping licences.
	dir := t.TempDir()
	write(t, dir, "a.jpg", jpegBytes(t, picture(64, 64, 1)))
	write(t, dir, "manifest.json", []byte(`{"defaults": {`))
	if _, err := p.IngestDir(ctx, dir); err == nil || !strings.Contains(err.Error(), "manifest.json") {
		t.Errorf("broken manifest: got %v", err)
	}

	// No metadata at all: every file is skipped, none imported.
	os.Remove(filepath.Join(dir, "manifest.json"))
	rep, err := p.IngestDir(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Imported != 0 || len(rep.Results) != 1 || rep.Results[0].Status != "skipped" ||
		!strings.Contains(rep.Results[0].Detail, "missing license and author") {
		t.Errorf("no metadata: %+v", rep)
	}

	// A broken sidecar only affects its own file.
	write(t, dir, "manifest.json", []byte(`{"defaults":{"author":"A","license":"L"}}`))
	write(t, dir, "b.jpg", jpegBytes(t, picture(64, 64, 2)))
	write(t, dir, "b.json", []byte(`{nope`))
	rep, _ = p.IngestDir(ctx, dir)
	res := byFile(rep)
	if res["a.jpg"].Status != "added" || res["b.jpg"].Status != "skipped" || !strings.Contains(res["b.jpg"].Detail, "b.json") {
		t.Errorf("broken sidecar: %+v", rep.Results)
	}

	// An empty folder imports nothing and reports nothing.
	rep, err = p.IngestDir(ctx, t.TempDir())
	if err != nil || rep.Imported != 0 || len(rep.Results) != 0 {
		t.Errorf("empty folder: %+v %v", rep, err)
	}
}

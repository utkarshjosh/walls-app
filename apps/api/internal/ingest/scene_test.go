package ingest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/utkarshjoshi/walls/apps/api/internal/catalog"
	"github.com/utkarshjoshi/walls/apps/api/internal/scenecheck"
)

// sceneHTML is a minimal scene that meets the standard.
func sceneHTML(marker string) string {
	return "<!doctype html>\n<html>\n<head>\n<meta charset=\"utf-8\">\n" + scenecheck.PolicyLine + "\n<title>" + marker + "</title>\n" +
		"</head>\n<body>\n<script>\nvar paused = /paused=1/.test(location.hash); // " + marker + "\n</script>\n</body>\n</html>\n"
}

func writeSceneDir(t *testing.T, dir string, html string, posterW, posterH int, meta string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, dir, "scene.html", []byte(html))
	write(t, dir, "poster.jpg", jpegBytes(t, picture(posterW, posterH, 70)))
	write(t, dir, "meta.json", []byte(meta))
}

func TestIngestScenes(t *testing.T) {
	ctx := context.Background()
	p := newPipeline(t)
	dir := t.TempDir()

	write(t, dir, "manifest.json", []byte(`{"defaults":{"author":"Studio North","license":"Licensed"}}`))
	write(t, dir, "ridge.jpg", jpegBytes(t, picture(800, 500, 10)))

	clock := sceneHTML("quiet clock")
	writeSceneDir(t, filepath.Join(dir, "quiet-clock"), clock, 1920, 1200,
		`{"standard":1,"title":"Quiet Clock","tags":["clock"],"color":"#1C2230","author":"Ana Ruiz","license":"MIT","sourceUrl":"https://example.com/clock","layout":"per-display"}`)
	// Two things wrong: it calls out, and its poster is the wrong size.
	writeSceneDir(t, filepath.Join(dir, "sneaky"), strings.Replace(sceneHTML("sneaky"), "var paused", "navigator.sendBeacon('/x'); var paused", 1), 1280, 720,
		`{"standard":1,"title":"Sneaky","author":"X","license":"Y","layout":"canvas"}`)
	// A sub-folder that is not a scene is left alone.
	os.Mkdir(filepath.Join(dir, "notes"), 0o755)
	write(t, filepath.Join(dir, "notes"), "deep.jpg", jpegBytes(t, picture(64, 64, 1)))

	rep, err := p.IngestDir(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	res := byFile(rep)
	if len(res) != 3 || rep.Imported != 2 {
		t.Fatalf("report: %d rows, %d imported: %+v", len(res), rep.Imported, rep.Results)
	}
	if res["ridge.jpg"].Status != "added" {
		t.Errorf("the image beside the scenes: %+v", res["ridge.jpg"])
	}

	// The failing scene is skipped with every reason, and nothing of it is stored.
	sneaky := res["sneaky/"]
	reasons := strings.Join(sneaky.More, "\n")
	if sneaky.Status != "skipped" || !strings.Contains(sneaky.Detail, "failed the scene check (3 problems)") ||
		!strings.Contains(reasons, "forbidden: sendBeacon") || !strings.Contains(reasons, "forbidden: navigator.sendBeacon") ||
		!strings.Contains(reasons, "poster.jpg: is 1280×720; it must be exactly 1920×1200") {
		t.Errorf("sneaky scene: %+v", sneaky)
	}
	if n, _ := p.DB.CountItems(ctx); n != 2 {
		t.Errorf("catalog has %d items, want 2", n)
	}

	// The passing scene: its own credits, its own bytes.
	got := res["quiet-clock/"]
	if got.Status != "added" {
		t.Fatalf("quiet-clock: %+v", got)
	}
	sum := sha256.Sum256([]byte(clock))
	it, err := p.DB.GetItem(ctx, got.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != hex.EncodeToString(sum[:])[:12] || it.Kind != catalog.KindScene || it.Category != "live" || it.Title != "Quiet Clock" ||
		it.Author != "Ana Ruiz" || it.License != "MIT" || it.SourceURL != "https://example.com/clock" || it.Color != "#1c2230" ||
		strings.Join(it.Tags, ",") != "clock" || it.Origin != "ingest" || it.Width != 1920 || it.Height != 1200 || !it.HasPoster ||
		it.OriginalType != "text/html; charset=utf-8" || it.Bytes != int64(len(clock)) || it.Layout != "per-display" {
		t.Errorf("scene item = %+v", it)
	}
	if len(ItemKeys(it)) != 4 {
		t.Errorf("scene should own 4 objects: %v", ItemKeys(it))
	}

	// Again: nothing changes.
	rep, _ = p.IngestDir(ctx, dir)
	if r := byFile(rep)["quiet-clock/"]; r.Status != "unchanged" || r.ID != got.ID {
		t.Errorf("second run: %+v", r)
	}

	// Changing only meta.json (same scene.html, so the same id) updates the
	// stored details in place.
	write(t, filepath.Join(dir, "quiet-clock"), "meta.json",
		[]byte(`{"standard":1,"title":"Quieter Clock","tags":["clock","calm"],"author":"Ana Ruiz","license":"MIT","sourceUrl":null,"layout":"canvas"}`))
	rep, _ = p.IngestDir(ctx, dir)
	if r := byFile(rep)["quiet-clock/"]; r.Status != "updated" || r.ID != got.ID {
		t.Errorf("after editing meta.json: %+v", r)
	}
	if now, _ := p.DB.GetItem(ctx, got.ID); now.Layout != "canvas" || now.Title != "Quieter Clock" || now.SourceURL != "" ||
		strings.Join(now.Tags, ",") != "clock,calm" || now.Rank != it.Rank || !now.CreatedAt.Equal(it.CreatedAt) {
		t.Errorf("item after editing meta.json = %+v", now)
	}
	if n, _ := p.DB.CountItems(ctx); n != 2 {
		t.Errorf("a metadata change must not add an item; count = %d", n)
	}

	// A scene folder can also be named directly.
	rep, err = p.IngestDir(ctx, filepath.Join(dir, "quiet-clock"))
	if err != nil || len(rep.Results) != 1 || rep.Results[0].Status != "unchanged" || rep.Imported != 1 {
		t.Errorf("scene folder given directly: %+v %v", rep, err)
	}
	rep, err = p.IngestDir(ctx, filepath.Join(dir, "sneaky"))
	if err != nil || len(rep.Results) != 1 || rep.Results[0].Status != "skipped" || rep.Imported != 0 {
		t.Errorf("failing scene folder given directly: %+v %v", rep, err)
	}
}

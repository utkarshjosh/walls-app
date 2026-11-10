// Package seed loads the first-party starter catalog: the procedurally drawn
// wallpapers, any live scenes found on disk, and the curated packs.
package seed

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/utkarshjoshi/walls/apps/api/internal/artgen"
	"github.com/utkarshjoshi/walls/apps/api/internal/catalog"
	"github.com/utkarshjoshi/walls/apps/api/internal/imaging"
	"github.com/utkarshjoshi/walls/apps/api/internal/ingest"
)

const (
	Author  = "Still Originals"
	License = "Still Original"

	originArt   = "seed"
	originScene = "scene"

	sceneMaxBytes = 5 << 20
)

// Date is the creation date given to first-party items, fixed so that
// re-seeding an empty catalog reproduces it exactly.
var Date = time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)

// Options controls a seed run.
type Options struct {
	ScenesDir string  // folder of live scenes; "" or missing is fine
	Scale     float64 // render size relative to full resolution; 0 or 1 = full (tests use a small value)
	Logf      func(format string, args ...any)
}

// Stats summarises a seed run.
type Stats struct {
	Added, Updated, Unchanged, Removed int
	Scenes                             int
	ScenesSkipped                      []string // "folder: reason"
	Elapsed                            time.Duration
}

// Run seeds the catalog. It is safe to run repeatedly: unchanged artwork is
// recognised by its content hash and left alone, and first-party items that
// the current generator no longer produces are removed.
func Run(ctx context.Context, p *ingest.Pipeline, opt Options) (Stats, error) {
	start := time.Now()
	logf := opt.Logf
	if logf == nil {
		logf = func(string, ...any) {}
	}
	scale := opt.Scale
	if scale <= 0 || scale > 1 {
		scale = 1
	}
	var stats Stats
	count := func(status string) {
		switch status {
		case "added":
			stats.Added++
		case "updated":
			stats.Updated++
		default:
			stats.Unchanged++
		}
	}

	for _, style := range []string{artgen.StyleMesh, artgen.StyleHorizon, artgen.StyleDunes, artgen.StyleContours, artgen.StyleAurora} {
		slug, name := artgen.Category(style)
		if err := p.DB.SetCategory(ctx, slug, name); err != nil {
			return stats, err
		}
	}
	if err := p.DB.SetCategory(ctx, "live", "Live"); err != nil {
		return stats, err
	}

	// --- Artwork -----------------------------------------------------------
	pieces := artgen.Catalog()
	ids := make([]string, len(pieces))
	errs := make([]error, len(pieces))
	results := make([]ingest.Result, len(pieces))

	// A few pieces at a time: each render already uses every core, so this
	// mostly overlaps the single-threaded JPEG encoding and resizing.
	sem := make(chan struct{}, 3)
	var wg sync.WaitGroup
	for i, piece := range pieces {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if ctx.Err() != nil {
				errs[i] = ctx.Err()
				return
			}
			fw, fh := piece.Size()
			w, h := max(16, int(float64(fw)*scale)), max(16, int(float64(fh)*scale))
			img := artgen.Render(piece, w, h)
			data, err := imaging.EncodeJPEG(img, artgen.JPEGQuality)
			if err != nil {
				errs[i] = err
				return
			}
			category, _ := artgen.Category(piece.Style)
			results[i], errs[i] = p.Publish(ctx, ingest.Asset{
				Data: data, Kind: catalog.KindImage, Ext: "jpg", ContentType: "image/jpeg", Image: img,
				Meta: ingest.Meta{
					Title: piece.Title, Category: category, Tags: piece.Tags, Author: Author, License: License,
				},
				Rank: int64(i+1) * 100, Origin: originArt, CreatedAt: Date, Grain: 1.0,
			})
			ids[i] = results[i].ID
		}()
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			return stats, fmt.Errorf("%s: %w", pieces[i].Title, err)
		}
		count(results[i].Status)
		logf("%-9s %s  %-14s %s", results[i].Status, ids[i], pieces[i].Title, ingest.HumanBytes(results[i].Item.Bytes))
	}

	// --- Live scenes -------------------------------------------------------
	sceneIDs, scenesComplete, err := seedScenes(ctx, p, opt.ScenesDir, &stats, count, logf)
	if err != nil {
		return stats, err
	}

	// --- Retire first-party items this run no longer produces -----------------
	keep := map[string]bool{}
	for _, id := range ids {
		keep[id] = true
	}
	if err := prune(ctx, p, originArt, keep, &stats, logf); err != nil {
		return stats, err
	}
	// Scenes are only retired when the scenes folder was fully readable;
	// a missing or half-written folder must not wipe scenes already loaded.
	if scenesComplete {
		keep = map[string]bool{}
		for _, id := range sceneIDs {
			keep[id] = true
		}
		if err := prune(ctx, p, originScene, keep, &stats, logf); err != nil {
			return stats, err
		}
	}

	// --- Packs -------------------------------------------------------------
	var gradients, dark, wide []string
	for i, piece := range pieces {
		if piece.Style == artgen.StyleMesh {
			gradients = append(gradients, ids[i])
		}
		if piece.Dark {
			dark = append(dark, ids[i])
		}
		if piece.Wide {
			wide = append(wide, ids[i])
		}
	}
	packs := []struct {
		pack  catalog.Pack
		items []string
	}{
		{catalog.Pack{ID: "quiet-gradients", Name: "Quiet gradients", Position: 1,
			Description: "Soft blends of colour that stay out of the way of your windows."}, gradients},
		{catalog.Pack{ID: "after-dark", Name: "After dark", Position: 2,
			Description: "Deep, low-light pieces that are easy on the eyes at night."}, dark},
		{catalog.Pack{ID: "two-displays", Name: "Two displays", Position: 3,
			Description: "Extra-wide panoramas made to stretch across a pair of screens."}, wide},
	}
	for _, pk := range packs {
		if err := p.DB.PutPack(ctx, pk.pack, pk.items); err != nil {
			return stats, err
		}
	}
	scenes, err := p.DB.ItemsByOrigin(ctx, originScene)
	if err != nil {
		return stats, err
	}
	if len(scenes) == 0 {
		if err := p.DB.DeletePack(ctx, "live"); err != nil {
			return stats, err
		}
	} else {
		live := make([]string, len(scenes))
		for i, it := range scenes {
			live[i] = it.ID
		}
		if err := p.DB.PutPack(ctx, catalog.Pack{ID: "live", Name: "Live", Position: 4,
			Description: "Gently animated scenes that keep moving on your desktop."}, live); err != nil {
			return stats, err
		}
	}

	stats.Elapsed = time.Since(start)
	return stats, nil
}

func prune(ctx context.Context, p *ingest.Pipeline, origin string, keep map[string]bool, stats *Stats,
	logf func(string, ...any)) error {
	items, err := p.DB.ItemsByOrigin(ctx, origin)
	if err != nil {
		return err
	}
	for _, it := range items {
		if keep[it.ID] {
			continue
		}
		if _, err := p.Remove(ctx, it.ID); err != nil {
			return err
		}
		stats.Removed++
		logf("%-9s %s  %s (no longer part of the seed)", "removed", it.ID, it.Title)
	}
	return nil
}

type sceneMeta struct {
	Title string   `json:"title"`
	Tags  []string `json:"tags"`
	Color string   `json:"color"`
}

var hexColour = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// seedScenes registers every complete scene folder. It returns the ids it
// registered and whether the folder was read without skipping anything.
func seedScenes(ctx context.Context, p *ingest.Pipeline, dir string, stats *Stats, count func(string),
	logf func(string, ...any)) (ids []string, complete bool, err error) {
	if dir == "" {
		return nil, false, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			logf("no scenes folder at %s; continuing without live scenes", dir)
			return nil, false, nil
		}
		logf("cannot read scenes folder %s (%v); continuing without live scenes", dir, err)
		return nil, false, nil
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	complete = true
	for n, name := range names {
		asset, reason := loadScene(filepath.Join(dir, name), name)
		if reason != "" {
			complete = false
			stats.ScenesSkipped = append(stats.ScenesSkipped, name+": "+reason)
			logf("%-9s scene %s: %s", "skipped", name, reason)
			continue
		}
		// Scenes are sprinkled through the feed: one after every sixth
		// picture, starting after the fourth.
		asset.Rank = int64(4+6*n)*100 + 50
		res, err := p.Publish(ctx, asset)
		if err != nil {
			if errors.Is(err, ingest.ErrOverCap) {
				return ids, false, err
			}
			complete = false
			stats.ScenesSkipped = append(stats.ScenesSkipped, name+": "+err.Error())
			logf("%-9s scene %s: %v", "skipped", name, err)
			continue
		}
		ids = append(ids, res.ID)
		stats.Scenes++
		count(res.Status)
		logf("%-9s %s  %-14s live scene", res.Status, res.ID, res.Item.Title)
	}
	return ids, complete, nil
}

// loadScene reads one scene folder. A non-empty reason means the folder is
// incomplete or invalid and should be skipped for now.
func loadScene(dir, name string) (ingest.Asset, string) {
	read := func(file string, limit int64) ([]byte, string) {
		path := filepath.Join(dir, file)
		info, err := os.Stat(path)
		if errors.Is(err, fs.ErrNotExist) {
			return nil, file + " is missing"
		}
		if err != nil {
			return nil, err.Error()
		}
		if info.Size() == 0 {
			return nil, file + " is empty"
		}
		if info.Size() > limit {
			return nil, fmt.Sprintf("%s is larger than %s", file, ingest.HumanBytes(limit))
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err.Error()
		}
		return data, ""
	}

	html, reason := read("scene.html", sceneMaxBytes)
	if reason != "" {
		return ingest.Asset{}, reason
	}
	posterData, reason := read("poster.jpg", imaging.MaxImageBytes)
	if reason != "" {
		return ingest.Asset{}, reason
	}
	metaData, reason := read("meta.json", 1<<20)
	if reason != "" {
		return ingest.Asset{}, reason
	}

	var meta sceneMeta
	if err := json.Unmarshal(metaData, &meta); err != nil {
		return ingest.Asset{}, "meta.json is not valid JSON yet"
	}
	// A scene being written may have a truncated file; a quick sanity check
	// on the HTML avoids registering half a document.
	lower := strings.ToLower(string(html))
	if !strings.Contains(lower, "<html") || !strings.Contains(lower, "</html>") {
		return ingest.Asset{}, "scene.html does not look like a complete HTML document yet"
	}
	poster, format, err := imaging.DecodeImage(posterData)
	if err != nil {
		return ingest.Asset{}, "poster.jpg: " + err.Error()
	}

	title := strings.TrimSpace(meta.Title)
	if title == "" {
		title = catalog.Humanize(name)
	}
	tags := []string{}
	for _, t := range meta.Tags {
		if t = strings.ToLower(strings.TrimSpace(t)); t != "" {
			tags = append(tags, t)
		}
	}
	asset := ingest.Asset{
		Data: html, Kind: catalog.KindScene, Ext: "html", ContentType: "text/html; charset=utf-8", Image: poster,
		Meta:   ingest.Meta{Title: title, Category: "live", Tags: tags, Author: Author, License: License},
		Origin: originScene, CreatedAt: Date,
	}
	if format == imaging.FormatJPEG && imaging.Upright(posterData) {
		asset.PosterJPEG = posterData
	}
	if hexColour.MatchString(meta.Color) {
		asset.Color = strings.ToLower(meta.Color)
	}
	return asset, ""
}

// Package seed loads the first-party starter catalog: the procedurally drawn
// wallpapers, the stills and live scenes found on disk, and the curated packs.
package seed

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
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
	// Author and License credit the artwork this program draws itself. Live
	// scenes carry their own credits in meta.json.
	Author  = "Still Originals"
	License = "Still Original"

	originArt   = "seed"
	originScene = "scene"
)

// Date is the creation date given to first-party items, fixed so that
// re-seeding an empty catalog reproduces it exactly.
var Date = time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)

// Options controls a seed run.
type Options struct {
	ScenesDir string  // folder of live scenes; "" or missing is fine
	StillsDir string  // folder of still pictures with their stills.json; "" or missing is fine
	Scale     float64 // render size relative to full resolution; 0 or 1 = full (tests use a small value)
	Logf      func(format string, args ...any)
}

// Stats summarises a seed run.
type Stats struct {
	Added, Updated, Unchanged, Removed int
	Scenes                             int
	Stills                             int
	ScenesSkipped                      []SkippedScene
	Elapsed                            time.Duration
}

// SkippedScene is a scene folder that failed the scene gate, with every
// reason.
type SkippedScene struct {
	Name    string
	Reasons []string
}

// Run seeds the catalog. It is safe to run repeatedly: unchanged artwork is
// recognised by its content hash and left alone, and first-party items that
// the current generator no longer produces are removed.
//
// Stills (finished pictures listed in stills.json) are published as they are.
// A mistake in the list or a picture that does not match it stops the run.
//
// Every live scene goes through the scene gate (internal/scenecheck). A
// scene that fails is skipped with its reasons and does not stop the run.
// Afterwards the catalog's seeded scenes are exactly the ones that passed: a
// scene loaded by an earlier run that is gone from the folder, or no longer
// passes, is removed.
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

	// --- Stills ------------------------------------------------------------
	stills, stillsRead, err := seedStills(ctx, p, opt.StillsDir, &stats, count, logf)
	if err != nil {
		return stats, err
	}

	// --- Live scenes -------------------------------------------------------
	sceneIDs, scenesRead, err := seedScenes(ctx, p, opt.ScenesDir, &stats, count, logf)
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
	// Stills and scenes are only retired when their folder could be read: a
	// missing folder (seed run from the wrong place, say) must not wipe what
	// is already loaded.
	if stillsRead {
		keep = map[string]bool{}
		for _, s := range stills {
			keep[s.ID] = true
		}
		if err := prune(ctx, p, originStill, keep, &stats, logf); err != nil {
			return stats, err
		}
	}
	// A scene that is present but fails the gate is retired, because a
	// published scene has to be one that passes.
	if scenesRead {
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
	// Stills join the packs they suit, and have two of their own. When the
	// list of stills could not be read, the ones loaded earlier keep their
	// places: panoramas are known by their size, the dark ones by already
	// being in "After dark", and the stills' own packs are left as they are.
	held, err := p.DB.ItemsByOrigin(ctx, originStill)
	if err != nil {
		return stats, err
	}
	if !stillsRead {
		_, before, err := p.DB.GetPack(ctx, "after-dark")
		if err != nil && !errors.Is(err, catalog.ErrNotFound) {
			return stats, err
		}
		for _, it := range before {
			if it.Origin == originStill {
				dark = append(dark, it.ID)
			}
		}
	}
	byID := map[string]seededStill{}
	for _, s := range stills {
		byID[s.ID] = s
	}
	own := map[string][]string{}
	for _, it := range held {
		if isPanorama(it.Width, it.Height) {
			wide = append(wide, it.ID)
		}
		s, listed := byID[it.ID]
		if !listed {
			continue
		}
		if s.Dark {
			dark = append(dark, it.ID)
		}
		if s.Pack != "" {
			own[s.Pack] = append(own[s.Pack], it.ID)
		}
	}
	if stillsRead {
		for _, pk := range stillPacks {
			if len(own[pk.ID]) == 0 {
				if err := p.DB.DeletePack(ctx, pk.ID); err != nil {
					return stats, err
				}
			} else if err := p.DB.PutPack(ctx, pk, own[pk.ID]); err != nil {
				return stats, err
			}
		}
	}
	packs := []struct {
		pack  catalog.Pack
		items []string
	}{
		{catalog.Pack{ID: "quiet-gradients", Name: "Quiet gradients", Position: 3,
			Description: "Soft blends of colour that stay out of the way of your windows."}, gradients},
		{catalog.Pack{ID: "after-dark", Name: "After dark", Position: 4,
			Description: "Deep, low-light pieces that are easy on the eyes at night."}, dark},
		{catalog.Pack{ID: "two-displays", Name: "Two displays", Position: 5,
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
		if err := p.DB.PutPack(ctx, catalog.Pack{ID: "live", Name: "Live", Position: 6,
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

// seedScenes registers every scene folder that passes the gate. It returns
// the ids it registered and whether the scenes folder could be read at all.
func seedScenes(ctx context.Context, p *ingest.Pipeline, dir string, stats *Stats, count func(string),
	logf func(string, ...any)) (ids []string, read bool, err error) {
	if dir == "" {
		return nil, false, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			logf("no scenes folder at %s; continuing without touching live scenes", dir)
		} else {
			logf("cannot read scenes folder %s (%v); continuing without touching live scenes", dir, err)
		}
		return nil, false, nil
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	for n, name := range names {
		asset, check := ingest.SceneAsset(filepath.Join(dir, name))
		if !check.OK() {
			stats.ScenesSkipped = append(stats.ScenesSkipped, SkippedScene{Name: name, Reasons: check.Reasons()})
			logf("%-9s scene %s failed the scene check:", "skipped", name)
			for _, reason := range check.Reasons() {
				logf("            %s", reason)
			}
			continue
		}
		asset.Origin, asset.CreatedAt = originScene, Date
		// Scenes are sprinkled through the feed: one after every sixth
		// picture, starting after the fourth.
		asset.Rank = int64(4+6*n)*100 + 50
		res, err := p.Publish(ctx, asset)
		if err != nil {
			return ids, true, fmt.Errorf("scene %s: %w", name, err)
		}
		ids = append(ids, res.ID)
		stats.Scenes++
		count(res.Status)
		logf("%-9s %s  %-14s live scene by %s", res.Status, res.ID, res.Item.Title, res.Item.Author)
	}
	return ids, true, nil
}

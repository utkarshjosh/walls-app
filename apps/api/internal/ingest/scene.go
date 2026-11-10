package ingest

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/utkarshjoshi/walls/apps/api/internal/catalog"
	"github.com/utkarshjoshi/walls/apps/api/internal/imaging"
	"github.com/utkarshjoshi/walls/apps/api/internal/scenecheck"
)

// SceneCategory is the category every live scene is filed under.
const SceneCategory = "live"

// SceneAsset runs the scene gate on a folder. If the scene passes, the
// returned Asset holds exactly the bytes that were checked, with its title,
// credits, tags and colour taken from the scene's own meta.json. The caller
// sets Origin and Rank and publishes it. If it fails, the Result says why and
// the Asset must not be used.
//
// The stored original is scene.html byte for byte, so the item's id is the
// hash of the file the gate approved.
func SceneAsset(dir string) (Asset, scenecheck.Result) {
	res := scenecheck.CheckDir(dir)
	if !res.OK() {
		return Asset{}, res
	}
	asset := Asset{
		Data: res.HTML, Kind: catalog.KindScene, Ext: "html", ContentType: "text/html; charset=utf-8",
		Image: res.Poster, Color: res.Meta.Color,
		Meta: Meta{
			Title: res.Meta.Title, Category: SceneCategory, Tags: res.Meta.Tags,
			Author: res.Meta.Author, License: res.Meta.License, SourceURL: res.Meta.SourceURL,
		},
	}
	if imaging.Upright(res.PosterData) { // otherwise the poster is re-encoded the right way up
		asset.PosterJPEG = res.PosterData
	}
	return asset, res
}

// IsSceneDir reports whether dir looks like a scene folder (it holds a
// scene.html).
func IsSceneDir(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, "scene.html"))
	return err == nil && !info.IsDir()
}

// ingestScene checks and publishes one scene folder for IngestDir.
func (p *Pipeline) ingestScene(ctx context.Context, dir, label string) FileResult {
	asset, check := SceneAsset(dir)
	if !check.OK() {
		return FileResult{File: label, Status: "skipped",
			Detail: fmt.Sprintf("failed the scene check (%d %s)", len(check.Problems), plural(len(check.Problems), "problem")),
			More:   check.Reasons()}
	}
	asset.Origin = "ingest"
	res, err := p.Publish(ctx, asset)
	if err != nil {
		status := "failed"
		if errors.Is(err, ErrOverCap) {
			status = "skipped"
		}
		return FileResult{File: label, Status: status, Detail: err.Error()}
	}
	it := res.Item
	return FileResult{File: label, Status: res.Status, ID: res.ID,
		Detail: fmt.Sprintf("%s · %s · %d×%d · %s", it.Title, it.Kind, it.Width, it.Height, HumanBytes(it.Bytes))}
}

func plural(n int, word string) string {
	if n == 1 {
		return word
	}
	return word + "s"
}

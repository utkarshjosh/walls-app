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
	"strings"

	"github.com/utkarshjoshi/walls/apps/api/internal/catalog"
	"github.com/utkarshjoshi/walls/apps/api/internal/imaging"
	"github.com/utkarshjoshi/walls/apps/api/internal/ingest"
)

// Stills are finished pictures kept in the repository (seed/stills), as
// opposed to the artwork this program draws itself. They are rendered from the
// live scenes' shaders at full resolution by the scene kit
// (seed/scene-sources/scenekit.py stills), which reads the same list.

const (
	// StillsList is the file in the stills folder that lists the pictures.
	StillsList = "stills.json"

	originStill = "still"

	// panorama is the least width-to-height ratio, times ten, of a picture
	// made for a pair of displays (the artwork's panoramas are 3.2 to 1).
	panoramaTenths = 26
)

// Still is one entry of stills.json.
type Still struct {
	File     string   `json:"file"`     // the JPEG, beside stills.json
	Title    string   `json:"title"`    // two words at most
	Category string   `json:"category"` // slug; its display name comes from "categories"
	Tags     []string `json:"tags"`
	Color    string   `json:"color"` // dominant colour, "#rrggbb"
	Dark     bool     `json:"dark"`  // belongs in the "After dark" pack
	Pack     string   `json:"pack"`  // id of the curated pack it belongs to
	Width    int      `json:"width"` // pixel size; must match the file
	Height   int      `json:"height"`
	Author   string   `json:"author"`  // "" for the list's default
	License  string   `json:"license"` // "" for the list's default
	// "render" (the scene, moment and variant the kit draws it from) is
	// read by the kit only.
}

// StillsFile is stills.json: the pictures in feed order, the credit they
// share, and the display names of their categories.
type StillsFile struct {
	Author     string            `json:"author"`
	License    string            `json:"license"`
	Categories map[string]string `json:"categories"`
	Stills     []Still           `json:"stills"`
}

// stillPacks are the curated packs made only of stills. Positions leave the
// first places in the list to them: they are what the catalog is judged by.
var stillPacks = []catalog.Pack{
	{ID: "still-life", Name: "Still life", Position: 1,
		Description: "Silk, glass and threads of light, drawn sharp enough to look into."},
	{ID: "far-places", Name: "Far places", Position: 2,
		Description: "The edge of a world at sunrise and a mountain lake through the day."},
}

var (
	stillFileRe  = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*\.jpg$`)
	stillColorRe = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
)

// ReadStills reads and checks stills.json in dir. It does not open the
// pictures. A missing folder or list returns an error matching fs.ErrNotExist.
func ReadStills(dir string) (StillsFile, error) {
	var list StillsFile
	path := filepath.Join(dir, StillsList)
	data, err := os.ReadFile(path)
	if err != nil {
		return list, err
	}
	if err := json.Unmarshal(data, &list); err != nil {
		return list, fmt.Errorf("%s is not valid JSON: %v", path, err)
	}
	packs := map[string]bool{}
	for _, pk := range stillPacks {
		packs[pk.ID] = true
	}
	files, titles := map[string]bool{}, map[string]bool{}
	for i := range list.Stills {
		s := &list.Stills[i]
		bad := func(format string, args ...any) error {
			name := s.File
			if name == "" {
				name = fmt.Sprintf("entry %d", i+1)
			}
			return fmt.Errorf("%s: %s: %s", path, name, fmt.Sprintf(format, args...))
		}
		s.Title = strings.TrimSpace(s.Title)
		if s.Author = strings.TrimSpace(s.Author); s.Author == "" {
			s.Author = strings.TrimSpace(list.Author)
		}
		if s.License = strings.TrimSpace(s.License); s.License == "" {
			s.License = strings.TrimSpace(list.License)
		}
		switch {
		case !stillFileRe.MatchString(s.File):
			return list, bad(`"file" must be a lowercase name like low-orbit-dawn.jpg`)
		case files[s.File]:
			return list, bad("is listed twice")
		case s.Title == "" || len(strings.Fields(s.Title)) > 2:
			return list, bad(`"title" must be one or two words, got %q`, s.Title)
		case titles[strings.ToLower(s.Title)]:
			return list, bad("the title %q is used twice", s.Title)
		case s.Author == "" || s.License == "":
			return list, bad("has no author or licence, and the list gives no default")
		case s.Category == "" || s.Category != catalog.Slugify(s.Category):
			return list, bad(`"category" must be a slug like "silk", got %q`, s.Category)
		case len(s.Tags) == 0:
			return list, bad("has no tags")
		case s.Color != "" && !stillColorRe.MatchString(s.Color):
			return list, bad(`"color" must look like "#1c2230", got %q`, s.Color)
		case s.Width <= 0 || s.Height <= 0:
			return list, bad(`needs its pixel size as "width" and "height"`)
		case s.Pack != "" && !packs[s.Pack]:
			return list, bad("names the pack %q, which the seed does not define", s.Pack)
		}
		for _, t := range s.Tags {
			if t == "" || t != strings.ToLower(strings.TrimSpace(t)) {
				return list, bad("tag %q must be lowercase text with no spaces around it", t)
			}
		}
		s.Color = strings.ToLower(s.Color)
		files[s.File], titles[strings.ToLower(s.Title)] = true, true
	}
	return list, nil
}

// seededStill is a still that is now in the catalog.
type seededStill struct {
	Still
	ID string
}

// seedStills publishes every picture listed in dir. It returns what it
// published and whether the list could be read at all: a missing folder or
// list is not an error (and must not retire stills loaded earlier), but a list
// that is wrong, or a picture that is missing or does not match its entry, is:
// these files are part of the repository, so a mistake in them is a mistake to
// put right, not to publish around.
func seedStills(ctx context.Context, p *ingest.Pipeline, dir string, stats *Stats, count func(string),
	logf func(string, ...any)) (out []seededStill, read bool, err error) {
	if dir == "" {
		return nil, false, nil
	}
	list, err := ReadStills(dir)
	if errors.Is(err, fs.ErrNotExist) {
		logf("no list of stills at %s; continuing without touching stills", filepath.Join(dir, StillsList))
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	for slug, name := range list.Categories {
		if err := p.DB.SetCategory(ctx, slug, name); err != nil {
			return nil, true, err
		}
	}
	for n, s := range list.Stills {
		if _, named := list.Categories[s.Category]; !named {
			if err := p.DB.SetCategory(ctx, s.Category, catalog.Humanize(s.Category)); err != nil {
				return out, true, err
			}
		}
		path := filepath.Join(dir, s.File)
		info, err := os.Stat(path)
		if err != nil {
			return out, true, fmt.Errorf("still %s: %w", s.File, err)
		}
		if info.Size() > imaging.MaxImageBytes {
			return out, true, fmt.Errorf("still %s: %w: %s", s.File, imaging.ErrTooBig, ingest.HumanBytes(info.Size()))
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return out, true, fmt.Errorf("still %s: %w", s.File, err)
		}
		img, format, err := imaging.DecodeImage(data)
		if err != nil {
			return out, true, fmt.Errorf("still %s: %w", s.File, err)
		}
		if format != imaging.FormatJPEG {
			return out, true, fmt.Errorf("still %s: is not a JPEG, whatever its name says", s.File)
		}
		// The app refuses to show a picture smaller than the display, so the
		// size on record has to be the real one: it is taken from the picture
		// itself, and a list that disagrees is refused.
		if b := img.Bounds(); b.Dx() != s.Width || b.Dy() != s.Height {
			return out, true, fmt.Errorf("still %s: the picture is %d×%d but %s says %d×%d", s.File, b.Dx(), b.Dy(),
				StillsList, s.Width, s.Height)
		}
		res, err := p.Publish(ctx, ingest.Asset{
			Data: data, Kind: catalog.KindImage, Ext: format.Ext, ContentType: format.ContentType, Image: img,
			Meta: ingest.Meta{Title: s.Title, Category: s.Category, Tags: s.Tags, Author: s.Author, License: s.License},
			// Stills lead the feed and then come after every second drawn
			// picture (those sit at 100, 200, …).
			Rank: int64(n)*200 + 20, Origin: originStill, CreatedAt: Date, Color: s.Color, Grain: 1.0,
		})
		if err != nil {
			return out, true, fmt.Errorf("still %s: %w", s.File, err)
		}
		out = append(out, seededStill{Still: s, ID: res.ID})
		stats.Stills++
		count(res.Status)
		logf("%-9s %s  %-14s still %d×%d  %s", res.Status, res.ID, s.Title, s.Width, s.Height, ingest.HumanBytes(res.Item.Bytes))
	}
	return out, true, nil
}

// isPanorama reports whether a picture of this size is one for a pair of displays.
func isPanorama(w, h int) bool { return w*10 >= h*panoramaTenths }

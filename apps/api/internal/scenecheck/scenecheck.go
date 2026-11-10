// Package scenecheck is the gate every live scene passes before it is
// published. A scene is a web page the app runs behind the desktop icons, so
// it is running code; the gate checks, from the files alone, that a scene
// cannot reach anything and follows the layout contract.
//
// The rules are written down in docs/16-live-scene-standard.md. What the gate
// cannot judge is whether a scene is calm, cheap to run or pleasant to look
// at: a person still watches it and reads its code.
package scenecheck

import (
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/utkarshjoshi/walls/apps/api/internal/imaging"
)

const (
	// Standard is the version of the scene standard this gate checks.
	Standard = 1

	// MaxSceneBytes is the publishing limit for scene.html: 512 KB.
	MaxSceneBytes = 512 * 1024

	// PosterWidth and PosterHeight are the exact size of poster.jpg.
	PosterWidth, PosterHeight = 1920, 1200

	// Sanity limits, only so a mislabelled huge file is never read whole.
	maxScanBytes = 8 << 20
	maxMetaBytes = 64 << 10
)

// The two ways a scene can lay itself out when a wallpaper covers more than
// one display. The scene's meta.json must say which it is, because the app
// cannot tell from outside and has to draw its previews the same way.
const (
	LayoutCanvas     = "canvas"      // one picture continuing across the displays
	LayoutPerDisplay = "per-display" // the whole scene drawn in every window (a clock, say)
)

// Meta is a scene's meta.json after checking.
type Meta struct {
	Title     string
	Author    string
	License   string
	Layout    string   // LayoutCanvas or LayoutPerDisplay
	SourceURL string   // "" when the scene has no source link
	Tags      []string // lowercase; never nil
	Color     string   // "#rrggbb" in lowercase, or "" if not given
}

// Result is the outcome of checking one scene folder. When OK reports true
// the remaining fields hold exactly what was checked, ready to publish; a
// caller must publish these bytes rather than read the files again.
type Result struct {
	Dir      string
	Name     string // the folder's name
	Problems []Problem

	Meta       Meta
	HTML       []byte      // scene.html, byte for byte
	PosterData []byte      // poster.jpg, byte for byte
	Poster     image.Image // poster.jpg decoded
}

// OK reports whether the scene passed every check.
func (r Result) OK() bool { return len(r.Problems) == 0 }

// Reasons returns the problems as plain lines ("scene.html:12: …").
func (r Result) Reasons() []string {
	out := make([]string, len(r.Problems))
	for i, p := range r.Problems {
		out[i] = p.String()
	}
	return out
}

// CheckDir checks the scene in dir and returns every problem found, not just
// the first.
func CheckDir(dir string) Result {
	res := Result{Dir: dir, Name: filepath.Base(filepath.Clean(dir))}
	if abs, err := filepath.Abs(dir); err == nil {
		res.Name = filepath.Base(abs)
	}
	fail := func(file, code, format string, args ...any) {
		res.Problems = append(res.Problems, Problem{File: file, Code: code, Message: fmt.Sprintf(format, args...)})
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		fail("", "missing", "%s is not a folder", dir)
		return res
	}

	// read returns the file's bytes, or nil after recording why not. A file
	// over hardLimit is not read at all.
	read := func(name string, hardLimit int64) []byte {
		path := filepath.Join(dir, name)
		info, err := os.Stat(path)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			fail(name, "missing", "is missing")
			return nil
		case err != nil:
			fail(name, "missing", "cannot be read: %v", err)
			return nil
		case !info.Mode().IsRegular():
			fail(name, "missing", "is not a regular file")
			return nil
		case info.Size() == 0:
			fail(name, "missing", "is empty")
			return nil
		case info.Size() > hardLimit:
			fail(name, "too-large", "is %s, far beyond any sensible size for this file", kb(info.Size()))
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			fail(name, "missing", "cannot be read: %v", err)
			return nil
		}
		return data
	}

	if html := read("scene.html", maxScanBytes); html != nil {
		if len(html) > MaxSceneBytes {
			fail("scene.html", "too-large", "is %s; the limit is 512 KB", kb(int64(len(html))))
		}
		res.Problems = append(res.Problems, CheckHTML(html)...)
		res.HTML = html
	}
	if data := read("poster.jpg", imaging.MaxImageBytes); data != nil {
		img, problems := CheckPoster(data)
		res.Problems = append(res.Problems, problems...)
		res.PosterData, res.Poster = data, img
	}
	if data := read("meta.json", maxMetaBytes); data != nil {
		meta, problems := CheckMeta(data)
		res.Problems = append(res.Problems, problems...)
		res.Meta = meta
	}
	return res
}

func kb(n int64) string { return fmt.Sprintf("%.0f KB", float64(n)/1024) }

// CheckPoster checks that data is a real JPEG of exactly 1920×1200.
func CheckPoster(data []byte) (image.Image, []Problem) {
	fail := func(format string, args ...any) (image.Image, []Problem) {
		return nil, []Problem{{File: "poster.jpg", Code: "poster", Message: fmt.Sprintf(format, args...)}}
	}
	if f, ok := imaging.Sniff(data); !ok || f != imaging.FormatJPEG {
		return fail("is not a JPEG (whatever its name says)")
	}
	img, _, err := imaging.DecodeImage(data)
	if err != nil {
		return fail("does not decode as a JPEG: %v", err)
	}
	if b := img.Bounds(); b.Dx() != PosterWidth || b.Dy() != PosterHeight {
		return fail("is %d×%d; it must be exactly %d×%d", b.Dx(), b.Dy(), PosterWidth, PosterHeight)
	}
	return img, nil
}

var colourRe = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// CheckMeta checks a meta.json: it must say which standard it follows, who
// made the scene under what licence, and how the scene lays itself out across
// displays. Attribution is never guessed.
func CheckMeta(data []byte) (Meta, []Problem) {
	var problems []Problem
	fail := func(format string, args ...any) {
		problems = append(problems, Problem{File: "meta.json", Code: "meta", Message: fmt.Sprintf(format, args...)})
	}
	meta := Meta{Tags: []string{}}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil || fields == nil {
		fail("is not a JSON object")
		return meta, problems
	}

	var standard float64
	if raw, ok := fields["standard"]; !ok {
		fail(`has no "standard"; it must be %d`, Standard)
	} else if json.Unmarshal(raw, &standard) != nil || standard != Standard {
		fail(`"standard" must be the number %d, not %s`, Standard, raw)
	}

	// text reads a required, non-empty string.
	text := func(name string) string {
		raw, ok := fields[name]
		var s string
		if !ok || string(raw) == "null" {
			fail(`has no %q; every scene needs one`, name)
		} else if json.Unmarshal(raw, &s) != nil {
			fail(`%q must be text`, name)
		} else if s = strings.TrimSpace(s); s == "" {
			fail(`%q is empty; every scene needs one`, name)
		}
		return s
	}
	meta.Title = text("title")
	meta.Author = text("author")
	meta.License = text("license")

	if raw, ok := fields["layout"]; !ok || string(raw) == "null" {
		fail(`has no "layout"; it must be %q (one picture across displays) or %q (drawn whole on each display)`, LayoutCanvas, LayoutPerDisplay)
	} else if json.Unmarshal(raw, &meta.Layout) != nil || (meta.Layout != LayoutCanvas && meta.Layout != LayoutPerDisplay) {
		fail(`"layout" must be %q (one picture across displays) or %q (drawn whole on each display), not %s`, LayoutCanvas, LayoutPerDisplay, raw)
		meta.Layout = ""
	}

	if raw, ok := fields["sourceUrl"]; ok && string(raw) != "null" {
		var s string
		if json.Unmarshal(raw, &s) != nil {
			fail(`"sourceUrl" must be null or an http(s) link`)
		} else if u, err := url.Parse(s); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			fail(`"sourceUrl" must be null or an http(s) link`)
		} else {
			meta.SourceURL = s
		}
	}

	if raw, ok := fields["tags"]; ok && string(raw) != "null" {
		var tags []string
		if json.Unmarshal(raw, &tags) != nil {
			fail(`"tags" must be a list of text`)
		}
		for _, t := range tags {
			if t == "" || t != strings.ToLower(t) || t != strings.TrimSpace(t) {
				fail(`tag %q must be lowercase text with no spaces around it`, t)
				continue
			}
			meta.Tags = append(meta.Tags, t)
		}
	}

	if raw, ok := fields["color"]; ok && string(raw) != "null" {
		var s string
		if json.Unmarshal(raw, &s) != nil || !colourRe.MatchString(s) {
			fail(`"color" must look like "#1c2230"`)
		} else {
			meta.Color = strings.ToLower(s)
		}
	}
	return meta, problems
}

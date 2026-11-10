package artgen

import (
	"crypto/sha256"
	"strings"
	"testing"

	"github.com/utkarshjoshi/walls/apps/api/internal/imaging"
)

func TestCatalogShape(t *testing.T) {
	pieces := Catalog()
	standard, wide := 0, 0
	slugs, titles := map[string]bool{}, map[string]bool{}
	seeds := map[uint64]bool{}
	styles := map[string]int{}
	categories := map[string]bool{"gradients": true, "horizons": true, "dunes": true, "contours": true, "aurora": true}
	for _, p := range pieces {
		if p.Wide {
			wide++
			if w, h := p.Size(); w != 7680 || h != 2400 {
				t.Errorf("%s: panorama size %d×%d", p.Slug, w, h)
			}
		} else {
			standard++
			if w, h := p.Size(); w != 3840 || h != 2400 {
				t.Errorf("%s: standard size %d×%d", p.Slug, w, h)
			}
		}
		if slugs[p.Slug] || titles[p.Title] || seeds[p.Seed] {
			t.Errorf("%s: duplicate slug, title or seed", p.Slug)
		}
		slugs[p.Slug], titles[p.Title], seeds[p.Seed] = true, true, true
		if n := len(strings.Fields(p.Title)); n < 1 || n > 2 || strings.ContainsAny(p.Title, "0123456789") {
			t.Errorf("title %q should be one or two plain words", p.Title)
		}
		if len(p.Tags) < 2 || len(p.Tags) > 3 {
			t.Errorf("%s: %d tags, want 2–3", p.Slug, len(p.Tags))
		}
		for _, tag := range p.Tags {
			if tag != strings.ToLower(tag) || strings.TrimSpace(tag) == "" {
				t.Errorf("%s: tag %q should be lowercase", p.Slug, tag)
			}
		}
		if slug, name := Category(p.Style); !categories[slug] || name == "" {
			t.Errorf("%s: style %q maps to unexpected category %q (%q)", p.Slug, p.Style, slug, name)
		}
		styles[p.Style]++
	}
	if standard != 24 || wide != 6 {
		t.Errorf("catalog has %d standard and %d wide pieces, want 24 and 6", standard, wide)
	}
	for _, s := range []string{StyleMesh, StyleHorizon, StyleDunes, StyleContours, StyleAurora} {
		if styles[s] < 4 {
			t.Errorf("style %s has only %d pieces", s, styles[s])
		}
	}
	// The feed should alternate: no two neighbours share a style.
	for i := 1; i < len(pieces); i++ {
		if pieces[i].Style == pieces[i-1].Style {
			t.Errorf("%s and %s are neighbours with the same style", pieces[i-1].Slug, pieces[i].Slug)
		}
	}
}

// TestRenderIsDeterministic renders every piece twice at a small size and
// checks the encoded bytes match exactly. Ids are content hashes, so this is
// what keeps "walls seed" from minting new ids on every run.
func TestRenderIsDeterministic(t *testing.T) {
	for _, p := range Catalog() {
		fw, fh := p.Size()
		w, h := fw/16, fh/16
		a, err := imaging.EncodeJPEG(Render(p, w, h), JPEGQuality)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := imaging.EncodeJPEG(Render(p, w, h), JPEGQuality)
		if sha256.Sum256(a) != sha256.Sum256(b) {
			t.Errorf("%s: two renders differ", p.Slug)
		}
		img := Render(p, w, h)
		if img.Bounds().Dx() != w || img.Bounds().Dy() != h {
			t.Errorf("%s: rendered %v, want %d×%d", p.Slug, img.Bounds(), w, h)
		}
		// Not a blank or blown-out frame.
		var lo, hi uint8 = 255, 0
		for i := 0; i < len(img.Pix); i += 4 {
			for c := 0; c < 3; c++ {
				lo, hi = min(lo, img.Pix[i+c]), max(hi, img.Pix[i+c])
			}
		}
		if hi-lo < 24 {
			t.Errorf("%s: picture is nearly flat (values %d–%d)", p.Slug, lo, hi)
		}
	}
}

func TestDifferentSeedsDiffer(t *testing.T) {
	pieces := Catalog()
	p := pieces[0]
	q := p
	q.Seed++
	a, _ := imaging.EncodeJPEG(Render(p, 240, 150), JPEGQuality)
	b, _ := imaging.EncodeJPEG(Render(q, 240, 150), JPEGQuality)
	if sha256.Sum256(a) == sha256.Sum256(b) {
		t.Error("changing the seed did not change the picture")
	}
}

func TestColourRoundTrip(t *testing.T) {
	for _, hex := range []string{"#000000", "#ffffff", "#b89a7c", "#1f6f7a", "#ff8a4c"} {
		c := hexRGB(hex)
		back := toLab(c).rgb()
		for _, d := range []float64{back.r - c.r, back.g - c.g, back.b - c.b} {
			if d > 1e-6 || d < -1e-6 {
				t.Errorf("%s: OKLab round trip is off by %g", hex, d)
			}
		}
	}
	// The fast sRGB encoder agrees with the exact formula to well under one 8-bit step.
	for i := 0; i <= 1000; i++ {
		v := float64(i) / 1000
		if d := srgbEncode(v) - imaging.LinearToSRGB(v); d > 1e-4 || d < -1e-4 {
			t.Fatalf("srgbEncode(%v) is off by %g", v, d)
		}
	}
}

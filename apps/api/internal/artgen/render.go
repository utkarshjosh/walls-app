// Package artgen renders the first-party starter wallpapers. Every picture is
// a pure function of its pixel position and a fixed seed, so the same piece
// always comes out byte-for-byte identical (on the same CPU architecture and
// Go release), no matter how many goroutines draw it.
package artgen

import (
	"image"
	"math"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/utkarshjoshi/walls/apps/api/internal/imaging"
)

// Output sizes and encoding quality for the shipped files.
const (
	StandardW, StandardH = 3840, 2400
	WideW, WideH         = 7680, 2400 // spans two displays
	JPEGQuality          = 90
)

// defaultGrain is the film-grain strength in 8-bit steps (standard
// deviation). About one step is invisible at desktop viewing distance but
// survives JPEG compression well enough to stop gradients from banding.
const defaultGrain = 1.0

// frame describes the canvas a shader draws on. Shaders work in "picture
// units": y runs 0 (top) to 1 (bottom) and x runs 0 to aspect, so the same
// description renders at any resolution.
type frame struct {
	w, h   int
	aspect float64 // width / height
	px     float64 // size of one pixel in picture units
}

// shader returns the linear-light colour at a point.
type shader func(x, y float64) rgb

// builder prepares a shader for a canvas, drawing any random choices from r.
type builder func(f frame, r *rng) shader

// Piece is one wallpaper in the starter set.
type Piece struct {
	Slug  string // stable identifier, also the file name stem
	Title string
	Style string // mesh | horizon | dunes | contours | aurora
	Tags  []string
	Dark  bool // belongs in the "After dark" pack
	Wide  bool // 7680×2400 panorama
	Seed  uint64

	grain float64 // 0 means defaultGrain
	build builder
}

// Size is the pixel size the piece ships at.
func (p Piece) Size() (w, h int) {
	if p.Wide {
		return WideW, WideH
	}
	return StandardW, StandardH
}

// Render draws the piece at w×h. The picture is the same composition at any
// size; only the amount of detail changes.
func Render(p Piece, w, h int) *image.RGBA {
	grain := p.grain
	if grain == 0 {
		grain = defaultGrain
	}
	return RenderGrain(p, w, h, grain)
}

// RenderGrain is Render with an explicit film-grain strength in 8-bit steps.
// It exists for tuning; shipped artwork goes through Render.
func RenderGrain(p Piece, w, h int, grain float64) *image.RGBA {
	f := frame{w: w, h: h, aspect: float64(w) / float64(h), px: 1 / float64(h)}
	shade := p.build(f, newRNG(p.Seed))
	grainSeed := p.Seed ^ 0x6772_6169_6e21

	img := image.NewRGBA(image.Rect(0, 0, w, h))
	imaging.BlueNoise(0, 0) // build the dither mask before the workers start

	// Workers take bands of rows from a shared counter. Each pixel depends
	// only on its own position, so the order of work cannot change the result.
	const band = 16
	var next atomic.Int64
	var wg sync.WaitGroup
	for range runtime.GOMAXPROCS(0) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				y0 := int(next.Add(band)) - band
				if y0 >= h {
					return
				}
				for y := y0; y < min(y0+band, h); y++ {
					fy := (float64(y) + 0.5) * f.px
					i := img.PixOffset(0, y)
					for x := 0; x < w; x++ {
						c := shade((float64(x)+0.5)*f.px, fy)
						// Grain and dither are added to the gamma-encoded
						// value, where one step looks the same size in
						// shadows and highlights.
						offset := grain*imaging.Grain(grainSeed, x, y) + imaging.BlueNoise(x, y)
						img.Pix[i] = quantize(srgbEncode(c.r)*255 + offset)
						img.Pix[i+1] = quantize(srgbEncode(c.g)*255 + offset)
						img.Pix[i+2] = quantize(srgbEncode(c.b)*255 + offset)
						img.Pix[i+3] = 255
						i += 4
					}
				}
			}
		}()
	}
	wg.Wait()
	return img
}

func quantize(v float64) uint8 {
	v = math.Floor(v)
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}

// screen brightens a with b without ever exceeding white, like projecting two
// slides onto the same wall. Used for glows.
func screen(a, b rgb) rgb {
	return rgb{1 - (1-a.r)*(1-clamp01(b.r)), 1 - (1-a.g)*(1-clamp01(b.g)), 1 - (1-a.b)*(1-clamp01(b.b))}
}

// starField scatters stars of varied brightness and tint. Each grid cell
// holds at most one star, found again from the cell's hash, so a pixel only
// needs to look at its neighbouring cells.
type starField struct {
	seed    uint64
	cell    float64 // grid size in picture units
	density float64 // chance a cell holds a star
	radius  float64 // radius of a typical star in picture units
}

func (s starField) at(x, y, px float64) rgb {
	cx, cy := int(math.Floor(x/s.cell)), int(math.Floor(y/s.cell))
	var out rgb
	for j := -1; j <= 1; j++ {
		for i := -1; i <= 1; i++ {
			h := imaging.Hash(s.seed, cx+i, cy+j)
			if unit(h) > s.density {
				continue
			}
			h2 := imaging.Hash(s.seed+1, cx+i, cy+j)
			h3 := imaging.Hash(s.seed+2, cx+i, cy+j)
			sx := (float64(cx+i) + unit(h2)) * s.cell
			sy := (float64(cy+j) + unit(h3)) * s.cell
			// Most stars are faint; a few are bright.
			b := math.Pow(unit(imaging.Hash(s.seed+3, cx+i, cy+j)), 5)
			r := s.radius * (0.75 + 1.1*b)
			gain := 1.0
			if minR := 0.55 * px; r < minR { // keep sub-pixel stars from flickering away
				gain = (r / minR) * (r / minR)
				r = minR
			}
			dx, dy := x-sx, y-sy
			d2 := dx*dx + dy*dy
			if d2 > 60*r*r {
				continue
			}
			v := (0.10 + 0.90*b) * gain * math.Exp(-d2/(2*r*r))
			v += 0.05 * b * b * math.Exp(-d2/(2*16*r*r)) // soft halo on the bright ones
			tint := unit(imaging.Hash(s.seed+4, cx+i, cy+j))
			out = out.add(rgb{0.80 + 0.20*tint, 0.88 + 0.06*tint, 1.0 - 0.22*tint}.scale(v))
		}
	}
	return out
}

// skyline precomputes a ridge height for every pixel column so shaders can
// test "is this pixel below the ridge?" with smooth, anti-aliased edges.
type skyline struct {
	h  []float64 // ridge y per column, picture units
	px float64
}

func newSkyline(f frame, height func(x float64) float64) skyline {
	s := skyline{h: make([]float64, f.w+2), px: f.px}
	for i := range s.h {
		s.h[i] = height((float64(i) - 0.5) * f.px) // one extra column each side
	}
	return s
}

// cover returns how much of the pixel at (x,y) lies below the ridge (0–1)
// and how far below the ridge the point is, in picture units.
func (s skyline) cover(x, y float64) (coverage, depth float64) {
	i := int(math.Floor(x/s.px)) + 1 // h[i] belongs to the pixel column containing x
	if i < 1 {
		i = 1
	}
	if i > len(s.h)-2 {
		i = len(s.h) - 2
	}
	h := s.h[i]
	slope := (s.h[i+1] - s.h[i-1]) / (2 * s.px)
	depth = y - h
	// Distance to the edge measured across it, in pixels, turned into a
	// one-pixel-wide ramp.
	coverage = clamp01(depth/(s.px*math.Sqrt(1+slope*slope)) + 0.5)
	return coverage, depth
}

// slope is the ridge's steepness at x (dy/dx; negative means rising to the
// right, because y grows downward).
func (s skyline) slope(x float64) float64 {
	i := int(math.Floor(x/s.px)) + 1
	if i < 1 {
		i = 1
	}
	if i > len(s.h)-2 {
		i = len(s.h) - 2
	}
	return (s.h[i+1] - s.h[i-1]) / (2 * s.px)
}

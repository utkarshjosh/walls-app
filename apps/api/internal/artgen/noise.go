package artgen

import (
	"math"

	"github.com/utkarshjoshi/walls/apps/api/internal/imaging"
)

// rng is a tiny SplitMix64 generator. The standard library's generators are
// avoided on purpose: the artwork must come out byte-identical on every run,
// and this one is fully specified right here.
type rng struct{ s uint64 }

func newRNG(seed uint64) *rng { return &rng{s: seed*0x9E3779B97F4A7C15 + 0x1234567} }

func (r *rng) next() uint64 {
	r.s += 0x9E3779B97F4A7C15
	z := r.s
	z = (z ^ (z >> 30)) * 0xBF58476D1CE4E5B9
	z = (z ^ (z >> 27)) * 0x94D049BB133111EB
	return z ^ (z >> 31)
}

// float returns a value in [0,1).
func (r *rng) float() float64 { return float64(r.next()>>11) / (1 << 53) }

// between returns a value in [a,b).
func (r *rng) between(a, b float64) float64 { return a + (b-a)*r.float() }

// sub derives an independent seed for a named part of a picture.
func (r *rng) sub() uint64 { return r.next() }

// unit turns hash bits into a float in [0,1).
func unit(h uint64) float64 { return float64(h>>11) / (1 << 53) }

var gradients = func() (g [64][2]float64) {
	for i := range g {
		a := 2 * math.Pi * (float64(i) + 0.5) / float64(len(g))
		g[i] = [2]float64{math.Cos(a), math.Sin(a)}
	}
	return
}()

// noise is 2-D gradient ("Perlin") noise: smooth, band-limited values in
// roughly [-1,1] with features about one unit across.
type noise struct{ seed uint64 }

func (n noise) at(x, y float64) float64 {
	x0, y0 := math.Floor(x), math.Floor(y)
	ix, iy := int(x0), int(y0)
	fx, fy := x-x0, y-y0
	u := fx * fx * fx * (fx*(fx*6-15) + 10)
	v := fy * fy * fy * (fy*(fy*6-15) + 10)

	g00 := gradients[imaging.Hash(n.seed, ix, iy)>>58]
	g10 := gradients[imaging.Hash(n.seed, ix+1, iy)>>58]
	g01 := gradients[imaging.Hash(n.seed, ix, iy+1)>>58]
	g11 := gradients[imaging.Hash(n.seed, ix+1, iy+1)>>58]

	n00 := g00[0]*fx + g00[1]*fy
	n10 := g10[0]*(fx-1) + g10[1]*fy
	n01 := g01[0]*fx + g01[1]*(fy-1)
	n11 := g11[0]*(fx-1) + g11[1]*(fy-1)

	return lerp(lerp(n00, n10, u), lerp(n01, n11, u), v) * math.Sqrt2
}

// fbm stacks octaves of noise ("fractal Brownian motion"): each octave is
// twice as fine and gain times as strong. The result stays in about [-1,1].
// Octaves are rotated against each other so no grid direction shows through.
func (n noise) fbm(x, y float64, octaves int, gain float64) float64 {
	const cosR, sinR = 0.8, 0.6 // a 36.87° turn between octaves
	sum, amp, norm := 0.0, 1.0, 0.0
	o := n
	for i := 0; i < octaves; i++ {
		sum += amp * o.at(x, y)
		norm += amp
		x, y = (x*cosR-y*sinR)*2+17.3, (x*sinR+y*cosR)*2+5.1
		amp *= gain
		o.seed += 0x632BE59BD9B4E019
	}
	return sum / norm
}

// ridged is fbm folded at zero so it forms sharp crests, in [0,1]. Good for
// mountain skylines.
func (n noise) ridged(x, y float64, octaves int, gain float64) float64 {
	sum, amp, norm := 0.0, 1.0, 0.0
	o := n
	for i := 0; i < octaves; i++ {
		v := 1 - math.Abs(o.at(x, y))
		sum += amp * v * v
		norm += amp
		x, y = x*2+31.7, y*2+11.9
		amp *= gain
		o.seed += 0x632BE59BD9B4E019
	}
	return sum / norm
}

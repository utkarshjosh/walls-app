package artgen

import "math"

// meshSpec describes a soft mesh gradient: a palette that flows across the
// canvas in broad, organically bent bands, like dye moving through water.
type meshSpec struct {
	stops   []string // colours in the order they appear along the flow
	angle   float64  // direction of flow in degrees (0 = left to right, 90 = top to bottom)
	stretch float64  // how much of the palette the plain tilt covers (1 = all of it, corner to corner)
	warp    float64  // how far noise pushes the bands off the plain tilt (0.15–0.45)
	freq    float64  // size of the bends: lower is broader (0.5–1.2 per picture height)
	swirl   float64  // extra bending of the bends themselves; 0 keeps things calm
	depth   float64  // light/dark variation that crosses the colour bands (0–0.08)
	sheen   float64  // soft highlight along the folds, as if lit from one side (0–0.06)
	blooms  []bloom  // optional pools of a second colour laid over the flow
}

// bloom is a large soft pool of colour blended over the flow.
type bloom struct {
	colour string
	x, y   float64 // centre: x as a fraction of the width, y of the height
	size   float64 // radius in picture heights
	amount float64 // 0–1, how strongly it replaces what is underneath
}

func mesh(s meshSpec) builder {
	return func(f frame, r *rng) shader {
		colours := newRamp(s.stops...)
		rad := s.angle * math.Pi / 180
		dx, dy := math.Cos(rad), math.Sin(rad)
		// Distance from corner to corner measured along the flow direction.
		span := f.aspect*math.Abs(dx) + math.Abs(dy)

		field := noise{r.sub()}
		wx, wy := noise{r.sub()}, noise{r.sub()}
		shade := noise{r.sub()}
		ox, oy := r.between(0, 40), r.between(0, 40)

		type pool struct {
			c            lab
			x, y, inv2r2 float64
			amount       float64
		}
		pools := make([]pool, len(s.blooms))
		for i, b := range s.blooms {
			pools[i] = pool{hexLab(b.colour), b.x * f.aspect, b.y, 1 / (2 * b.size * b.size), b.amount}
		}

		// flow is the position along the palette (0 = first stop, 1 = last)
		// before clamping.
		flow := func(x, y float64) float64 {
			u := ((x-f.aspect/2)*dx + (y-0.5)*dy) / span
			px, py := x*s.freq+ox, y*s.freq+oy
			if s.swirl != 0 {
				sx := wx.fbm(px*0.8+3.1, py*0.8, 2, 0.5)
				sy := wy.fbm(px*0.8, py*0.8+7.4, 2, 0.5)
				px, py = px+s.swirl*sx, py+s.swirl*sy
			}
			return 0.5 + u*s.stretch + s.warp*field.fbm(px, py, 3, 0.45)
		}

		const e = 0.004 // step for measuring which way the bands lean
		return func(x, y float64) rgb {
			t := flow(x, y)
			c := colours.at(t)

			for i := range pools {
				p := &pools[i]
				ddx, ddy := x-p.x, y-p.y
				c = c.mix(p.c, p.amount*math.Exp(-(ddx*ddx+ddy*ddy)*p.inv2r2))
			}

			if s.sheen != 0 {
				// Treat the flow as a gently folded surface lit from the upper
				// left: slopes facing the light lift slightly, the others sink.
				gx := (flow(x+e, y) - t) / e
				gy := (flow(x, y+e) - t) / e
				c.l += s.sheen * math.Tanh(-(gx*0.7+gy*0.7)*0.6)
			}
			if s.depth != 0 {
				c.l += s.depth * shade.fbm(x*0.6+11, y*0.6+2, 2, 0.5)
			}
			return c.rgb()
		}
	}
}

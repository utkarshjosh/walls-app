package artgen

import "math"

// contoursSpec describes a topographic map: thin lines of equal height over
// an imaginary terrain, with one accent colour on the high ground.
type contoursSpec struct {
	ground  string  // base colour
	ground2 string  // second ground tone the base drifts toward, for depth
	line    string  // line colour
	accent  string  // the one accent colour, used on the highest contours
	scale   float64 // terrain feature frequency (about 1–2 per picture height)
	step    float64 // height difference between neighbouring lines
	peak    float64 // terrain height above which lines take the accent colour
	weight  float64 // line width multiplier
}

func contours(s contoursSpec) builder {
	return func(f frame, r *rng) shader {
		ground, ground2 := hexLab(s.ground), hexLab(s.ground2)
		lineCol, accent := hexRGB(s.line), hexRGB(s.accent)
		terrain := noise{r.sub()}
		wx, wy := noise{r.sub()}, noise{r.sub()}
		tone := noise{r.sub()}
		quiet := noise{r.sub()}
		ox, oy := r.between(0, 50), r.between(0, 50)

		height := func(x, y float64) float64 {
			// A gentle warp keeps the hills from looking like plain noise.
			qx := x + 0.18*wx.at(x*0.8+3.3, y*0.8)
			qy := y + 0.18*wy.at(x*0.8, y*0.8+8.1)
			return terrain.fbm(qx*s.scale+ox, qy*s.scale+oy, 4, 0.42)
		}

		// Line widths are defined for a 2400-pixel-tall picture and scale
		// with resolution, but never drop below what a pixel grid can show.
		unitPx := float64(f.h) / 2400
		thin := math.Max(1.5*s.weight*unitPx, 0.9)
		bold := math.Max(2.6*s.weight*unitPx, 1.2)

		return func(x, y float64) rgb {
			h := height(x, y)
			// How fast the terrain changes per pixel, from two extra samples.
			gx := height(x+f.px, y) - h
			gy := height(x, y+f.px) - h
			grad := math.Sqrt(gx*gx+gy*gy) + 1e-9

			level := math.Round(h / s.step)
			distPx := math.Abs(h-level*s.step) / grad // pixels to the nearest line

			width, strength := thin, 0.74
			if math.Mod(math.Abs(level), 5) == 0 { // every fifth line is an index contour
				width, strength = bold, 1.0
			}
			alpha := strength * (1 - smoothstep(width/2-0.6, width/2+0.6, distPx))
			// Where the terrain is nearly flat the "distance" estimate breaks
			// down and would paint blobs; fade lines out there.
			alpha *= smoothstep(0.00004*2400/float64(f.h), 0.00016*2400/float64(f.h), grad)

			// Ground: a slow drift between two close tones, slightly lighter
			// on high ground.
			g := ground.mix(ground2, 0.5+0.5*tone.fbm(x*0.45+9, y*0.45, 2, 0.5))
			g.l += 0.018 * h
			c := g.rgb()
			if alpha <= 0 {
				return c
			}

			// Lines breathe: some regions are quieter than others, which
			// leaves calm space for icons and stops the pattern feeling
			// mechanical.
			alpha *= 0.5 + 0.5*smoothstep(-0.35, 0.3, quiet.fbm(x*0.6+20, y*0.6+4, 2, 0.5))

			lc := lineCol.mix(accent, smoothstep(s.peak-0.02, s.peak+0.02, h))
			return c.mix(lc, alpha)
		}
	}
}

package artgen

import "math"

// dunesSpec describes flowing layered ridges, each shaded from a lit crest
// into shadow.
type dunesSpec struct {
	sky    []string // sky colours, top to the first crest
	lit    []string // sunlit colour of the ridges, far to near
	shade  []string // shadow colour of the ridges, far to near
	layers int
	top    float64 // y of the farthest crest
	light  float64 // -1: light comes from the left, +1: from the right
	glow   string  // soft glow in the sky on the lit side; "" for none
	swell  float64 // crest height multiplier
}

func dunes(s dunesSpec) builder {
	return func(f frame, r *rng) shader {
		sky := newRamp(s.sky...)
		lit, shade := newRamp(s.lit...), newRamp(s.shade...)
		var glowCol rgb
		if s.glow != "" {
			glowCol = hexRGB(s.glow)
		}
		glowX := f.aspect * 0.22
		if s.light > 0 {
			glowX = f.aspect * 0.78
		}

		type layer struct {
			line       skyline
			lit, shade lab
			t          float64
			depthScale float64 // how far below the crest the shading runs
		}
		layers := make([]layer, s.layers)
		phase1, phase2 := r.between(0, 2*math.Pi), r.between(0, 2*math.Pi)
		drift1, drift2 := r.between(1.1, 1.9), r.between(2.0, 3.1)
		nz := noise{r.sub()}
		bases := make([]float64, s.layers+1)
		for i := range bases {
			t := float64(i) / float64(s.layers-1)
			bases[i] = lerp(s.top, 0.99, math.Pow(t, 1.3)) // layers spread apart toward the viewer
		}
		for i := range layers {
			t := float64(i) / float64(s.layers-1)
			amp := lerp(0.028, 0.085, t) * s.swell
			wave := 2 * math.Pi / lerp(1.0, 2.3, t) // nearer dunes are broader
			fi := float64(i)
			line := newSkyline(f, func(x float64) float64 {
				// Two sines whose phase drifts from layer to layer make the
				// crests weave past each other; slow noise breaks the rhythm.
				v := 0.62*math.Sin(wave*x+phase1+fi*drift1) +
					0.30*math.Sin(wave*1.83*x+phase2+fi*drift2) +
					0.75*nz.fbm(x*0.55+fi*13.1, fi*7.3, 3, 0.5)
				return bases[i] + amp*v
			})
			layers[i] = layer{
				line: line, lit: lit.at(t), shade: shade.at(t), t: t,
				depthScale: math.Max(bases[i+1]-bases[i], 0.03) * 1.6,
			}
		}

		return func(x, y float64) rgb {
			c := sky.at(y / (s.top + 0.03)).rgb()
			if s.glow != "" {
				d := math.Hypot((x-glowX)*0.7, y-s.top*0.95)
				c = screen(c, glowCol.scale(0.55*math.Exp(-d*d/(2*0.22*0.22))+0.25*math.Exp(-d/0.5)))
			}

			for i := range layers {
				ly := &layers[i]
				cov, depth := ly.line.cover(x, y)
				if cov <= 0 {
					continue
				}
				depth = math.Max(depth, 0)
				// Which way does this part of the ridge face? The test point
				// slides sideways with depth, so the boundary between light
				// and shadow leans like a real slip face instead of dropping
				// straight down.
				slope := ly.line.slope(x - s.light*depth*0.9)
				facing := 0.5 + 0.5*math.Tanh(9*slope*s.light)
				// Light is strongest at the crest and dies away below it.
				fall := 1 - math.Exp(-depth/ly.depthScale)
				lightAmt := facing*(1-0.75*fall) + 0.16*(1-fall)
				// A thin bright rim right on the crest sells the edge.
				rim := math.Exp(-depth/(0.006+0.004*ly.t)) * (0.25 + 0.5*facing)
				lc := ly.shade.mix(ly.lit, clamp01(lightAmt+0.35*rim))
				c = c.mix(lc.rgb(), cov)
			}
			return c
		}
	}
}

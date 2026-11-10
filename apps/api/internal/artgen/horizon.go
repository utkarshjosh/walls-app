package artgen

import "math"

// horizonSpec describes an atmospheric landscape: a sky gradient, a soft sun
// or moon, and a few hazy ridgelines fading with distance.
type horizonSpec struct {
	sky     []string // sky colours from the top of the picture down to the horizon
	sun     string   // colour of the glow around the sun or moon
	disc    string   // colour of the disc itself; "" for glow only
	sunX    float64  // position across the width, 0–1
	sunY    float64  // position down the height, 0–1
	sunR    float64  // disc radius in picture heights
	glow    float64  // glow strength, about 0.2–1
	far     string   // colour of the most distant ridge
	near    string   // colour of the closest ridge
	layers  int      // number of ridgelines
	horizon float64  // y of the farthest ridge
	rough   float64  // ridge height multiplier
	mist    float64  // how strongly valleys fill with haze, 0–1
	stars   bool
}

func horizon(s horizonSpec) builder {
	return func(f frame, r *rng) shader {
		sky := newRamp(s.sky...)
		sunCol := hexRGB(s.sun)
		var discCol rgb
		if s.disc != "" {
			discCol = hexRGB(s.disc)
		}
		sunX, sunY := s.sunX*f.aspect, s.sunY
		far, near := hexLab(s.far), hexLab(s.near)
		hazeLin := sky.at(1).rgb()

		type ridge struct {
			line skyline
			col  lab     // base colour
			fog  lab     // colour its lower slopes fade into
			t    float64 // 0 = farthest, 1 = nearest
		}
		ridges := make([]ridge, s.layers)
		for i := range ridges {
			t := 0.0
			if s.layers > 1 {
				t = float64(i) / float64(s.layers-1)
			}
			base := lerp(s.horizon, 0.93, math.Pow(t, 1.2))
			amp := lerp(0.05, 0.13, t) * s.rough
			freq := lerp(2.1, 0.95, t) // distant ranges look finer
			nz := noise{r.sub()}
			off := r.between(0, 100)
			line := newSkyline(f, func(x float64) float64 {
				rolling := nz.fbm(x*freq+off, float64(i)*9.7, 5, 0.5)
				peaks := nz.ridged(x*freq*0.6+off*0.37, float64(i)*3.1+50, 4, 0.5) - 0.55
				return base - amp*(0.9*rolling+1.1*peaks)
			})
			col := far.mix(near, math.Pow(t, 0.85))
			ridges[i] = ridge{line: line, col: col, fog: col.mix(far, 0.6).mix(sky.at(1), 0.35), t: t}
		}

		stars := starField{seed: r.sub(), cell: 0.022, density: 0.5, radius: 0.00042}

		return func(x, y float64) rgb {
			c := sky.at(y / s.horizon).rgb()

			if s.stars {
				fade := 1 - smoothstep(s.horizon-0.45, s.horizon, y) // stars thin out toward the horizon
				if fade > 0 {
					c = c.add(stars.at(x, y, f.px).scale(fade))
				}
			}

			// Glow: a tight bloom plus a wide, soft one, slightly wider than tall.
			dx, dy := x-sunX, y-sunY
			d := math.Hypot(dx, dy)
			dg := math.Hypot(dx*0.8, dy)
			g := s.glow * (0.75*math.Exp(-dg*dg/(2*0.085*0.085)) + 0.42*math.Exp(-dg/0.32))
			// A band of warmth along the horizon under the sun.
			g += s.glow * 0.22 * math.Exp(-math.Pow((y-s.horizon)/0.11, 2)) * math.Exp(-math.Pow(dx/0.95, 2))
			c = screen(c, sunCol.scale(g))

			if s.disc != "" {
				edge := math.Max(1.2*f.px, 0.0018)
				c = c.mix(discCol, 1-smoothstep(s.sunR-edge, s.sunR+edge, d))
			}

			for i := range ridges {
				rg := &ridges[i]
				cov, depth := rg.line.cover(x, y)
				if cov <= 0 {
					continue
				}
				// Lower slopes sink into valley haze.
				fog := s.mist * (1 - 0.55*rg.t) * (1 - math.Exp(-math.Max(depth, 0)/0.07))
				rc := rg.col.mix(rg.fog, fog).rgb()
				// Distant ridges catch some of the sun's colour near it.
				warm := s.glow * 0.24 * (1 - rg.t) * (1 - rg.t) * math.Exp(-math.Pow(dx/0.6, 2))
				rc = screen(rc, sunCol.mix(hazeLin, 0.3).scale(warm))
				c = c.mix(rc, cov)
			}
			return c
		}
	}
}

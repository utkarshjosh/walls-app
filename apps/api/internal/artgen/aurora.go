package artgen

import "math"

// auroraSpec describes a night sky with stars and curtains of light.
type auroraSpec struct {
	sky      []string // sky colours, top to bottom
	curtain  []string // curtain colours from the bright lower hem up to the faint top
	ground   string   // silhouette along the bottom
	curtains int      // number of ribbons
	power    float64  // overall brightness of the curtains
	sweep    float64  // how steeply the ribbons climb across the picture (-1…1)
	reach    float64  // ray length multiplier (1 = about a third of the picture)
}

type auroraRibbon struct {
	base, tilt float64 // lower hem: base + tilt·(x - centre) …
	wander     float64 // … plus this much slow rise and fall
	height     float64 // longest rays, picture heights
	power      float64
	off        float64
	path       noise // shape of the hem
	swell      noise // where the ribbon brightens and fades out
	rays       noise // fine vertical structure
}

func aurora(s auroraSpec) builder {
	return func(f frame, r *rng) shader {
		sky := newRamp(s.sky...)
		cols := make([]rgb, len(s.curtain))
		for i, h := range s.curtain {
			cols[i] = hexRGB(h)
		}
		ground := hexRGB(s.ground)
		reach := s.reach
		if reach == 0 {
			reach = 1
		}

		ribbons := make([]auroraRibbon, s.curtains)
		for i := range ribbons {
			// Ribbons are stacked from the upper sky down toward the horizon;
			// the lower ones read as closer and are a little brighter.
			t := (float64(i) + 0.5) / float64(s.curtains)
			ribbons[i] = auroraRibbon{
				base:   lerp(0.50, 0.72, t) + r.between(-0.03, 0.03),
				tilt:   s.sweep * 0.34 / f.aspect * r.between(0.7, 1.3),
				wander: r.between(0.10, 0.16),
				height: reach * r.between(0.36, 0.50),
				power:  s.power * lerp(0.75, 1.1, t) * r.between(0.85, 1.1),
				off:    r.between(0, 200),
				path:   noise{r.sub()},
				swell:  noise{r.sub()},
				rays:   noise{r.sub()},
			}
		}

		stars := starField{seed: r.sub(), cell: 0.03, density: 0.55, radius: 0.00040}
		dust := starField{seed: r.sub(), cell: 0.011, density: 0.45, radius: 0.00024}
		hills := noise{r.sub()}
		horizonLine := newSkyline(f, func(x float64) float64 {
			return 0.93 - 0.035*hills.fbm(x*0.9+40, 2.5, 4, 0.5) - 0.02*hills.ridged(x*1.7, 9.1, 3, 0.5)
		})

		// colourAt blends along the curtain's height in linear light.
		colourAt := func(t float64) rgb {
			p := clamp01(t) * float64(len(cols)-1)
			i := int(p)
			if i >= len(cols)-1 {
				return cols[len(cols)-1]
			}
			return cols[i].mix(cols[i+1], smoothstep(0, 1, p-float64(i)))
		}

		return func(x, y float64) rgb {
			c := sky.at(y).rgb()

			var light rgb
			for i := range ribbons {
				rb := &ribbons[i]
				hem := rb.base + rb.tilt*(x-f.aspect/2) +
					rb.wander*rb.path.fbm(x*0.36+rb.off, 0.5, 2, 0.5) +
					0.010*rb.path.at(x*2.2+rb.off, 9.3)
				above := hem - y // how far this point is above the hem

				// Rays lean a touch and differ in length, which gives the top
				// of the curtain its ragged edge.
				rx := x + 0.04*above
				fine := rb.rays.fbm(rx*34+rb.off, y*0.5, 3, 0.55)    // about -0.6…0.6
				medium := rb.rays.fbm(rx*5.5+rb.off+70, 4.4, 2, 0.5) // slower variation
				length := rb.height * (0.70 + 0.50*medium) * (0.92 + 0.16*fine)
				t := above / length // 0 at the hem, 1 at the tip of the ray

				// Brightness along the ribbon: it swells and dies away in
				// places, so ribbons have beginnings and ends.
				swell := smoothstep(-0.18, 0.42, rb.swell.fbm(x*0.5+rb.off+31, 1.3, 2, 0.5))
				swell *= 0.8 + 0.4*medium

				// Brightest at the hem; a slow fade up the rays, a quick but
				// soft one below.
				var profile float64
				if above >= 0 {
					profile = math.Exp(-2.3*t) * (1 - smoothstep(0.35, 1.0, t))
				} else {
					profile = math.Exp(above / 0.013)
				}
				streak := 0.86 + 0.34*fine + 0.10*rb.rays.at(rx*90+rb.off, 2.2)
				v := rb.power * swell * streak * profile

				// Faint, ray-less bloom hugging the ribbon, mostly above it.
				d := above/rb.height - 0.15
				bloom := rb.power * swell * 0.06 * math.Exp(-d*d/(2*0.32*0.32))
				if v+bloom > 0 {
					light = light.add(colourAt(above / (rb.height * 1.05)).scale(v + bloom))
				}
			}

			// Stars: a sparse set of visible ones over a dusting of very faint
			// ones. Both dim behind bright curtains.
			lum := light.luma()
			st := stars.at(x, y, f.px).add(dust.at(x, y, f.px).scale(0.35))
			c = c.add(st.scale(1 / (1 + 9*lum)))

			// Soft roll-off so the brightest folds never clip to flat colour.
			c = rgb{
				c.r + 1 - math.Exp(-light.r),
				c.g + 1 - math.Exp(-light.g),
				c.b + 1 - math.Exp(-light.b),
			}

			if cov, _ := horizonLine.cover(x, y); cov > 0 {
				c = c.mix(ground, cov)
			}
			return c
		}
	}
}

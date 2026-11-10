package artgen

import (
	"math"
	"strconv"
)

// rgb is a colour in linear light (not gamma-encoded). All blending of light
// (glows, coverage, haze) happens here.
type rgb struct{ r, g, b float64 }

func (c rgb) add(o rgb) rgb       { return rgb{c.r + o.r, c.g + o.g, c.b + o.b} }
func (c rgb) scale(k float64) rgb { return rgb{c.r * k, c.g * k, c.b * k} }
func (c rgb) mix(o rgb, t float64) rgb {
	return rgb{c.r + (o.r-c.r)*t, c.g + (o.g-c.g)*t, c.b + (o.b-c.b)*t}
}
func (c rgb) luma() float64 { return 0.2126*c.r + 0.7152*c.g + 0.0722*c.b }

// lab is a colour in OKLab, a space where equal steps look equally big to the
// eye. Gradients between two colours are interpolated here so they pass
// through clean in-between hues instead of muddy or over-dark ones.
type lab struct{ l, a, b float64 }

func (c lab) mix(o lab, t float64) lab {
	return lab{c.l + (o.l-c.l)*t, c.a + (o.a-c.a)*t, c.b + (o.b-c.b)*t}
}

func srgbDecode(v float64) float64 {
	if v <= 0.04045 {
		return v / 12.92
	}
	return math.Pow((v+0.055)/1.055, 2.4)
}

// hexRGB parses "#rrggbb" (sRGB) into linear light. Palettes are written by
// hand in this file set, so a malformed one is a programming error.
func hexRGB(s string) rgb {
	if len(s) != 7 || s[0] != '#' {
		panic("artgen: bad colour " + s)
	}
	v, err := strconv.ParseUint(s[1:], 16, 32)
	if err != nil {
		panic("artgen: bad colour " + s)
	}
	return rgb{
		srgbDecode(float64(v>>16&0xff) / 255),
		srgbDecode(float64(v>>8&0xff) / 255),
		srgbDecode(float64(v&0xff) / 255),
	}
}

func hexLab(s string) lab { return toLab(hexRGB(s)) }

func hexLabs(ss []string) []lab {
	out := make([]lab, len(ss))
	for i, s := range ss {
		out[i] = hexLab(s)
	}
	return out
}

func toLab(c rgb) lab {
	l := math.Cbrt(0.4122214708*c.r + 0.5363325363*c.g + 0.0514459929*c.b)
	m := math.Cbrt(0.2119034982*c.r + 0.6806995451*c.g + 0.1073969566*c.b)
	s := math.Cbrt(0.0883024619*c.r + 0.2817188376*c.g + 0.6299787005*c.b)
	return lab{
		0.2104542553*l + 0.7936177850*m - 0.0040720468*s,
		1.9779984951*l - 2.4285922050*m + 0.4505937099*s,
		0.0259040371*l + 0.7827717662*m - 0.8086757660*s,
	}
}

func (c lab) rgb() rgb {
	l := c.l + 0.3963377774*c.a + 0.2158037573*c.b
	m := c.l - 0.1055613458*c.a - 0.0638541728*c.b
	s := c.l - 0.0894841775*c.a - 1.2914855480*c.b
	l, m, s = l*l*l, m*m*m, s*s*s
	return rgb{
		4.0767416621*l - 3.3077115913*m + 0.2309699292*s,
		-1.2684380046*l + 2.6097574011*m - 0.3413193965*s,
		-0.0041960863*l - 0.7034186147*m + 1.7076147010*s,
	}
}

// ramp is a smooth gradient through evenly spaced colour stops. It uses a
// Catmull-Rom spline rather than straight segments, because straight segments
// leave a faint visible crease at every stop in a large sky gradient.
type ramp []lab

func newRamp(hex ...string) ramp { return ramp(hexLabs(hex)) }

func (r ramp) at(t float64) lab {
	n := len(r)
	if n == 1 {
		return r[0]
	}
	t = clamp01(t) * float64(n-1)
	i := int(t)
	if i >= n-1 {
		i = n - 2
	}
	f := t - float64(i)
	p0, p1, p2, p3 := r[max(i-1, 0)], r[i], r[i+1], r[min(i+2, n-1)]
	return lab{
		catmull(p0.l, p1.l, p2.l, p3.l, f),
		catmull(p0.a, p1.a, p2.a, p3.a, f),
		catmull(p0.b, p1.b, p2.b, p3.b, f),
	}
}

func catmull(p0, p1, p2, p3, t float64) float64 {
	return 0.5 * (2*p1 + (p2-p0)*t + (2*p0-5*p1+4*p2-p3)*t*t + (3*p1-p0-3*p2+p3)*t*t*t)
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func lerp(a, b, t float64) float64 { return a + (b-a)*t }

// smoothstep eases from 0 at a to 1 at b.
func smoothstep(a, b, v float64) float64 {
	t := clamp01((v - a) / (b - a))
	return t * t * (3 - 2*t)
}

// linear→sRGB lookup with interpolation; exact enough (error far below one
// 8-bit step) and much faster than calling Pow three times per pixel.
const encodeLUTSize = 4096

var encodeLUT = func() (t [encodeLUTSize + 2]float32) {
	for i := range t {
		v := float64(i) / encodeLUTSize
		if v <= 0.0031308 {
			t[i] = float32(v * 12.92)
		} else {
			t[i] = float32(1.055*math.Pow(v, 1/2.4) - 0.055)
		}
	}
	return
}()

// srgbEncode converts linear light to an sRGB value in [0,1], clamping
// out-of-range input.
func srgbEncode(v float64) float64 {
	if !(v > 0) { // also catches NaN
		return 0
	}
	if v >= 1 {
		return 1
	}
	f := v * encodeLUTSize
	i := int(f)
	a, b := float64(encodeLUT[i]), float64(encodeLUT[i+1])
	return a + (b-a)*(f-float64(i))
}

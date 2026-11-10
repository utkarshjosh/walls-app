package imaging

import "sync"

// Dithering and grain helpers shared by the derivative pipeline and the
// procedural art generator.
//
// Why both exist: rounding a smooth gradient to 8 bits leaves visible steps
// ("banding"). Dithering nudges each pixel by less than one step in a
// well-spread pattern so the steps average out. JPEG compression throws most
// of that tiny pattern away again, so seed artwork also gets a faint film
// grain that is just strong enough to survive compression.

const blueNoiseSize = 64

var (
	blueNoiseOnce sync.Once
	blueNoiseMask [blueNoiseSize * blueNoiseSize]float32
)

// BlueNoise returns a threshold in [0,1) for pixel (x,y) from a tiling 64×64
// blue-noise mask. Add it before truncating to an integer: floor(v + mask).
func BlueNoise(x, y int) float64 {
	blueNoiseOnce.Do(buildBlueNoise)
	return float64(blueNoiseMask[(y&(blueNoiseSize-1))*blueNoiseSize+(x&(blueNoiseSize-1))])
}

// buildBlueNoise makes the mask with the void-and-cluster method (Ulichney,
// 1993): repeatedly find the most crowded and the emptiest spots of a dot
// pattern and rank pixels by the order dots are removed and added. Energy is
// kept in whole numbers so the mask is identical on every machine.
func buildBlueNoise() {
	const size = blueNoiseSize
	const n = size * size
	const radius = 6

	// Gaussian bump (sigma 1.5 px) as integer weights.
	var kernel [2*radius + 1][2*radius + 1]int64
	for dy := -radius; dy <= radius; dy++ {
		for dx := -radius; dx <= radius; dx++ {
			// exp(-(d²)/(2·1.5²)) scaled to 1e6, from a small table of
			// powers so no floating point is involved.
			kernel[dy+radius][dx+radius] = gaussInt(dx*dx + dy*dy)
		}
	}

	energy := make([]int64, n)
	on := make([]bool, n)
	spread := func(i int, sign int64) {
		x0, y0 := i%size, i/size
		for dy := -radius; dy <= radius; dy++ {
			row := ((y0 + dy + size) % size) * size
			for dx := -radius; dx <= radius; dx++ {
				energy[row+(x0+dx+size)%size] += sign * kernel[dy+radius][dx+radius]
			}
		}
	}
	// pick returns the "on" (or "off") pixel with the highest (or lowest)
	// energy; the first one wins ties, which keeps the result deterministic.
	pick := func(state bool, highest bool) int {
		best := -1
		for i := 0; i < n; i++ {
			if on[i] != state {
				continue
			}
			if best < 0 || (highest && energy[i] > energy[best]) || (!highest && energy[i] < energy[best]) {
				best = i
			}
		}
		return best
	}

	// Start from a sparse pseudo-random pattern (about 10% of pixels).
	state := uint64(0x5DEECE66D)
	ones := 0
	for ones < n/10 {
		state = state*6364136223846793005 + 1442695040888963407
		i := int((state >> 33) % n)
		if !on[i] {
			on[i] = true
			spread(i, 1)
			ones++
		}
	}
	// Even it out: move the most crowded dot into the biggest gap until
	// nothing changes.
	for {
		c := pick(true, true)
		on[c] = false
		spread(c, -1)
		v := pick(false, false)
		on[v] = true
		spread(v, 1)
		if v == c {
			break
		}
	}

	rank := make([]int, n)
	protoOn := append([]bool(nil), on...)
	protoEnergy := append([]int64(nil), energy...)

	// Phase 1: remove dots from the most crowded spot; last removed ranks lowest.
	for k := ones - 1; k >= 0; k-- {
		c := pick(true, true)
		on[c] = false
		spread(c, -1)
		rank[c] = k
	}
	// Phase 2: back to the prototype, then fill the biggest gaps up to half.
	copy(on, protoOn)
	copy(energy, protoEnergy)
	for k := ones; k < n/2; k++ {
		v := pick(false, false)
		on[v] = true
		spread(v, 1)
		rank[v] = k
	}
	// Phase 3: now the empty pixels are the minority; fill the most crowded
	// empty spot each time. Energy is rebuilt to measure empties.
	for i := range energy {
		energy[i] = 0
	}
	for i := 0; i < n; i++ {
		if !on[i] {
			spread(i, 1)
		}
	}
	for k := n / 2; k < n; k++ {
		c := pick(false, true)
		on[c] = true
		spread(c, -1)
		rank[c] = k
	}

	for i, r := range rank {
		blueNoiseMask[i] = (float32(r) + 0.5) / n
	}
}

// gaussInt is round(1e6 · exp(-d2 / 4.5)) for the squared distances the
// kernel uses, computed by repeated multiplication of exact constants.
func gaussInt(d2 int) int64 {
	// exp(-1/4.5) as a 1e12 fixed-point fraction.
	const step = 800737402917 // exp(-0.2222…) · 1e12
	v := int64(1_000_000_000_000)
	for i := 0; i < d2; i++ {
		v = v / 1_000_000 * step / 1_000_000
	}
	return (v + 500_000) / 1_000_000
}

// Hash mixes a seed and a pixel position into 64 well-scrambled bits.
func Hash(seed uint64, x, y int) uint64 {
	h := seed + uint64(int64(x))*0x9E3779B97F4A7C15 + uint64(int64(y))*0xC2B2AE3D27D4EB4F
	h ^= h >> 32
	h *= 0xD6E8FEB86659FD93
	h ^= h >> 32
	h *= 0xD6E8FEB86659FD93
	h ^= h >> 32
	return h
}

// Grain returns soft noise for pixel (x,y) with mean 0 and standard
// deviation 1. It is white noise passed through a 3×3 tent filter, so it
// clumps slightly like film grain instead of sparkling per pixel, which both
// looks better and compresses better.
func Grain(seed uint64, x, y int) float64 {
	var s float64
	for j := -1; j <= 1; j++ {
		wj := 2 - abs(j)
		for i := -1; i <= 1; i++ {
			u := float64(Hash(seed, x+i, y+j)>>11) / (1 << 53) // [0,1)
			s += float64(wj*(2-abs(i))) * (u - 0.5)
		}
	}
	return s * 0.5773502691896258 // 1/sqrt(3): tent weights give variance 3
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

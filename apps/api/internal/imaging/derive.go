package imaging

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"math"

	"golang.org/x/image/draw"
)

// Derivative sizes and quality.
const (
	PreviewSize       = 1920 // longest side of the preview, never upscaled
	ThumbSize         = 640  // longest side of the thumbnail
	DerivativeQuality = 84
)

// Derivatives are the JPEGs made from an original image, plus its average
// colour (used by the UI as a placeholder while the tile loads).
type Derivatives struct {
	Preview []byte
	Thumb   []byte
	Color   string // "#rrggbb"
}

// DeriveOptions tunes derivative generation.
type DeriveOptions struct {
	// Grain adds faint film grain to the downscaled images, measured in
	// 8-bit steps (standard deviation). The seed artwork uses about 1 so its
	// smooth gradients do not show bands after JPEG compression; photographs
	// should leave it at 0.
	Grain float64
	Seed  uint64 // makes the grain pattern repeatable
}

// Fit scales w×h to fit inside a size×size box without ever enlarging.
func Fit(w, h, size int) (int, int) {
	longest := max(w, h)
	if longest <= size {
		return w, h
	}
	s := float64(size) / float64(longest)
	return max(1, int(math.Round(float64(w)*s))), max(1, int(math.Round(float64(h)*s)))
}

// Derive makes the preview and thumbnail for an image.
//
// Downscaling goes through a 16-bit intermediate and is dithered on the way
// back to 8 bits, so smooth gradients stay smooth.
func Derive(src image.Image, opt DeriveOptions) (Derivatives, error) {
	b := src.Bounds()
	if b.Dx() <= 0 || b.Dy() <= 0 {
		return Derivatives{}, fmt.Errorf("%w: empty image", ErrCorrupt)
	}
	src = fastSource(src)

	pw, ph := Fit(b.Dx(), b.Dy(), PreviewSize)
	preview, preview16 := scale(src, pw, ph, opt, 1)

	tw, th := Fit(pw, ph, ThumbSize)
	var thumb *image.RGBA
	if preview16 != nil {
		thumb, _ = scale(preview16, tw, th, opt, 2)
	} else {
		thumb, _ = scale(preview, tw, th, opt, 2)
	}

	var d Derivatives
	var err error
	if d.Preview, err = EncodeJPEG(preview, DerivativeQuality); err != nil {
		return d, err
	}
	if d.Thumb, err = EncodeJPEG(thumb, DerivativeQuality); err != nil {
		return d, err
	}
	d.Color = AverageColor(thumb)
	return d, nil
}

// fastSource converts image types the scaler has no fast path for (paletted
// PNGs, 16-bit images, CMYK…) into plain RGBA once, up front.
func fastSource(src image.Image) image.Image {
	switch src.(type) {
	case *image.RGBA, *image.NRGBA, *image.YCbCr, *image.Gray, *image.RGBA64:
		return src
	}
	return toRGBA(src)
}

func toRGBA(src image.Image) *image.RGBA {
	b := src.Bounds()
	if r, ok := src.(*image.RGBA); ok && b.Min == (image.Point{}) {
		return r
	}
	dst := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(dst, dst.Bounds(), src, b.Min, draw.Src)
	return dst
}

// scale resizes src to w×h with Catmull-Rom filtering. When no resize is
// needed it just converts to RGBA and returns a nil 16-bit image.
func scale(src image.Image, w, h int, opt DeriveOptions, salt uint64) (*image.RGBA, *image.RGBA64) {
	b := src.Bounds()
	if b.Dx() == w && b.Dy() == h {
		if s16, ok := src.(*image.RGBA64); ok {
			return quantize(s16, opt, salt), s16
		}
		return toRGBA(src), nil
	}
	hi := image.NewRGBA64(image.Rect(0, 0, w, h))
	draw.CatmullRom.Scale(hi, hi.Bounds(), src, b, draw.Src, nil)
	return quantize(hi, opt, salt), hi
}

// quantize reduces 16-bit pixels to 8 bits with blue-noise dithering and
// optional grain. Transparency is flattened onto black.
func quantize(src *image.RGBA64, opt DeriveOptions, salt uint64) *image.RGBA {
	b := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	seed := opt.Seed ^ (salt * 0x9E3779B97F4A7C15)
	for y := 0; y < b.Dy(); y++ {
		si := src.PixOffset(b.Min.X, b.Min.Y+y)
		di := dst.PixOffset(0, y)
		for x := 0; x < b.Dx(); x++ {
			offset := BlueNoise(x, y)
			if opt.Grain > 0 {
				offset += opt.Grain * Grain(seed, x, y)
			}
			for c := 0; c < 3; c++ {
				v16 := float64(uint16(src.Pix[si+2*c])<<8 | uint16(src.Pix[si+2*c+1]))
				v := math.Floor(v16/257 + offset)
				dst.Pix[di+c] = uint8(min(255, max(0, v)))
			}
			dst.Pix[di+3] = 255
			si += 8
			di += 4
		}
	}
	return dst
}

// EncodeJPEG encodes img as a baseline JPEG.
func EncodeJPEG(img image.Image, quality int) ([]byte, error) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
		return nil, fmt.Errorf("encode jpeg: %w", err)
	}
	return buf.Bytes(), nil
}

// AverageColor is the mean colour of the image as "#rrggbb". The mean is
// taken in linear light, which matches what a heavily blurred version of the
// picture looks like.
func AverageColor(img *image.RGBA) string {
	b := img.Bounds()
	var sum [3]float64
	for y := b.Min.Y; y < b.Max.Y; y++ {
		i := img.PixOffset(b.Min.X, y)
		for x := b.Min.X; x < b.Max.X; x++ {
			sum[0] += srgbToLinear[img.Pix[i]]
			sum[1] += srgbToLinear[img.Pix[i+1]]
			sum[2] += srgbToLinear[img.Pix[i+2]]
			i += 4
		}
	}
	n := float64(b.Dx() * b.Dy())
	var out [3]uint8
	for c := range sum {
		out[c] = uint8(math.Round(LinearToSRGB(sum[c]/n) * 255))
	}
	return fmt.Sprintf("#%02x%02x%02x", out[0], out[1], out[2])
}

var srgbToLinear = func() (t [256]float64) {
	for i := range t {
		t[i] = SRGBToLinear(float64(i) / 255)
	}
	return
}()

// SRGBToLinear converts an sRGB-encoded value in [0,1] to linear light.
func SRGBToLinear(v float64) float64 {
	if v <= 0.04045 {
		return v / 12.92
	}
	return math.Pow((v+0.055)/1.055, 2.4)
}

// LinearToSRGB converts linear light in [0,1] to an sRGB-encoded value.
func LinearToSRGB(v float64) float64 {
	switch {
	case v <= 0:
		return 0
	case v >= 1:
		return 1
	case v <= 0.0031308:
		return v * 12.92
	}
	return 1.055*math.Pow(v, 1/2.4) - 0.055
}

// Resize scales src to exactly w×h with the same filtering and dithering as
// the derivatives.
func Resize(src image.Image, w, h int) *image.RGBA {
	out, _ := scale(fastSource(src), w, h, DeriveOptions{}, 3)
	return out
}

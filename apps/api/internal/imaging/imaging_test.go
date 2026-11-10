package imaging

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math"
	"testing"
)

func gradient(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, color.RGBA{uint8(40 + 100*x/w), uint8(60 + 80*y/h), 120, 255})
		}
	}
	return img
}

func encodePNG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// hugePNGHeader is a PNG that is only a signature and a header claiming
// w×h pixels: tiny on disk, enormous if decoded.
func hugePNGHeader(w, h uint32) []byte {
	var buf bytes.Buffer
	buf.WriteString("\x89PNG\r\n\x1a\n")
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:], w)
	binary.BigEndian.PutUint32(ihdr[4:], h)
	ihdr[8], ihdr[9] = 8, 2 // 8-bit RGB
	binary.Write(&buf, binary.BigEndian, uint32(len(ihdr)))
	chunk := append([]byte("IHDR"), ihdr...)
	buf.Write(chunk)
	binary.Write(&buf, binary.BigEndian, crc32.ChecksumIEEE(chunk))
	return buf.Bytes()
}

func TestSniff(t *testing.T) {
	pad := func(b []byte) []byte { return append(b, make([]byte, 64)...) }
	cases := []struct {
		name string
		head []byte
		want Format
		ok   bool
	}{
		{"jpeg", pad([]byte{0xFF, 0xD8, 0xFF, 0xE0}), FormatJPEG, true},
		{"png", pad([]byte("\x89PNG\r\n\x1a\n")), FormatPNG, true},
		{"webp", pad([]byte("RIFF\x10\x00\x00\x00WEBPVP8 ")), FormatWebP, true},
		{"mp4", pad([]byte("\x00\x00\x00\x18ftypmp42\x00\x00\x00\x00")), FormatMP4, true},
		{"mp4 isom", pad([]byte("\x00\x00\x00\x20ftypisom\x00\x00\x02\x00")), FormatMP4, true},
		{"mov", pad([]byte("\x00\x00\x00\x14ftypqt  \x00\x00\x00\x00")), FormatMOV, true},
		{"old mov", pad([]byte("\x00\x00\x00\x08wide\x00\x00\x00\x00mdat")), FormatMOV, true},
		{"webm", pad([]byte("\x1A\x45\xDF\xA3\x9F\x42\x86\x81\x01\x42\x82\x84webm")), FormatWebM, true},
		{"mkv is not webm", pad([]byte("\x1A\x45\xDF\xA3\x9F\x42\x82\x88matroska")), Format{}, false},
		{"heic is not a video", pad([]byte("\x00\x00\x00\x18ftypheic\x00\x00\x00\x00")), Format{}, false},
		{"avif is not a video", pad([]byte("\x00\x00\x00\x18ftypavif\x00\x00\x00\x00")), Format{}, false},
		{"gif", pad([]byte("GIF89a")), Format{}, false},
		{"text", pad([]byte("hello, world")), Format{}, false},
		{"html", pad([]byte("<!doctype html><html>")), Format{}, false},
		{"empty", nil, Format{}, false},
		{"short", []byte{0xFF}, Format{}, false},
	}
	for _, c := range cases {
		got, ok := Sniff(c.head)
		if ok != c.ok || got != c.want {
			t.Errorf("%s: Sniff = %+v, %v; want %+v, %v", c.name, got, ok, c.want, c.ok)
		}
	}
}

func TestDecodeImageLimitsAndTypes(t *testing.T) {
	// A real small PNG decodes.
	img, f, err := DecodeImage(encodePNG(t, gradient(64, 40)))
	if err != nil || f != FormatPNG || img.Bounds().Dx() != 64 {
		t.Fatalf("small PNG: %v %+v", err, f)
	}

	// 10000×7000 = 70 megapixels: refused from the header alone.
	if _, _, err := DecodeImage(hugePNGHeader(10000, 7000)); !errors.Is(err, ErrTooManyPixels) {
		t.Fatalf("70 MP image: got %v, want ErrTooManyPixels", err)
	}
	// Exactly at the limit passes the pixel check (and then fails as truncated, not as too big).
	if _, _, err := DecodeImage(hugePNGHeader(10000, 6000)); errors.Is(err, ErrTooManyPixels) || !errors.Is(err, ErrCorrupt) {
		t.Fatalf("60 MP header-only image: got %v, want ErrCorrupt", err)
	}

	// Wrong type, regardless of what the file is called.
	for _, data := range [][]byte{[]byte("GIF89a..........................................."), []byte("plain text"), {}} {
		if _, _, err := DecodeImage(data); !errors.Is(err, ErrUnsupported) {
			t.Errorf("non-image %q: got %v, want ErrUnsupported", data, err)
		}
	}
	// A video is not an image.
	if _, _, err := DecodeImage(append([]byte("\x00\x00\x00\x18ftypmp42"), make([]byte, 64)...)); !errors.Is(err, ErrUnsupported) {
		t.Errorf("mp4 as image: got %v", err)
	}

	// Over 25 MiB.
	big := append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, MaxImageBytes+1)...)
	if _, _, err := DecodeImage(big); !errors.Is(err, ErrTooBig) {
		t.Errorf("26 MiB file: got %v, want ErrTooBig", err)
	}

	// Looks like a JPEG but is garbage.
	if _, _, err := DecodeImage(append([]byte{0xFF, 0xD8, 0xFF, 0xE0}, []byte("garbage garbage garbage")...)); !errors.Is(err, ErrCorrupt) {
		t.Errorf("garbage JPEG: got %v, want ErrCorrupt", err)
	}
}

func TestFit(t *testing.T) {
	cases := []struct{ w, h, size, wantW, wantH int }{
		{3840, 2400, 1920, 1920, 1200},
		{7680, 2400, 1920, 1920, 600},
		{7680, 2400, 640, 640, 200},
		{2160, 3840, 1920, 1080, 1920}, // portrait: the long side is the height
		{800, 500, 1920, 800, 500},     // never upscale
		{1920, 1080, 1920, 1920, 1080},
		{10000, 1, 640, 640, 1},
	}
	for _, c := range cases {
		if w, h := Fit(c.w, c.h, c.size); w != c.wantW || h != c.wantH {
			t.Errorf("Fit(%d,%d,%d) = %d×%d, want %d×%d", c.w, c.h, c.size, w, h, c.wantW, c.wantH)
		}
	}
}

func jpegSize(t *testing.T, data []byte) (int, int) {
	t.Helper()
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	return cfg.Width, cfg.Height
}

func TestDerive(t *testing.T) {
	d, err := Derive(gradient(2560, 1600), DeriveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if w, h := jpegSize(t, d.Preview); w != 1920 || h != 1200 {
		t.Errorf("preview is %d×%d, want 1920×1200", w, h)
	}
	if w, h := jpegSize(t, d.Thumb); w != 640 || h != 400 {
		t.Errorf("thumb is %d×%d, want 640×400", w, h)
	}
	if len(d.Color) != 7 || d.Color[0] != '#' {
		t.Errorf("colour = %q", d.Color)
	}

	// Small images are never enlarged.
	d, err = Derive(gradient(500, 300), DeriveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if w, h := jpegSize(t, d.Preview); w != 500 || h != 300 {
		t.Errorf("small preview is %d×%d, want 500×300", w, h)
	}
	if w, h := jpegSize(t, d.Thumb); w != 500 || h != 300 {
		t.Errorf("small thumb is %d×%d, want 500×300", w, h)
	}

	// Same input and options give the same bytes; image types other than
	// RGBA (here: paletted) go through too.
	pal := image.NewPaletted(image.Rect(0, 0, 900, 700), color.Palette{color.Black, color.RGBA{200, 30, 30, 255}})
	for i := range pal.Pix {
		pal.Pix[i] = uint8(i / 7 % 2)
	}
	a, err := Derive(pal, DeriveOptions{Grain: 1, Seed: 7})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := Derive(pal, DeriveOptions{Grain: 1, Seed: 7})
	if !bytes.Equal(a.Thumb, b.Thumb) || !bytes.Equal(a.Preview, b.Preview) {
		t.Error("Derive is not repeatable")
	}
}

func TestAverageColor(t *testing.T) {
	flat := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for i := 0; i < len(flat.Pix); i += 4 {
		flat.Pix[i], flat.Pix[i+1], flat.Pix[i+2], flat.Pix[i+3] = 0xb8, 0x9a, 0x7c, 0xff
	}
	if got := AverageColor(flat); got != "#b89a7c" {
		t.Errorf("flat colour = %s, want #b89a7c", got)
	}

	// Half black, half white averages in linear light to mid-grey ≈ #bcbcbc,
	// not the #808080 a naive average of the encoded values gives.
	half := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for i := 0; i < len(half.Pix); i += 4 {
		v := uint8(0)
		if (i/4)%8 < 4 {
			v = 255
		}
		half.Pix[i], half.Pix[i+1], half.Pix[i+2], half.Pix[i+3] = v, v, v, 255
	}
	if got := AverageColor(half); got != "#bcbcbc" {
		t.Errorf("half/half colour = %s, want #bcbcbc", got)
	}
}

func TestBlueNoiseAndGrain(t *testing.T) {
	// The mask holds every threshold level exactly once, so its mean is 0.5
	// and dithering is unbiased.
	seen := map[float32]bool{}
	sum := 0.0
	for y := 0; y < blueNoiseSize; y++ {
		for x := 0; x < blueNoiseSize; x++ {
			v := BlueNoise(x, y)
			if v < 0 || v >= 1 {
				t.Fatalf("mask value %v out of [0,1)", v)
			}
			seen[float32(v)] = true
			sum += v
		}
	}
	n := blueNoiseSize * blueNoiseSize
	if len(seen) != n || math.Abs(sum/float64(n)-0.5) > 1e-6 {
		t.Errorf("mask has %d distinct levels (want %d), mean %v", len(seen), n, sum/float64(n))
	}
	if BlueNoise(3, 5) != BlueNoise(3+blueNoiseSize, 5+2*blueNoiseSize) {
		t.Error("mask does not tile")
	}

	// Grain: mean about 0, standard deviation about 1.
	var s, s2 float64
	const m = 200
	for y := 0; y < m; y++ {
		for x := 0; x < m; x++ {
			g := Grain(42, x, y)
			s += g
			s2 += g * g
		}
	}
	mean := s / (m * m)
	sd := math.Sqrt(s2/(m*m) - mean*mean)
	if math.Abs(mean) > 0.03 || math.Abs(sd-1) > 0.05 {
		t.Errorf("grain mean %.3f, sd %.3f; want about 0 and 1", mean, sd)
	}
}

func TestExifOrientation(t *testing.T) {
	// A 4×2 JPEG with an EXIF block that says "rotate 90° clockwise to view".
	var plain bytes.Buffer
	if err := jpeg.Encode(&plain, gradient(4, 2), nil); err != nil {
		t.Fatal(err)
	}
	tiff := []byte("II*\x00\x08\x00\x00\x00" + // little-endian TIFF header, first IFD at offset 8
		"\x01\x00" + // one entry
		"\x12\x01\x03\x00\x01\x00\x00\x00\x06\x00\x00\x00" + // tag 0x0112 Orientation, SHORT, count 1, value 6
		"\x00\x00\x00\x00")
	exif := append([]byte("Exif\x00\x00"), tiff...)
	seg := []byte{0xFF, 0xE1, byte((len(exif) + 2) >> 8), byte(len(exif) + 2)}
	data := append([]byte{0xFF, 0xD8}, append(append(seg, exif...), plain.Bytes()[2:]...)...)

	if Upright(data) {
		t.Fatal("orientation 6 was not detected")
	}
	if !Upright(plain.Bytes()) {
		t.Fatal("a JPEG without EXIF should count as upright")
	}
	img, _, err := DecodeImage(data)
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 2 || b.Dy() != 4 {
		t.Fatalf("rotated image is %d×%d, want 2×4", b.Dx(), b.Dy())
	}
}

func TestDecodeWebP(t *testing.T) {
	// The smallest valid lossless WebP: a single pixel.
	tiny := []byte("RIFF\x1a\x00\x00\x00WEBPVP8L\x0d\x00\x00\x00/\x00\x00\x00\x10\x07\x10\x11\x11\x88\x88\xfe\x07\x00")
	img, f, err := DecodeImage(tiny)
	if err != nil {
		t.Fatalf("WebP decode: %v", err)
	}
	if f != FormatWebP || img.Bounds().Dx() != 1 || img.Bounds().Dy() != 1 {
		t.Fatalf("WebP: format %+v, bounds %v", f, img.Bounds())
	}
	if _, err := Derive(img, DeriveOptions{}); err != nil {
		t.Fatalf("Derive on a 1×1 image: %v", err)
	}
}

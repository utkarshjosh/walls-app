// Package imaging validates incoming media and produces the JPEG derivatives
// (preview, thumbnail, dominant colour) every catalog item needs.
package imaging

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"
	"image/png"

	"golang.org/x/image/webp"
)

// Limits for owner-supplied files.
const (
	MaxImageBytes  = 25 << 20   // 25 MiB
	MaxImagePixels = 60_000_000 // 60 megapixels
	MaxVideoBytes  = 200 << 20  // 200 MiB
)

// Reasons a file is refused. Wrapped errors carry the specifics.
var (
	ErrUnsupported   = errors.New("unsupported file type")
	ErrTooBig        = errors.New("file is too large")
	ErrTooManyPixels = errors.New("image has too many pixels")
	ErrCorrupt       = errors.New("file could not be decoded")
)

// Format is what a file really is, judged from its first bytes.
type Format struct {
	Kind        string // "image" or "video"
	Ext         string // canonical extension without the dot
	ContentType string
}

var (
	FormatJPEG = Format{"image", "jpg", "image/jpeg"}
	FormatPNG  = Format{"image", "png", "image/png"}
	FormatWebP = Format{"image", "webp", "image/webp"}
	FormatMP4  = Format{"video", "mp4", "video/mp4"}
	FormatMOV  = Format{"video", "mov", "video/quicktime"}
	FormatWebM = Format{"video", "webm", "video/webm"}
)

// Sniff identifies a file from its leading bytes (pass at least 64). The file
// name and extension are deliberately ignored.
func Sniff(head []byte) (Format, bool) {
	switch {
	case len(head) >= 3 && head[0] == 0xFF && head[1] == 0xD8 && head[2] == 0xFF:
		return FormatJPEG, true
	case bytes.HasPrefix(head, []byte("\x89PNG\r\n\x1a\n")):
		return FormatPNG, true
	case len(head) >= 12 && string(head[0:4]) == "RIFF" && string(head[8:12]) == "WEBP":
		return FormatWebP, true
	case bytes.HasPrefix(head, []byte{0x1A, 0x45, 0xDF, 0xA3}):
		// EBML container: WebM declares its doctype near the start; plain
		// Matroska (.mkv) does not qualify.
		if bytes.Contains(head[:min(len(head), 64)], []byte("webm")) {
			return FormatWebM, true
		}
		return Format{}, false
	case len(head) >= 12:
		switch string(head[4:8]) {
		case "ftyp":
			brand := string(head[8:12])
			switch brand {
			case "qt  ":
				return FormatMOV, true
			case "heic", "heix", "hevc", "hevx", "heim", "heis", "mif1", "msf1", "avif", "avis":
				return Format{}, false // still-image containers, not video
			}
			return FormatMP4, true
		case "moov", "mdat", "wide", "free", "skip", "pnot":
			return FormatMOV, true // older QuickTime files have no ftyp box
		}
	}
	return Format{}, false
}

// DecodeImage checks an image against the size and pixel limits before
// decoding it, so a small file that claims enormous dimensions is refused
// without allocating memory for it. JPEGs are turned upright according to
// their EXIF orientation.
func DecodeImage(data []byte) (image.Image, Format, error) {
	f, ok := Sniff(data)
	if !ok || f.Kind != "image" {
		return nil, f, fmt.Errorf("%w: not a JPEG, PNG or WebP image", ErrUnsupported)
	}
	if len(data) > MaxImageBytes {
		return nil, f, fmt.Errorf("%w: %.1f MiB, limit is 25 MiB", ErrTooBig, float64(len(data))/(1<<20))
	}

	var cfg image.Config
	var err error
	switch f {
	case FormatJPEG:
		cfg, err = jpeg.DecodeConfig(bytes.NewReader(data))
	case FormatPNG:
		cfg, err = png.DecodeConfig(bytes.NewReader(data))
	case FormatWebP:
		cfg, err = webp.DecodeConfig(bytes.NewReader(data))
	}
	if err != nil {
		return nil, f, fmt.Errorf("%w: %v", ErrCorrupt, err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return nil, f, fmt.Errorf("%w: empty image", ErrCorrupt)
	}
	if px := int64(cfg.Width) * int64(cfg.Height); px > MaxImagePixels {
		return nil, f, fmt.Errorf("%w: %d×%d is %.1f megapixels, limit is 60", ErrTooManyPixels,
			cfg.Width, cfg.Height, float64(px)/1e6)
	}

	var img image.Image
	switch f {
	case FormatJPEG:
		img, err = jpeg.Decode(bytes.NewReader(data))
	case FormatPNG:
		img, err = png.Decode(bytes.NewReader(data))
	case FormatWebP:
		img, err = webp.Decode(bytes.NewReader(data))
	}
	if err != nil {
		return nil, f, fmt.Errorf("%w: %v", ErrCorrupt, err)
	}
	if f == FormatJPEG {
		img = orient(img, exifOrientation(data))
	}
	return img, f, nil
}

// exifOrientation returns the EXIF orientation (1–8) of a JPEG, or 1 when
// there is none or it cannot be read.
func exifOrientation(data []byte) int {
	i := 2 // past the SOI marker
	for i+4 <= len(data) {
		if data[i] != 0xFF {
			return 1
		}
		marker := data[i+1]
		if marker == 0xD8 || (marker >= 0xD0 && marker <= 0xD7) || marker == 0x01 {
			i += 2
			continue
		}
		if marker == 0xDA || marker == 0xD9 { // image data starts; no EXIF before it
			return 1
		}
		size := int(binary.BigEndian.Uint16(data[i+2:]))
		if size < 2 || i+2+size > len(data) {
			return 1
		}
		seg := data[i+4 : i+2+size]
		if marker == 0xE1 && bytes.HasPrefix(seg, []byte("Exif\x00\x00")) {
			return tiffOrientation(seg[6:])
		}
		i += 2 + size
	}
	return 1
}

func tiffOrientation(t []byte) int {
	if len(t) < 8 {
		return 1
	}
	var bo binary.ByteOrder
	switch string(t[:2]) {
	case "II":
		bo = binary.LittleEndian
	case "MM":
		bo = binary.BigEndian
	default:
		return 1
	}
	off := int(bo.Uint32(t[4:8]))
	if off < 8 || off+2 > len(t) {
		return 1
	}
	n := int(bo.Uint16(t[off:]))
	for k := 0; k < n; k++ {
		e := off + 2 + k*12
		if e+12 > len(t) {
			return 1
		}
		if bo.Uint16(t[e:]) == 0x0112 { // Orientation tag
			if v := int(bo.Uint16(t[e+8:])); v >= 1 && v <= 8 {
				return v
			}
			return 1
		}
	}
	return 1
}

// orient returns the image rotated/flipped upright for the given EXIF
// orientation. Orientation 1 (already upright) returns the input unchanged.
func orient(src image.Image, o int) image.Image {
	if o <= 1 || o > 8 {
		return src
	}
	b := src.Bounds()
	rgba := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(rgba, rgba.Bounds(), src, b.Min, draw.Src)
	w, h := b.Dx(), b.Dy()
	dw, dh := w, h
	if o >= 5 {
		dw, dh = h, w
	}
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for y := 0; y < dh; y++ {
		for x := 0; x < dw; x++ {
			var sx, sy int
			switch o {
			case 2: // mirrored left-right
				sx, sy = w-1-x, y
			case 3: // rotated 180°
				sx, sy = w-1-x, h-1-y
			case 4: // mirrored top-bottom
				sx, sy = x, h-1-y
			case 5: // transposed
				sx, sy = y, x
			case 6: // needs 90° clockwise
				sx, sy = y, h-1-x
			case 7: // transversed
				sx, sy = w-1-y, h-1-x
			case 8: // needs 90° counter-clockwise
				sx, sy = w-1-y, x
			}
			copy(dst.Pix[dst.PixOffset(x, y):dst.PixOffset(x, y)+4], rgba.Pix[rgba.PixOffset(sx, sy):rgba.PixOffset(sx, sy)+4])
		}
	}
	return dst
}

// Upright reports whether a JPEG needs no EXIF rotation to display correctly.
func Upright(jpegData []byte) bool { return exifOrientation(jpegData) == 1 }

package main

import (
	"bytes"
	"flag"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/image/draw"

	"github.com/utkarshjoshi/walls/apps/api/internal/artgen"
	"github.com/utkarshjoshi/walls/apps/api/internal/imaging"
)

// cmdRender writes the starter artwork to a folder as JPEGs without touching
// the catalog. It exists for looking at the pictures while tuning the
// generator.
func cmdRender(args []string) error {
	fs := flag.NewFlagSet("render", flag.ContinueOnError)
	out := fs.String("out", "data/render", "folder to write JPEGs into")
	scale := fs.Float64("scale", 0.5, "size relative to the shipped resolution (1 = full size)")
	only := fs.String("only", "", "comma-separated slugs or styles to render (default: everything)")
	sheet := fs.Int("sheet", 0, "also write contact sheets with this many pictures per sheet")
	quality := fs.Int("quality", artgen.JPEGQuality, "JPEG quality")
	grain := fs.Float64("grain", -1, "override film grain strength in 8-bit steps (default: each piece's own)")
	inspect := fs.String("inspect", "", "x,y,w,h,gain: also write a contrast-boosted PNG crop of the decoded JPEG, for checking banding (negative gain: brightness only)")
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "usage: walls render [-out dir] [-scale 0.5] [-only slug,style] [-sheet 6]")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *scale <= 0 || *scale > 1 {
		return fmt.Errorf("-scale must be between 0 and 1")
	}
	want := map[string]bool{}
	for _, s := range strings.Split(*only, ",") {
		if s = strings.TrimSpace(s); s != "" {
			want[s] = true
		}
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		return err
	}

	var tiles []image.Image
	for _, p := range artgen.Catalog() {
		if len(want) > 0 && !want[p.Slug] && !want[p.Style] {
			continue
		}
		fw, fh := p.Size()
		w, h := int(float64(fw)**scale), int(float64(fh)**scale)
		start := time.Now()
		var img *image.RGBA
		if *grain >= 0 {
			img = artgen.RenderGrain(p, w, h, *grain)
		} else {
			img = artgen.Render(p, w, h)
		}
		renderTime := time.Since(start)
		data, err := imaging.EncodeJPEG(img, *quality)
		if err != nil {
			return err
		}
		path := filepath.Join(*out, p.Slug+".jpg")
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return err
		}
		fmt.Printf("%-14s %-9s %5d×%-5d %7.2f MB  render %v\n", p.Slug, p.Style, w, h,
			float64(len(data))/1e6, renderTime.Round(time.Millisecond))
		if *sheet > 0 {
			tiles = append(tiles, img)
		}
		if *inspect != "" {
			if err := writeInspection(data, *inspect, filepath.Join(*out, p.Slug+"-inspect.png")); err != nil {
				return err
			}
		}
	}

	// Contact sheets: two columns of 960-pixel-wide tiles.
	for n := 0; *sheet > 0 && n*(*sheet) < len(tiles); n++ {
		group := tiles[n*(*sheet) : min((n+1)*(*sheet), len(tiles))]
		const tileW, gap, cols = 960, 12, 2
		y, rowH, col := gap, 0, 0
		type placed struct {
			img  image.Image
			rect image.Rectangle
		}
		var places []placed
		for _, t := range group {
			b := t.Bounds()
			tw := tileW
			if b.Dx() > b.Dy()*2 { // panoramas take a full row
				if col != 0 {
					y += rowH + gap
					col, rowH = 0, 0
				}
				tw = tileW*cols + gap*(cols-1)
			}
			th := b.Dy() * tw / b.Dx()
			x := gap + col*(tileW+gap)
			places = append(places, placed{t, image.Rect(x, y, x+tw, y+th)})
			rowH = max(rowH, th)
			if tw > tileW || col == cols-1 {
				y += rowH + gap
				col, rowH = 0, 0
			} else {
				col++
			}
		}
		if col != 0 {
			y += rowH + gap
		}
		canvas := image.NewRGBA(image.Rect(0, 0, gap+cols*(tileW+gap), y))
		for i := range canvas.Pix {
			canvas.Pix[i] = 0x80
		}
		for _, pl := range places {
			draw.CatmullRom.Scale(canvas, pl.rect, pl.img, pl.img.Bounds(), draw.Src, nil)
		}
		data, err := imaging.EncodeJPEG(canvas, 92)
		if err != nil {
			return err
		}
		path := filepath.Join(*out, fmt.Sprintf("sheet-%d.jpg", n+1))
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return err
		}
		fmt.Println("wrote", path)
	}
	return nil
}

// writeInspection decodes the JPEG the way a viewer would, crops a region and
// stretches its contrast around the region's average so one 8-bit step
// becomes plainly visible. Smooth areas should look like even noise; visible
// stripes mean banding.
func writeInspection(jpegData []byte, spec, path string) error {
	var x, y, w, h int
	var gain float64
	if _, err := fmt.Sscanf(spec, "%d,%d,%d,%d,%g", &x, &y, &w, &h, &gain); err != nil {
		return fmt.Errorf("-inspect wants x,y,w,h,gain: %w", err)
	}
	img, err := jpeg.Decode(bytes.NewReader(jpegData))
	if err != nil {
		return err
	}
	rect := image.Rect(x, y, x+w, y+h).Intersect(img.Bounds())
	if rect.Empty() {
		return fmt.Errorf("-inspect region is outside the picture")
	}
	crop := image.NewRGBA(image.Rect(0, 0, rect.Dx(), rect.Dy()))
	draw.Draw(crop, crop.Bounds(), img, rect.Min, draw.Src)
	var mean [3]float64
	for i := 0; i < len(crop.Pix); i += 4 {
		for c := 0; c < 3; c++ {
			mean[c] += float64(crop.Pix[i+c])
		}
	}
	n := float64(len(crop.Pix) / 4)
	for i := 0; gain != 0 && i < len(crop.Pix); i += 4 { // gain 0: plain crop
		if gain < 0 { // negative gain: show brightness only, ignoring colour
			l := 0.299*(float64(crop.Pix[i])-mean[0]/n) + 0.587*(float64(crop.Pix[i+1])-mean[1]/n) +
				0.114*(float64(crop.Pix[i+2])-mean[2]/n)
			v := uint8(min(255, max(0, 128-l*gain)))
			crop.Pix[i], crop.Pix[i+1], crop.Pix[i+2] = v, v, v
			continue
		}
		for c := 0; c < 3; c++ {
			v := 128 + (float64(crop.Pix[i+c])-mean[c]/n)*gain
			crop.Pix[i+c] = uint8(min(255, max(0, v)))
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, crop)
}

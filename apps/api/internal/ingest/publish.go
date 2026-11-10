// Package ingest puts media into the catalog: it stores the original and its
// derivatives in the object store and records the item in the database. The
// seed command and the folder importer both go through the same Pipeline.
package ingest

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"io"
	"os"
	"slices"
	"sync"
	"time"

	"github.com/utkarshjoshi/walls/apps/api/internal/catalog"
	"github.com/utkarshjoshi/walls/apps/api/internal/imaging"
	"github.com/utkarshjoshi/walls/apps/api/internal/storage"
)

// ErrOverCap means storing the item would push the catalog past its storage
// budget.
var ErrOverCap = errors.New("storage cap reached")

// PosterMaxWidth caps stored poster images for videos and scenes.
const PosterMaxWidth = 3840

// Pipeline is the shared "add this to the catalog" path.
type Pipeline struct {
	DB    *catalog.DB
	Store storage.Store
	// Cap is the most bytes the catalog may hold across all stored objects;
	// 0 means no limit.
	Cap int64

	mu sync.Mutex // serialises the cap check, uploads and the database write
}

// Meta is the descriptive information for an item.
type Meta struct {
	Title     string
	Category  string // slug
	Tags      []string
	Author    string
	License   string
	SourceURL string
}

// Asset is one thing to publish.
type Asset struct {
	// The original file: either Data (in memory) or Path (streamed from
	// disk, used for large videos).
	Data []byte
	Path string

	Kind        string // catalog.KindImage, KindVideo or KindScene
	Ext         string // extension for the stored original, without the dot
	ContentType string

	// Image is the decoded picture that previews are made from: the image
	// itself, or the poster for a video or scene.
	Image image.Image
	// PosterJPEG, when set, is stored as the poster untouched (it must be
	// the JPEG that Image was decoded from). Otherwise the poster is
	// encoded from Image. Ignored for images, which have no poster.
	PosterJPEG []byte

	Meta      Meta
	Rank      int64     // 0: keep the existing rank, or append after everything else
	Origin    string    // "seed", "scene" or "ingest"
	CreatedAt time.Time // zero: now (only used the first time an item is added)
	Color     string    // "#rrggbb" override; "" to compute from the thumbnail
	Grain     float64   // film grain for derivatives; see imaging.DeriveOptions
}

// Result says what Publish did.
type Result struct {
	ID     string
	Status string // "added", "updated" or "unchanged"
	Item   catalog.Item
}

// Keys returns the object keys for an item.
func Keys(id, ext string) (original, preview, thumb, poster string) {
	base := "catalog/" + id + "/"
	return base + "original." + ext, base + "preview.jpg", base + "thumb.jpg", base + "poster.jpg"
}

// ItemKeys lists every object key an item owns.
func ItemKeys(it catalog.Item) []string {
	original, preview, thumb, poster := Keys(it.ID, it.OriginalExt)
	keys := []string{original, preview, thumb}
	if it.HasPoster {
		keys = append(keys, poster)
	}
	return keys
}

func (a *Asset) open() (io.ReadSeekCloser, int64, error) {
	if a.Path == "" {
		return nopCloser{bytes.NewReader(a.Data)}, int64(len(a.Data)), nil
	}
	f, err := os.Open(a.Path)
	if err != nil {
		return nil, 0, err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, 0, err
	}
	return f, st.Size(), nil
}

type nopCloser struct{ io.ReadSeeker }

func (nopCloser) Close() error { return nil }

// Publish stores the asset and records it. Publishing the same bytes again is
// cheap: the id is the content hash, so nothing is re-uploaded and only
// changed details are updated.
func (p *Pipeline) Publish(ctx context.Context, a Asset) (Result, error) {
	if a.Image == nil {
		return Result{}, errors.New("ingest: asset has no image to make previews from")
	}
	src, size, err := a.open()
	if err != nil {
		return Result{}, err
	}
	defer src.Close()
	if size == 0 {
		return Result{}, errors.New("ingest: original is empty")
	}

	sum := sha256.New()
	if _, err := io.Copy(sum, src); err != nil {
		return Result{}, err
	}
	id := hex.EncodeToString(sum.Sum(nil))[:12]
	origKey, previewKey, thumbKey, posterKey := Keys(id, a.Ext)
	hasPoster := a.Kind != catalog.KindImage

	p.mu.Lock()
	defer p.mu.Unlock()

	existing, err := p.DB.GetItem(ctx, id)
	found := err == nil
	if err != nil && !errors.Is(err, catalog.ErrNotFound) {
		return Result{}, err
	}
	if found && existing.Bytes != size {
		return Result{}, fmt.Errorf("ingest: id %s already belongs to a different file (%q)", id, existing.Title)
	}

	// Are all of this item's objects already in the store?
	stored := found && existing.OriginalExt == a.Ext && existing.HasPoster == hasPoster
	if stored {
		for _, key := range ItemKeys(existing) {
			if _, err := p.Store.Head(ctx, key); err != nil {
				if !errors.Is(err, storage.ErrNotFound) {
					return Result{}, err
				}
				stored = false
				break
			}
		}
	}

	b := a.Image.Bounds()
	item := catalog.Item{
		ID: id, Title: a.Meta.Title, Kind: a.Kind, Width: b.Dx(), Height: b.Dy(), Bytes: size,
		Category: a.Meta.Category, Tags: a.Meta.Tags, Author: a.Meta.Author, License: a.Meta.License,
		SourceURL: a.Meta.SourceURL, OriginalExt: a.Ext, OriginalType: a.ContentType,
		HasPoster: hasPoster, Origin: a.Origin, CreatedAt: a.CreatedAt,
	}

	uploaded := false
	if stored {
		item.Width, item.Height = existing.Width, existing.Height
		item.StoredBytes, item.Color = existing.StoredBytes, existing.Color
	} else {
		d, err := imaging.Derive(a.Image, imaging.DeriveOptions{Grain: a.Grain, Seed: uint64(size)})
		if err != nil {
			return Result{}, err
		}
		var poster []byte
		if hasPoster {
			if poster, item.Width, item.Height, err = makePoster(a); err != nil {
				return Result{}, err
			}
		}
		item.Color = d.Color
		item.StoredBytes = size + int64(len(d.Preview)+len(d.Thumb)+len(poster))

		if p.Cap > 0 {
			others, err := p.DB.StoredBytes(ctx, id)
			if err != nil {
				return Result{}, err
			}
			if others+item.StoredBytes > p.Cap {
				return Result{}, fmt.Errorf("%w: the catalog holds %s of its %s budget and this item needs %s more "+
					"(raise WALLS_STORAGE_CAP_BYTES or remove items with \"walls rm\")", ErrOverCap,
					HumanBytes(others), HumanBytes(p.Cap), HumanBytes(item.StoredBytes))
			}
		}

		if err := p.Store.Put(ctx, previewKey, bytes.NewReader(d.Preview), int64(len(d.Preview)), "image/jpeg"); err != nil {
			return Result{}, err
		}
		if err := p.Store.Put(ctx, thumbKey, bytes.NewReader(d.Thumb), int64(len(d.Thumb)), "image/jpeg"); err != nil {
			return Result{}, err
		}
		if hasPoster {
			if err := p.Store.Put(ctx, posterKey, bytes.NewReader(poster), int64(len(poster)), "image/jpeg"); err != nil {
				return Result{}, err
			}
		}
		if _, err := src.Seek(0, io.SeekStart); err != nil {
			return Result{}, err
		}
		if err := p.Store.Put(ctx, origKey, src, size, a.ContentType); err != nil {
			return Result{}, err
		}
		uploaded = true
	}
	if a.Color != "" {
		item.Color = a.Color
	}

	switch {
	case a.Rank != 0:
		item.Rank = a.Rank
	case found:
		item.Rank = existing.Rank
	default:
		top, err := p.DB.MaxRank(ctx)
		if err != nil {
			return Result{}, err
		}
		// New owner-supplied items go after the first-party set.
		item.Rank = max(top, 1_000_000) + 100
	}
	if found {
		item.CreatedAt = existing.CreatedAt
	}

	status := "added"
	if found {
		status = "unchanged"
		if uploaded || !sameDetails(existing, item) {
			status = "updated"
		}
	}
	if status != "unchanged" {
		if err := p.DB.UpsertItem(ctx, item); err != nil {
			return Result{}, err
		}
	}
	if saved, err := p.DB.GetItem(ctx, id); err == nil {
		item = saved
	}
	return Result{ID: id, Status: status, Item: item}, nil
}

// makePoster returns the JPEG stored as the poster and its pixel size.
func makePoster(a Asset) ([]byte, int, int, error) {
	b := a.Image.Bounds()
	if a.PosterJPEG != nil && b.Dx() <= PosterMaxWidth {
		return a.PosterJPEG, b.Dx(), b.Dy(), nil
	}
	img := a.Image
	if b.Dx() > PosterMaxWidth {
		w, h := imaging.Fit(b.Dx(), b.Dy(), PosterMaxWidth)
		img = imaging.Resize(a.Image, w, h)
	}
	data, err := imaging.EncodeJPEG(img, 90)
	return data, img.Bounds().Dx(), img.Bounds().Dy(), err
}

func sameDetails(a, b catalog.Item) bool {
	return a.Title == b.Title && a.Kind == b.Kind && a.Category == b.Category && slices.Equal(a.Tags, b.Tags) &&
		a.Author == b.Author && a.License == b.License && a.SourceURL == b.SourceURL && a.Color == b.Color &&
		a.Rank == b.Rank && a.Origin == b.Origin && a.OriginalType == b.OriginalType &&
		a.Width == b.Width && a.Height == b.Height && a.StoredBytes == b.StoredBytes
}

// Remove deletes an item's objects and then its catalog row.
func (p *Pipeline) Remove(ctx context.Context, id string) (catalog.Item, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	it, err := p.DB.GetItem(ctx, id)
	if err != nil {
		return it, err
	}
	for _, key := range ItemKeys(it) {
		if err := p.Store.Delete(ctx, key); err != nil {
			return it, err
		}
	}
	return it, p.DB.DeleteItem(ctx, id)
}

// HumanBytes formats a byte count for people ("1.8 MB").
func HumanBytes(n int64) string {
	switch {
	case n >= 1e9:
		return fmt.Sprintf("%.2f GB", float64(n)/1e9)
	case n >= 1e6:
		return fmt.Sprintf("%.1f MB", float64(n)/1e6)
	case n >= 1e3:
		return fmt.Sprintf("%.0f kB", float64(n)/1e3)
	}
	return fmt.Sprintf("%d B", n)
}

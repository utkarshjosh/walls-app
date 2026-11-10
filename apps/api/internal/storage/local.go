package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// metaDir holds one tiny file per object with its content type. It mirrors
// the object tree and is hidden so it can never collide with a valid key.
const metaDir = ".meta"

// Local stores objects as plain files under a root folder.
type Local struct {
	root string
}

// NewLocal creates the root folder if needed.
func NewLocal(dir string) (*Local, error) {
	if dir == "" {
		return nil, errors.New("storage: local directory is empty")
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, fmt.Errorf("storage: create %s: %w", abs, err)
	}
	return &Local{root: abs}, nil
}

// path maps a key to a file path, refusing anything that would land outside
// the root.
func (l *Local) path(key string) (string, error) {
	if err := ValidateKey(key); err != nil {
		return "", fmt.Errorf("%w: %q", ErrInvalidKey, key)
	}
	p := filepath.Join(l.root, filepath.FromSlash(key))
	rel, err := filepath.Rel(l.root, p)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("%w: %q", ErrInvalidKey, key)
	}
	return p, nil
}

func (l *Local) metaPath(key string) string {
	return filepath.Join(l.root, metaDir, filepath.FromSlash(key))
}

// Put writes to a temporary file and renames it into place, so readers never
// see a half-written object.
func (l *Local) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	p, err := l.path(key)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), ".put-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op after a successful rename

	n, err := io.Copy(tmp, r)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return fmt.Errorf("storage: write %s: %w", key, err)
	}
	if size >= 0 && n != size {
		return fmt.Errorf("storage: write %s: got %d bytes, expected %d", key, n, size)
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmpName, p); err != nil {
		return err
	}

	mp := l.metaPath(key)
	if err := os.MkdirAll(filepath.Dir(mp), 0o755); err != nil {
		return err
	}
	return os.WriteFile(mp, []byte(contentType), 0o644)
}

func (l *Local) info(key, p string, st fs.FileInfo) ObjectInfo {
	ct := ""
	if b, err := os.ReadFile(l.metaPath(key)); err == nil {
		ct = strings.TrimSpace(string(b))
	}
	if ct == "" {
		ct = mime.TypeByExtension(filepath.Ext(p))
	}
	if ct == "" {
		ct = "application/octet-stream"
	}
	return ObjectInfo{Size: st.Size(), ContentType: ct, ModTime: st.ModTime()}
}

func (l *Local) Head(ctx context.Context, key string) (ObjectInfo, error) {
	p, err := l.path(key)
	if err != nil {
		return ObjectInfo{}, err
	}
	st, err := os.Stat(p)
	if err != nil || st.IsDir() {
		if err == nil || errors.Is(err, fs.ErrNotExist) {
			return ObjectInfo{}, ErrNotFound
		}
		return ObjectInfo{}, err
	}
	return l.info(key, p, st), nil
}

func (l *Local) Open(ctx context.Context, key string) (io.ReadSeekCloser, ObjectInfo, error) {
	p, err := l.path(key)
	if err != nil {
		return nil, ObjectInfo{}, err
	}
	f, err := os.Open(p)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, ObjectInfo{}, ErrNotFound
		}
		return nil, ObjectInfo{}, err
	}
	st, err := f.Stat()
	if err != nil || st.IsDir() {
		f.Close()
		if err == nil {
			err = ErrNotFound
		}
		return nil, ObjectInfo{}, err
	}
	return f, l.info(key, p, st), nil
}

// Delete removes the object. Deleting a key that is already gone is not an
// error, which keeps "rm" and re-runs simple.
func (l *Local) Delete(ctx context.Context, key string) error {
	p, err := l.path(key)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	_ = os.Remove(l.metaPath(key))
	// Tidy now-empty folders; these fail harmlessly when not empty.
	_ = os.Remove(filepath.Dir(p))
	_ = os.Remove(filepath.Dir(l.metaPath(key)))
	return nil
}

// GrantDownload reports that local disk cannot hand out signed URLs.
func (l *Local) GrantDownload(ctx context.Context, key string, ttl time.Duration) (string, error) {
	if err := ValidateKey(key); err != nil {
		return "", err
	}
	return "", nil
}

// Package storage hides where catalog files physically live. The rest of the
// program only sees the Store interface; "local disk" and "S3-compatible
// bucket" are details of this package.
package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"time"
)

// ErrNotFound is returned when a key does not exist in the store.
var ErrNotFound = errors.New("storage: object not found")

// ErrInvalidKey is returned for keys that are empty, absolute, or that try to
// climb out of the store with ".." or hidden path segments.
var ErrInvalidKey = errors.New("storage: invalid object key")

// ObjectInfo describes a stored object.
type ObjectInfo struct {
	Size        int64
	ContentType string
	ModTime     time.Time // zero when the store does not report one
}

// Store is the narrow contract every object store adapter implements.
type Store interface {
	Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error
	Head(ctx context.Context, key string) (ObjectInfo, error) // size, content type; ErrNotFound
	Open(ctx context.Context, key string) (io.ReadSeekCloser, ObjectInfo, error)
	Delete(ctx context.Context, key string) error
	// GrantDownload returns a short-lived URL the client can fetch directly.
	// It returns ("", nil) when the store cannot sign URLs (local disk); the
	// API then streams the bytes itself.
	GrantDownload(ctx context.Context, key string, ttl time.Duration) (string, error)
}

// Options selects and configures an adapter.
type Options struct {
	Kind     string // "local" or "s3"
	LocalDir string // root folder for the local adapter

	S3Endpoint  string // e.g. https://<namespace>.compat.objectstorage.<region>.oraclecloud.com
	S3Region    string
	S3Bucket    string
	S3AccessKey string
	S3SecretKey string
}

// New builds the adapter named by opts.Kind.
func New(opts Options) (Store, error) {
	switch opts.Kind {
	case "", "local":
		return NewLocal(opts.LocalDir)
	case "s3":
		return NewS3(S3Config{
			Endpoint:  opts.S3Endpoint,
			Region:    opts.S3Region,
			Bucket:    opts.S3Bucket,
			AccessKey: opts.S3AccessKey,
			SecretKey: opts.S3SecretKey,
		})
	default:
		return nil, fmt.Errorf("storage: unknown kind %q (want \"local\" or \"s3\")", opts.Kind)
	}
}

// ValidateKey accepts only plain relative keys such as
// "catalog/9f2c41d07a3b/thumb.jpg". Both adapters call it, so a bad key is
// rejected the same way everywhere.
func ValidateKey(key string) error {
	if key == "" || len(key) > 512 {
		return ErrInvalidKey
	}
	if strings.ContainsAny(key, "\\\x00") || strings.HasPrefix(key, "/") || strings.HasSuffix(key, "/") {
		return ErrInvalidKey
	}
	if path.Clean(key) != key {
		return ErrInvalidKey
	}
	for _, seg := range strings.Split(key, "/") {
		// Rejects "", ".", ".." and hidden names (the local adapter keeps
		// its own bookkeeping in a hidden folder).
		if seg == "" || strings.HasPrefix(seg, ".") {
			return ErrInvalidKey
		}
		for _, r := range seg {
			if r < 0x20 || r == 0x7f {
				return ErrInvalidKey
			}
		}
	}
	return nil
}

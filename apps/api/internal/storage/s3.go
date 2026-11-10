package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// S3Config is everything needed to reach an S3-compatible bucket.
type S3Config struct {
	Endpoint  string // full URL, or host[:port] (https assumed)
	Region    string
	Bucket    string
	AccessKey string
	SecretKey string
}

// S3 talks to any S3-compatible endpoint. It is set up the way Oracle Cloud
// Object Storage's S3 compatibility API needs: path-style addressing
// (https://host/bucket/key), Signature Version 4, a fixed region (so no
// bucket-location lookups), single-request uploads without the newer
// trailing-checksum scheme, and a private bucket read through presigned URLs.
type S3 struct {
	client *minio.Client
	bucket string
}

// NewS3 validates the config and builds the client. It does not contact the
// endpoint; the first real call does.
func NewS3(cfg S3Config) (*S3, error) {
	var missing []string
	for name, v := range map[string]string{
		"WALLS_S3_ENDPOINT":   cfg.Endpoint,
		"WALLS_S3_REGION":     cfg.Region,
		"WALLS_S3_BUCKET":     cfg.Bucket,
		"WALLS_S3_ACCESS_KEY": cfg.AccessKey,
		"WALLS_S3_SECRET_KEY": cfg.SecretKey,
	} {
		if strings.TrimSpace(v) == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		slices.Sort(missing)
		return nil, fmt.Errorf("storage: s3 needs %s to be set", strings.Join(missing, ", "))
	}

	host, secure, err := splitEndpoint(cfg.Endpoint)
	if err != nil {
		return nil, err
	}
	client, err := minio.New(host, &minio.Options{
		Creds:        credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure:       secure,
		Region:       cfg.Region,
		BucketLookup: minio.BucketLookupPath,
	})
	if err != nil {
		return nil, fmt.Errorf("storage: s3 client: %w", err)
	}
	return &S3{client: client, bucket: cfg.Bucket}, nil
}

func splitEndpoint(endpoint string) (host string, secure bool, err error) {
	endpoint = strings.TrimSpace(endpoint)
	if !strings.Contains(endpoint, "://") {
		return strings.TrimSuffix(endpoint, "/"), true, nil
	}
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" {
		return "", false, fmt.Errorf("storage: WALLS_S3_ENDPOINT is not a valid URL")
	}
	if u.Path != "" && u.Path != "/" {
		return "", false, fmt.Errorf("storage: WALLS_S3_ENDPOINT must not contain a path (the bucket goes in WALLS_S3_BUCKET)")
	}
	switch u.Scheme {
	case "https":
		return u.Host, true, nil
	case "http":
		return u.Host, false, nil
	}
	return "", false, fmt.Errorf("storage: WALLS_S3_ENDPOINT must start with https:// or http://")
}

// notFound reports whether err means "no such object".
func notFound(err error) bool {
	if err == nil {
		return false
	}
	resp := minio.ToErrorResponse(err)
	return resp.Code == minio.NoSuchKey || resp.StatusCode == http.StatusNotFound
}

func (s *S3) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	if err := ValidateKey(key); err != nil {
		return err
	}
	if size < 0 {
		return errors.New("storage: s3 uploads need a known size")
	}
	_, err := s.client.PutObject(ctx, s.bucket, key, r, size, minio.PutObjectOptions{
		ContentType: contentType,
		// One plain PUT with a Content-MD5 header: the most widely supported
		// upload shape, and the one OCI's compatibility API accepts.
		DisableMultipart:     true,
		DisableContentSha256: true,
		SendContentMd5:       true,
	})
	if err != nil {
		return fmt.Errorf("storage: put %s: %w", key, err)
	}
	return nil
}

func (s *S3) Head(ctx context.Context, key string) (ObjectInfo, error) {
	if err := ValidateKey(key); err != nil {
		return ObjectInfo{}, err
	}
	st, err := s.client.StatObject(ctx, s.bucket, key, minio.StatObjectOptions{})
	if err != nil {
		if notFound(err) {
			return ObjectInfo{}, ErrNotFound
		}
		return ObjectInfo{}, fmt.Errorf("storage: head %s: %w", key, err)
	}
	return ObjectInfo{Size: st.Size, ContentType: st.ContentType, ModTime: st.LastModified}, nil
}

// Open returns a seekable reader that fetches ranges from the bucket on
// demand.
func (s *S3) Open(ctx context.Context, key string) (io.ReadSeekCloser, ObjectInfo, error) {
	if err := ValidateKey(key); err != nil {
		return nil, ObjectInfo{}, err
	}
	obj, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, ObjectInfo{}, fmt.Errorf("storage: open %s: %w", key, err)
	}
	st, err := obj.Stat() // GetObject is lazy; this is the first real request
	if err != nil {
		obj.Close()
		if notFound(err) {
			return nil, ObjectInfo{}, ErrNotFound
		}
		return nil, ObjectInfo{}, fmt.Errorf("storage: open %s: %w", key, err)
	}
	return obj, ObjectInfo{Size: st.Size, ContentType: st.ContentType, ModTime: st.LastModified}, nil
}

func (s *S3) Delete(ctx context.Context, key string) error {
	if err := ValidateKey(key); err != nil {
		return err
	}
	err := s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{})
	if err != nil && !notFound(err) {
		return fmt.Errorf("storage: delete %s: %w", key, err)
	}
	return nil
}

// GrantDownload presigns a GET for the object. Signing is done locally; no
// request is sent to the bucket.
func (s *S3) GrantDownload(ctx context.Context, key string, ttl time.Duration) (string, error) {
	if err := ValidateKey(key); err != nil {
		return "", err
	}
	u, err := s.client.PresignedGetObject(ctx, s.bucket, key, ttl, nil)
	if err != nil {
		return "", fmt.Errorf("storage: presign %s: %w", key, err)
	}
	return u.String(), nil
}

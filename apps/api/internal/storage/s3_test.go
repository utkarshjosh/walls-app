package storage

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeS3 is a tiny in-memory stand-in for an S3-compatible endpoint. It does
// NOT check request signatures, so these tests prove the adapter sends the
// request shapes we intend (path-style URLs, one plain PUT with a Content-MD5,
// no chunked-signature or trailing-checksum encoding) and maps responses
// correctly. They do not prove a real provider accepts our signatures; the
// env-guarded test below is for that.
type fakeS3 struct {
	t       *testing.T
	bucket  string
	mu      sync.Mutex
	objects map[string]fakeObject
	puts    []*http.Request
}

type fakeObject struct {
	body        []byte
	contentType string
}

func (f *fakeS3) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	prefix := "/" + f.bucket + "/"
	if !strings.HasPrefix(r.URL.Path, prefix) {
		f.t.Errorf("request was not path-style for bucket %q: %s %s (host %s)", f.bucket, r.Method, r.URL.Path, r.Host)
		http.Error(w, "bad path", http.StatusBadRequest)
		return
	}
	key := strings.TrimPrefix(r.URL.Path, prefix)
	f.mu.Lock()
	defer f.mu.Unlock()

	notFound := func() {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusNotFound)
		if r.Method != http.MethodHead {
			io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?><Error><Code>NoSuchKey</Code><Message>The specified key does not exist.</Message></Error>`)
		}
	}

	switch r.Method {
	case http.MethodPut:
		body, _ := io.ReadAll(r.Body)
		sum := md5.Sum(body)
		if got := r.Header.Get("Content-Md5"); got != base64.StdEncoding.EncodeToString(sum[:]) {
			f.t.Errorf("PUT %s: Content-MD5 %q does not match the body", key, got)
		}
		f.puts = append(f.puts, r.Clone(context.Background()))
		f.objects[key] = fakeObject{body: body, contentType: r.Header.Get("Content-Type")}
		w.Header().Set("ETag", `"fake"`)
	case http.MethodHead, http.MethodGet:
		obj, ok := f.objects[key]
		if !ok {
			notFound()
			return
		}
		if r.Method == http.MethodGet && r.URL.Query().Get("X-Amz-Signature") == "" && r.Header.Get("Authorization") == "" {
			http.Error(w, "unsigned request", http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", obj.contentType)
		w.Header().Set("ETag", `"fake"`)
		http.ServeContent(w, r, "", time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC), bytes.NewReader(obj.body))
	case http.MethodDelete:
		delete(f.objects, key)
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
	}
}

func TestS3AgainstFakeEndpoint(t *testing.T) {
	fake := &fakeS3{t: t, bucket: "walls-assets", objects: map[string]fakeObject{}}
	srv := httptest.NewServer(fake)
	defer srv.Close()

	store, err := NewS3(S3Config{
		Endpoint: srv.URL, Region: "ap-mumbai-1", Bucket: "walls-assets", AccessKey: "test-access", SecretKey: "test-secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	roundTrip(t, store, func(url string) {
		if !strings.HasPrefix(url, srv.URL+"/walls-assets/catalog/") {
			t.Errorf("presigned URL is not path-style: %s", url)
		}
		for _, want := range []string{"X-Amz-Algorithm=AWS4-HMAC-SHA256", "X-Amz-Expires=900", "X-Amz-Signature=", "ap-mumbai-1"} {
			if !strings.Contains(url, want) {
				t.Errorf("presigned URL lacks %q: %s", want, url)
			}
		}
		if strings.Contains(url, "test-secret") {
			t.Error("presigned URL leaks the secret key")
		}
	})

	if len(fake.puts) == 0 {
		t.Fatal("no PUT reached the fake endpoint")
	}
	for _, r := range fake.puts {
		if enc := r.Header.Get("Content-Encoding"); strings.Contains(enc, "aws-chunked") {
			t.Errorf("PUT used aws-chunked encoding, which OCI's S3 API does not accept")
		}
		if sha := r.Header.Get("X-Amz-Content-Sha256"); strings.HasPrefix(sha, "STREAMING-") || strings.HasSuffix(sha, "-TRAILER") {
			t.Errorf("PUT used payload mode %q; expected a plain single-request upload", sha)
		}
		for name := range r.Header {
			if strings.HasPrefix(strings.ToLower(name), "x-amz-checksum") || strings.EqualFold(name, "X-Amz-Trailer") {
				t.Errorf("PUT sent %s; the newer checksum scheme must stay off", name)
			}
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 ") {
			t.Errorf("PUT was not signed with Signature Version 4")
		}
	}
}

// TestS3Integration runs the same round trip against a real S3-compatible
// endpoint (MinIO, or the OCI bucket itself). It is skipped unless
// WALLS_TEST_S3_ENDPOINT is set, along with WALLS_TEST_S3_REGION,
// WALLS_TEST_S3_BUCKET, WALLS_TEST_S3_ACCESS_KEY and WALLS_TEST_S3_SECRET_KEY.
// The bucket must already exist; the test only touches keys under
// "catalog/selftest0000/".
func TestS3Integration(t *testing.T) {
	endpoint := os.Getenv("WALLS_TEST_S3_ENDPOINT")
	if endpoint == "" {
		t.Skip("set WALLS_TEST_S3_ENDPOINT (and friends) to test against a live S3-compatible endpoint")
	}
	store, err := NewS3(S3Config{
		Endpoint:  endpoint,
		Region:    os.Getenv("WALLS_TEST_S3_REGION"),
		Bucket:    os.Getenv("WALLS_TEST_S3_BUCKET"),
		AccessKey: os.Getenv("WALLS_TEST_S3_ACCESS_KEY"),
		SecretKey: os.Getenv("WALLS_TEST_S3_SECRET_KEY"),
	})
	if err != nil {
		t.Fatal(err)
	}
	roundTrip(t, store, func(string) {})
}

// roundTrip exercises Put, Head, Open (with a seek), GrantDownload (fetching
// the URL) and Delete.
func roundTrip(t *testing.T, store Store, checkURL func(url string)) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	const key = "catalog/selftest0000/original.html"
	body := []byte("<html><body>" + strings.Repeat("walls ", 2000) + "</body></html>")

	_ = store.Delete(ctx, key)
	if _, err := store.Head(ctx, key); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Head of a missing key: got %v, want ErrNotFound", err)
	}
	if _, _, err := store.Open(ctx, key); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Open of a missing key: got %v, want ErrNotFound", err)
	}

	if err := store.Put(ctx, key, bytes.NewReader(body), int64(len(body)), "text/html; charset=utf-8"); err != nil {
		t.Fatalf("Put: %v", err)
	}
	info, err := store.Head(ctx, key)
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	if info.Size != int64(len(body)) || info.ContentType != "text/html; charset=utf-8" {
		t.Fatalf("Head = %+v, want size %d and the HTML content type", info, len(body))
	}

	f, info, err := store.Open(ctx, key)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if info.Size != int64(len(body)) {
		t.Fatalf("Open info = %+v", info)
	}
	if _, err := f.Seek(-14, io.SeekEnd); err != nil {
		t.Fatalf("Seek: %v", err)
	}
	tail, err := io.ReadAll(f)
	f.Close()
	if err != nil || string(tail) != "</body></html>" {
		t.Fatalf("read after seek = %q, %v", tail, err)
	}

	url, err := store.GrantDownload(ctx, key, 15*time.Minute)
	if err != nil || url == "" {
		t.Fatalf("GrantDownload = %q, %v", url, err)
	}
	checkURL(url)
	resp, err := http.Get(url) // a plain client with no credentials, like a browser
	if err != nil {
		t.Fatalf("fetching the granted URL: %v", err)
	}
	got, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !bytes.Equal(got, body) {
		t.Fatalf("granted URL returned status %d and %d bytes, want 200 and %d bytes", resp.StatusCode, len(got), len(body))
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Errorf("granted URL served Content-Type %q", ct)
	}

	if err := store.Delete(ctx, key); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := store.Head(ctx, key); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Head after Delete: got %v, want ErrNotFound", err)
	}
	if err := store.Delete(ctx, key); err != nil {
		t.Fatalf("deleting a missing key should not fail, got %v", err)
	}
	if err := store.Put(ctx, "../escape", strings.NewReader("x"), 1, "text/plain"); !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("Put with a traversal key = %v, want ErrInvalidKey", err)
	}
}

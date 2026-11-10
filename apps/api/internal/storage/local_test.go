package storage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLocalRoundTrip(t *testing.T) {
	ctx := context.Background()
	store, err := NewLocal(filepath.Join(t.TempDir(), "objects"))
	if err != nil {
		t.Fatal(err)
	}
	const key = "catalog/9f2c41d07a3b/original.html"
	body := []byte("<html>hello</html>")

	if _, err := store.Head(ctx, key); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Head before Put: got %v, want ErrNotFound", err)
	}
	if _, _, err := store.Open(ctx, key); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Open before Put: got %v, want ErrNotFound", err)
	}
	if err := store.Put(ctx, key, bytes.NewReader(body), int64(len(body)), "text/html; charset=utf-8"); err != nil {
		t.Fatal(err)
	}

	info, err := store.Head(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size != int64(len(body)) || info.ContentType != "text/html; charset=utf-8" {
		t.Fatalf("Head = %+v", info)
	}

	f, info, err := store.Open(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Seek(6, io.SeekStart); err != nil { // seeking is what Range requests rely on
		t.Fatal(err)
	}
	rest, _ := io.ReadAll(f)
	f.Close()
	if string(rest) != "hello</html>" || info.Size != int64(len(body)) {
		t.Fatalf("Open read %q, info %+v", rest, info)
	}

	// Overwriting replaces the object.
	if err := store.Put(ctx, key, strings.NewReader("v2"), 2, "text/plain"); err != nil {
		t.Fatal(err)
	}
	if info, _ := store.Head(ctx, key); info.Size != 2 || info.ContentType != "text/plain" {
		t.Fatalf("after overwrite Head = %+v", info)
	}

	url, err := store.GrantDownload(ctx, key, time.Minute)
	if err != nil || url != "" {
		t.Fatalf("GrantDownload = %q, %v; local disk must return an empty URL", url, err)
	}

	if err := store.Delete(ctx, key); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Head(ctx, key); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Head after Delete: got %v, want ErrNotFound", err)
	}
	if err := store.Delete(ctx, key); err != nil {
		t.Fatalf("deleting a missing key should be a no-op, got %v", err)
	}
}

func TestLocalRejectsWrongSize(t *testing.T) {
	store, _ := NewLocal(t.TempDir())
	err := store.Put(context.Background(), "catalog/a/b.jpg", strings.NewReader("abc"), 10, "image/jpeg")
	if err == nil {
		t.Fatal("Put with a wrong size should fail")
	}
	if _, err := store.Head(context.Background(), "catalog/a/b.jpg"); !errors.Is(err, ErrNotFound) {
		t.Fatal("a failed Put must not leave an object behind")
	}
}

func TestLocalRejectsPathTraversal(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "objects")
	store, err := NewLocal(root)
	if err != nil {
		t.Fatal(err)
	}
	// A file outside the store that a traversal would reach.
	secret := filepath.Join(filepath.Dir(root), "secret.txt")
	if err := os.WriteFile(secret, []byte("keep out"), 0o600); err != nil {
		t.Fatal(err)
	}

	bad := []string{
		"", "/etc/passwd", "../secret.txt", "catalog/../../secret.txt", "catalog/./x", "catalog//x",
		"catalog/x/", `catalog\..\secret.txt`, "catalog/..", "..", ".", ".meta/x", "catalog/.hidden",
		"catalog/a\x00b", "catalog/a\nb", strings.Repeat("a", 600),
	}
	for _, key := range bad {
		if err := store.Put(ctx, key, strings.NewReader("x"), 1, "text/plain"); !errors.Is(err, ErrInvalidKey) {
			t.Errorf("Put(%q) = %v, want ErrInvalidKey", key, err)
		}
		if _, err := store.Head(ctx, key); !errors.Is(err, ErrInvalidKey) {
			t.Errorf("Head(%q) = %v, want ErrInvalidKey", key, err)
		}
		if _, _, err := store.Open(ctx, key); !errors.Is(err, ErrInvalidKey) {
			t.Errorf("Open(%q) = %v, want ErrInvalidKey", key, err)
		}
		if err := store.Delete(ctx, key); !errors.Is(err, ErrInvalidKey) {
			t.Errorf("Delete(%q) = %v, want ErrInvalidKey", key, err)
		}
		if _, err := store.GrantDownload(ctx, key, time.Minute); !errors.Is(err, ErrInvalidKey) {
			t.Errorf("GrantDownload(%q) = %v, want ErrInvalidKey", key, err)
		}
	}
	if got, _ := os.ReadFile(secret); string(got) != "keep out" {
		t.Fatal("a file outside the store was modified")
	}
}

func TestNewRejectsUnknownKind(t *testing.T) {
	if _, err := New(Options{Kind: "ftp"}); err == nil {
		t.Fatal("unknown storage kind should be an error")
	}
	if _, err := New(Options{Kind: "s3"}); err == nil || !strings.Contains(err.Error(), "WALLS_S3_BUCKET") {
		t.Fatalf("s3 without settings should name what is missing, got %v", err)
	}
}

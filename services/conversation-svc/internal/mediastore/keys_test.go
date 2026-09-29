package mediastore

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
)

func TestDiskStoreRejectsNonFileNameKeys(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store, err := NewDiskStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	for _, key := range []string{"", ".", "..", "../escape", "a/b", `a\b`, "/abs", "nul\x00byte"} {
		if err := store.Put(ctx, key, bytes.NewReader([]byte("x")), 1, "text/plain"); !errors.Is(err, ErrInvalidKey) {
			t.Errorf("Put(%q) error = %v, want ErrInvalidKey", key, err)
		}
		if _, err := store.Get(ctx, key); !errors.Is(err, ErrInvalidKey) {
			t.Errorf("Get(%q) error = %v, want ErrInvalidKey", key, err)
		}
		if _, err := store.Exists(ctx, key); !errors.Is(err, ErrInvalidKey) {
			t.Errorf("Exists(%q) error = %v, want ErrInvalidKey", key, err)
		}
		if err := store.Delete(ctx, key); !errors.Is(err, ErrInvalidKey) {
			t.Errorf("Delete(%q) error = %v, want ErrInvalidKey", key, err)
		}
	}
}

func TestDiskStoreDistinctKeysDoNotCollide(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store, err := NewDiskStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// "a/b" used to be reduced to "b" and overwrite the object stored as "b".
	if err := store.Put(ctx, "b", bytes.NewReader([]byte("first")), 5, ""); err != nil {
		t.Fatal(err)
	}
	if err := store.Put(ctx, "a/b", bytes.NewReader([]byte("second")), 6, ""); !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("Put(a/b) error = %v, want ErrInvalidKey", err)
	}
	reader, err := store.Get(ctx, "b")
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	data, _ := io.ReadAll(reader)
	if string(data) != "first" {
		t.Errorf("object b = %q, want first", data)
	}
}

func TestParseS3Endpoint(t *testing.T) {
	t.Parallel()
	tests := []struct {
		endpoint   string
		useSSL     bool
		wantHost   string
		wantSecure bool
	}{
		{"minio:9000", false, "minio:9000", false},
		{"minio:9000", true, "minio:9000", true},
		{"https://minio.example.com", false, "minio.example.com", true},
		{"http://minio:9000", true, "minio:9000", false},
	}
	for _, tc := range tests {
		host, secure := parseS3Endpoint(tc.endpoint, tc.useSSL)
		if host != tc.wantHost || secure != tc.wantSecure {
			t.Errorf("parseS3Endpoint(%q, %v) = %q, %v; want %q, %v", tc.endpoint, tc.useSSL, host, secure, tc.wantHost, tc.wantSecure)
		}
	}
}

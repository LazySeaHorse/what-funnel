package mediastore

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestDiskStoreLifecycle(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("invalid root", func(t *testing.T) {
		_, err := NewDiskStore("   ")
		if err == nil {
			t.Fatal("expected error for empty root, got nil")
		}
	})

	t.Run("crud operations", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "media", "cache")
		store, err := NewDiskStore(root)
		if err != nil {
			t.Fatalf("NewDiskStore() error = %v", err)
		}

		key := "test-object-key-123"
		content := []byte("hello world media content")

		// 1. Initially not exists
		exists, err := store.Exists(ctx, key)
		if err != nil {
			t.Fatalf("Exists() error = %v", err)
		}
		if exists {
			t.Fatal("expected key to not exist")
		}

		_, err = store.Get(ctx, key)
		if err != ErrNotFound {
			t.Fatalf("Get() non-existent key error = %v, want ErrNotFound", err)
		}

		// 2. Put
		err = store.Put(ctx, key, bytes.NewReader(content), int64(len(content)), "text/plain")
		if err != nil {
			t.Fatalf("Put() error = %v", err)
		}

		// Check file permissions on disk
		filePath := filepath.Join(root, key)
		info, err := os.Stat(filePath)
		if err != nil {
			t.Fatalf("stat written file: %v", err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Errorf("file mode = %o, want 600", info.Mode().Perm())
		}

		// 3. Exists
		exists, err = store.Exists(ctx, key)
		if err != nil {
			t.Fatalf("Exists() error = %v", err)
		}
		if !exists {
			t.Fatal("expected key to exist after Put")
		}

		// 4. Get
		reader, err := store.Get(ctx, key)
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		defer reader.Close()

		data, err := io.ReadAll(reader)
		if err != nil {
			t.Fatalf("read content error = %v", err)
		}
		if string(data) != string(content) {
			t.Fatalf("got content %q, want %q", string(data), string(content))
		}

		// 5. Delete
		err = store.Delete(ctx, key)
		if err != nil {
			t.Fatalf("Delete() error = %v", err)
		}

		exists, err = store.Exists(ctx, key)
		if err != nil {
			t.Fatalf("Exists() after delete error = %v", err)
		}
		if exists {
			t.Fatal("expected key to not exist after Delete")
		}

		_, err = store.Get(ctx, key)
		if err != ErrNotFound {
			t.Fatalf("Get() after delete error = %v, want ErrNotFound", err)
		}

		// 6. Delete idempotent
		err = store.Delete(ctx, key)
		if err != nil {
			t.Fatalf("idempotent Delete() error = %v", err)
		}
	})
}

func TestS3ConfigValidation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("missing endpoint", func(t *testing.T) {
		_, err := NewS3Store(ctx, S3Config{
			Bucket: "test-bucket",
		})
		if err == nil {
			t.Fatal("expected error for empty endpoint, got nil")
		}
	})

	t.Run("missing bucket", func(t *testing.T) {
		_, err := NewS3Store(ctx, S3Config{
			Endpoint: "localhost:9000",
		})
		if err == nil {
			t.Fatal("expected error for empty bucket, got nil")
		}
	})
}

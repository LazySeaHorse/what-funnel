package service

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
)

func TestMediaCacheWritesPrivateFile(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "nested", "media")
	service := &Service{}
	if err := service.ConfigureMediaCache(root); err != nil {
		t.Fatalf("ConfigureMediaCache() error = %v", err)
	}
	id := uuid.New()
	key, err := service.writeMediaFile(id, []byte("content"))
	if err != nil {
		t.Fatalf("writeMediaFile() error = %v", err)
	}
	if key != id.String() {
		t.Errorf("storage key = %q, want %q", key, id)
	}
	file, err := os.Open(service.mediaPath("../" + key))
	if err != nil {
		t.Fatalf("open cached file: %v", err)
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil || string(data) != "content" {
		t.Fatalf("cached data = %q, error = %v", data, err)
	}
	info, err := file.Stat()
	if err != nil {
		t.Fatalf("stat cached file: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("file mode = %o, want 600", info.Mode().Perm())
	}
}

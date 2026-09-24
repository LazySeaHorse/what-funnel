package service

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/whatfunnel/whatfunnel/services/conversation-svc/internal/mediastore"
)

type memoryMediaStore struct {
	mu    sync.RWMutex
	files map[string][]byte
}

func newMemoryMediaStore() *memoryMediaStore {
	return &memoryMediaStore{files: make(map[string][]byte)}
}

func (m *memoryMediaStore) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	m.mu.Lock()
	m.files[key] = data
	m.mu.Unlock()
	return nil
}

func (m *memoryMediaStore) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	m.mu.RLock()
	data, ok := m.files[key]
	m.mu.RUnlock()
	if !ok {
		return nil, mediastore.ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (m *memoryMediaStore) Delete(ctx context.Context, key string) error {
	m.mu.Lock()
	delete(m.files, key)
	m.mu.Unlock()
	return nil
}

func (m *memoryMediaStore) Exists(ctx context.Context, key string) (bool, error) {
	m.mu.RLock()
	_, ok := m.files[key]
	m.mu.RUnlock()
	return ok, nil
}

func TestMediaCacheWritesPrivateFile(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "nested", "media")
	svc := NewMediaService(nil)
	if err := svc.ConfigureMediaCache(root); err != nil {
		t.Fatalf("ConfigureMediaCache() error = %v", err)
	}
	id := uuid.New()
	key, err := svc.writeMediaFile(id, []byte("content"))
	if err != nil {
		t.Fatalf("writeMediaFile() error = %v", err)
	}
	if key != id.String() {
		t.Errorf("storage key = %q, want %q", key, id)
	}
	file, err := os.Open(svc.mediaPath("../" + key))
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

func TestMediaServiceWithCustomMediaStore(t *testing.T) {
	t.Parallel()

	svc := NewMediaService(nil)

	// Unconfigured store returns error
	_, err := svc.writeMediaFile(uuid.New(), []byte("data"))
	if err == nil {
		t.Fatal("expected error when media store is unconfigured")
	}

	// Configure custom store
	memStore := newMemoryMediaStore()
	svc.ConfigureMediaStore(memStore)

	id := uuid.New()
	content := []byte("hello custom media store")
	key, err := svc.writeMediaFile(id, content)
	if err != nil {
		t.Fatalf("writeMediaFile() error = %v", err)
	}
	if key != id.String() {
		t.Fatalf("expected key %s, got %s", id.String(), key)
	}

	exists, err := memStore.Exists(context.Background(), key)
	if err != nil || !exists {
		t.Fatalf("expected key to exist in memStore, exists=%v err=%v", exists, err)
	}

	reader, err := memStore.Get(context.Background(), key)
	if err != nil {
		t.Fatalf("memStore.Get() error = %v", err)
	}
	defer reader.Close()

	readData, err := io.ReadAll(reader)
	if err != nil || !bytes.Equal(readData, content) {
		t.Fatalf("got data %q, want %q", readData, content)
	}
}

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
	key, err := svc.writeMediaFile(context.Background(), id, []byte("content"), "text/plain")
	if err != nil {
		t.Fatalf("writeMediaFile() error = %v", err)
	}
	if key != id.String() {
		t.Errorf("storage key = %q, want %q", key, id)
	}
	file, err := os.Open(filepath.Join(root, key))
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
	_, err := svc.writeMediaFile(context.Background(), uuid.New(), []byte("data"), "")
	if err == nil {
		t.Fatal("expected error when media store is unconfigured")
	}

	// Configure custom store
	memStore := newMemoryMediaStore()
	svc.ConfigureMediaStore(memStore)

	id := uuid.New()
	content := []byte("hello custom media store")
	key, err := svc.writeMediaFile(context.Background(), id, content, "text/plain")
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

func TestIsSafeInlineMIMEType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		mimeType string
		wantSafe bool
	}{
		// Safe raster images
		{"image/jpeg", true},
		{"image/png", true},
		{"image/webp", true},
		{"image/gif", true},
		// Safe audio
		{"audio/ogg", true},
		{"audio/mpeg", true},
		{"audio/mp4", true},
		{"audio/wav", true},
		{"audio/webm", true},
		{"audio/aac", true},
		{"audio/flac", true},
		{"audio/x-m4a", true},
		// Safe video
		{"video/mp4", true},
		{"video/webm", true},
		{"video/ogg", true},
		// Case insensitivity & media parameters
		{"IMAGE/PNG", true},
		{"image/jpeg; charset=utf-8", true},
		{"video/mp4; codecs=\"avc1.42E01E, mp4a.40.2\"", true},

		// Dangerous active / executable / scriptable types (Stored XSS risks)
		{"image/svg+xml", false},
		{"image/svg+xml; charset=utf-8", false},
		{"image/svg", false},
		{"text/html", false},
		{"text/html; charset=utf-8", false},
		{"application/xhtml+xml", false},
		{"text/xml", false},
		{"application/xml", false},
		{"application/pdf", false},
		{"application/javascript", false},
		{"text/javascript", false},
		{"text/plain", false},
		{"application/octet-stream", false},
		{"application/x-shockwave-flash", false},

		// Empty, malformed, or unknown
		{"", false},
		{"   ", false},
		{"unknown/type", false},
		{"invalid;;;type", false},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.mimeType, func(t *testing.T) {
			t.Parallel()
			got := IsSafeInlineMIMEType(tc.mimeType)
			if got != tc.wantSafe {
				t.Errorf("IsSafeInlineMIMEType(%q) = %v, want %v", tc.mimeType, got, tc.wantSafe)
			}
		})
	}
}

func TestMediaContentDisposition(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		mimeType        string
		filename        string
		wantDisposition string
	}{
		{
			name:            "safe raster image with filename",
			mimeType:        "image/png",
			filename:        "photo.png",
			wantDisposition: "inline; filename=photo.png",
		},
		{
			name:            "safe raster image without filename",
			mimeType:        "image/jpeg",
			filename:        "",
			wantDisposition: "inline",
		},
		{
			name:            "dangerous SVG with filename must be attachment",
			mimeType:        "image/svg+xml",
			filename:        "exploit.svg",
			wantDisposition: "attachment; filename=exploit.svg",
		},
		{
			name:            "dangerous SVG without filename must be attachment",
			mimeType:        "image/svg+xml",
			filename:        "",
			wantDisposition: "attachment",
		},
		{
			name:            "dangerous HTML with filename must be attachment",
			mimeType:        "text/html",
			filename:        "index.html",
			wantDisposition: "attachment; filename=index.html",
		},
		{
			name:            "dangerous PDF with filename must be attachment",
			mimeType:        "application/pdf",
			filename:        "report.pdf",
			wantDisposition: "attachment; filename=report.pdf",
		},
		{
			name:            "unknown binary with filename must be attachment",
			mimeType:        "application/octet-stream",
			filename:        "data.bin",
			wantDisposition: "attachment; filename=data.bin",
		},
		{
			name:            "path traversal in filename is sanitized to base name",
			mimeType:        "image/svg+xml",
			filename:        "../../../../malicious.svg",
			wantDisposition: "attachment; filename=malicious.svg",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := MediaContentDisposition(tc.mimeType, tc.filename)
			if got != tc.wantDisposition {
				t.Errorf("MediaContentDisposition(%q, %q) = %q, want %q", tc.mimeType, tc.filename, got, tc.wantDisposition)
			}
		})
	}
}

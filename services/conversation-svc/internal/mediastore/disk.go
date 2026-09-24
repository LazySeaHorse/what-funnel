package mediastore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// DiskStore implements Store using the local filesystem.
type DiskStore struct {
	root string
	mu   sync.RWMutex
}

// NewDiskStore initializes and secures a local directory for media storage.
func NewDiskStore(root string) (*DiskStore, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, errors.New("mediastore: root directory is required")
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, fmt.Errorf("mediastore: create media cache: %w", err)
	}
	if err := os.Chmod(root, 0o700); err != nil {
		return nil, fmt.Errorf("mediastore: secure media cache: %w", err)
	}
	return &DiskStore{root: root}, nil
}

// Put writes an object atomically with private permissions (0600).
func (d *DiskStore) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	d.mu.RLock()
	root := d.root
	d.mu.RUnlock()

	tempFile, err := os.CreateTemp(root, ".media-*")
	if err != nil {
		return fmt.Errorf("mediastore: create temp file: %w", err)
	}
	tempName := tempFile.Name()
	defer os.Remove(tempName)

	if err := tempFile.Chmod(0o600); err != nil {
		_ = tempFile.Close()
		return fmt.Errorf("mediastore: secure temp file: %w", err)
	}

	if _, err := io.Copy(tempFile, r); err != nil {
		_ = tempFile.Close()
		return fmt.Errorf("mediastore: write temp file: %w", err)
	}

	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("mediastore: close temp file: %w", err)
	}

	target := d.path(key)
	if err := os.Rename(tempName, target); err != nil {
		return fmt.Errorf("mediastore: publish file: %w", err)
	}
	return nil
}

// Get returns an io.ReadCloser for the given key, or ErrNotFound if missing.
func (d *DiskStore) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	target := d.path(key)
	file, err := os.Open(target)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("mediastore: open file: %w", err)
	}
	return file, nil
}

// Delete removes the file for the given key.
func (d *DiskStore) Delete(ctx context.Context, key string) error {
	target := d.path(key)
	err := os.Remove(target)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("mediastore: remove file: %w", err)
	}
	return nil
}

// Exists checks if the file exists on disk.
func (d *DiskStore) Exists(ctx context.Context, key string) (bool, error) {
	target := d.path(key)
	_, err := os.Stat(target)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, fmt.Errorf("mediastore: stat file: %w", err)
}

func (d *DiskStore) path(key string) string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return filepath.Join(d.root, filepath.Base(key))
}

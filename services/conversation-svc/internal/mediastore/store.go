package mediastore

import (
	"context"
	"errors"
	"io"
)

var (
	// ErrNotFound is returned when an object is not found in the media store.
	ErrNotFound = errors.New("mediastore: object not found")
)

// Store defines the storage operations required for conversation media.
type Store interface {
	Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error
	Get(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
	Exists(ctx context.Context, key string) (bool, error)
}

package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
)

// recordingStore fails Delete for chosen keys and records Put content types.
type recordingStore struct {
	mu           sync.Mutex
	failDelete   map[string]bool
	deleted      []string
	contentTypes map[string]string
}

func newRecordingStore() *recordingStore {
	return &recordingStore{failDelete: map[string]bool{}, contentTypes: map[string]string{}}
}

func (s *recordingStore) Put(_ context.Context, key string, r io.Reader, _ int64, contentType string) error {
	if _, err := io.Copy(io.Discard, r); err != nil {
		return err
	}
	s.mu.Lock()
	s.contentTypes[key] = contentType
	s.mu.Unlock()
	return nil
}

func (s *recordingStore) Get(context.Context, string) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(nil)), nil
}

func (s *recordingStore) Delete(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failDelete[key] {
		return errors.New("store unavailable")
	}
	s.deleted = append(s.deleted, key)
	return nil
}

func (s *recordingStore) Exists(context.Context, string) (bool, error) { return true, nil }

func TestCleanupExpiredMediaOnce_ContinuesPastDeleteFailures(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	f := newOutboxFixture(t)
	ctx := context.Background()
	svc := NewMediaService(f.pool)
	store := newRecordingStore()
	svc.ConfigureMediaStore(store)

	insert := func(key, providerRef, expiredAgo string) uuid.UUID {
		id := uuid.New()
		if _, err := f.pool.Exec(ctx, `
			INSERT INTO media_objects (id, account_id, channel_id, mime_type, size_bytes, storage_key, provider_ref, expires_at)
			VALUES ($1, $2, $3, 'image/png', 1, $4, NULLIF($5, ''), NOW() - $6::INTERVAL)
		`, id, f.accountID, f.channelID, key, providerRef, expiredAgo); err != nil {
			t.Fatal(err)
		}
		return id
	}
	failingID := insert("stuck-object", "ref-1", "3 hours") // oldest, so it is processed first
	cachedID := insert("cached-object", "ref-2", "2 hours") // re-downloadable: keeps its row
	uploadID := insert("upload-object", "", "1 hour")       // no provider copy: row is removed
	store.failDelete["stuck-object"] = true

	err := svc.CleanupExpiredMediaOnce(ctx)
	if err == nil || !strings.Contains(err.Error(), failingID.String()) {
		t.Fatalf("err = %v, want it to report the object that failed to delete", err)
	}
	if len(store.deleted) != 2 {
		t.Fatalf("deleted = %v, want both healthy objects removed despite the earlier failure", store.deleted)
	}

	var stuckKey, cachedKey *string
	if err := f.pool.QueryRow(ctx, `SELECT storage_key FROM media_objects WHERE id = $1`, failingID).Scan(&stuckKey); err != nil {
		t.Fatal(err)
	}
	if stuckKey == nil {
		t.Error("failed object lost its storage_key; it must stay eligible for the next run")
	}
	if err := f.pool.QueryRow(ctx, `SELECT storage_key FROM media_objects WHERE id = $1`, cachedID).Scan(&cachedKey); err != nil {
		t.Fatal(err)
	}
	if cachedKey != nil {
		t.Error("expired provider-backed object still has a storage_key")
	}
	var uploads int
	if err := f.pool.QueryRow(ctx, `SELECT COUNT(*) FROM media_objects WHERE id = $1`, uploadID).Scan(&uploads); err != nil {
		t.Fatal(err)
	}
	if uploads != 0 {
		t.Error("expired upload row should be removed once its blob is deleted")
	}
}

func TestWriteMediaFilePassesContentTypeAndContext(t *testing.T) {
	t.Parallel()
	svc := NewMediaService(nil)
	store := newRecordingStore()
	svc.ConfigureMediaStore(store)

	id := uuid.New()
	if _, err := svc.writeMediaFile(context.Background(), id, []byte("x"), "image/png"); err != nil {
		t.Fatal(err)
	}
	if got := store.contentTypes[id.String()]; got != "image/png" {
		t.Errorf("content type = %q, want image/png", got)
	}
	if _, err := svc.writeMediaFile(context.Background(), id, []byte("x"), ""); err != nil {
		t.Fatal(err)
	}
	if got := store.contentTypes[id.String()]; got != "application/octet-stream" {
		t.Errorf("empty content type stored as %q, want application/octet-stream", got)
	}
}

package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/whatfunnel/whatfunnel/packages/go-common/messaging"
)

type MediaObject struct {
	ID        uuid.UUID `json:"id"`
	Filename  string    `json:"filename,omitempty"`
	MIMEType  string    `json:"mime_type"`
	SizeBytes int64     `json:"size_bytes"`
	ExpiresAt time.Time `json:"expires_at"`
}

type MediaContent struct {
	MediaObject
	Reader io.ReadCloser
}

func (s *Service) ConfigureMediaCache(root string) error {
	root = strings.TrimSpace(root)
	if root == "" {
		return errors.New("media cache path is required")
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return fmt.Errorf("create media cache: %w", err)
	}
	if err := os.Chmod(root, 0o700); err != nil {
		return fmt.Errorf("secure media cache: %w", err)
	}
	s.mediaMu.Lock()
	s.mediaRoot = root
	s.mediaMu.Unlock()
	return nil
}

func (s *Service) RegisterProviderMediaFetcher(provider messaging.Provider, fetcher ProviderMediaFetcher) {
	s.mediaMu.Lock()
	defer s.mediaMu.Unlock()
	if fetcher == nil {
		delete(s.mediaFetchers, provider)
		return
	}
	s.mediaFetchers[provider] = fetcher
}

func (s *Service) SaveOutboundMedia(
	ctx context.Context,
	accountID, conversationID uuid.UUID,
	filename, mimeType string,
	source io.Reader,
) (MediaObject, error) {
	if source == nil {
		return MediaObject{}, errors.New("media file is required")
	}
	var channelID uuid.UUID
	if err := s.pool.QueryRow(ctx, `
		SELECT channel_id FROM conversations WHERE id = $1 AND account_id = $2
	`, conversationID, accountID).Scan(&channelID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return MediaObject{}, errors.New("conversation not found")
		}
		return MediaObject{}, fmt.Errorf("resolve media conversation: %w", err)
	}

	data, err := io.ReadAll(io.LimitReader(source, messaging.MaxMediaBytes+1))
	if err != nil {
		return MediaObject{}, fmt.Errorf("read media upload: %w", err)
	}
	if len(data) == 0 {
		return MediaObject{}, errors.New("media file is empty")
	}
	if int64(len(data)) > messaging.MaxMediaBytes {
		return MediaObject{}, messaging.ErrMediaTooLarge
	}
	if strings.TrimSpace(mimeType) == "" || mimeType == "application/octet-stream" {
		mimeType = http.DetectContentType(data)
	}
	filename = filepath.Base(strings.TrimSpace(filename))
	if filename == "." {
		filename = ""
	}
	retention, _ := messaging.RetentionForSize(int64(len(data)))
	media := MediaObject{
		ID: uuid.New(), Filename: filename, MIMEType: mimeType,
		SizeBytes: int64(len(data)), ExpiresAt: time.Now().UTC().Add(retention),
	}
	storageKey, err := s.writeMediaFile(media.ID, data)
	if err != nil {
		return MediaObject{}, err
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO media_objects (
			id, account_id, channel_id, filename, mime_type, size_bytes,
			storage_key, expires_at
		)
		VALUES ($1, $2, $3, NULLIF($4, ''), $5, $6, $7, $8)
	`, media.ID, accountID, channelID, media.Filename, media.MIMEType,
		media.SizeBytes, storageKey, media.ExpiresAt)
	if err != nil {
		_ = os.Remove(s.mediaPath(storageKey))
		return MediaObject{}, fmt.Errorf("save media upload: %w", err)
	}
	return media, nil
}

func (s *Service) OpenMedia(ctx context.Context, accountID *uuid.UUID, mediaID uuid.UUID) (MediaContent, error) {
	var (
		media       MediaObject
		channelID   uuid.UUID
		provider    messaging.Provider
		providerRef *string
		storageKey  *string
	)
	query := `
		SELECT media.id, COALESCE(media.filename, ''), media.mime_type, media.size_bytes,
		       media.expires_at, media.channel_id, channel.provider,
		       media.provider_ref, media.storage_key
		FROM media_objects AS media
		JOIN channels AS channel ON channel.id = media.channel_id
		WHERE media.id = $1`
	args := []any{mediaID}
	if accountID != nil {
		query += " AND media.account_id = $2"
		args = append(args, *accountID)
	}
	err := s.pool.QueryRow(ctx, query, args...).Scan(
		&media.ID, &media.Filename, &media.MIMEType, &media.SizeBytes,
		&media.ExpiresAt, &channelID, &provider, &providerRef, &storageKey,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return MediaContent{}, errors.New("media not found")
	}
	if err != nil {
		return MediaContent{}, fmt.Errorf("load media: %w", err)
	}

	if storageKey != nil && media.ExpiresAt.After(time.Now()) {
		file, err := os.Open(s.mediaPath(*storageKey))
		if err == nil {
			return MediaContent{MediaObject: media, Reader: file}, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return MediaContent{}, fmt.Errorf("open cached media: %w", err)
		}
	}
	if providerRef == nil || *providerRef == "" {
		return MediaContent{}, errors.New("media expired")
	}

	s.mediaMu.RLock()
	fetcher := s.mediaFetchers[provider]
	s.mediaMu.RUnlock()
	if fetcher == nil {
		return MediaContent{}, errors.New("media provider unavailable")
	}
	downloaded, err := fetcher.Download(ctx, channelID.String(), *providerRef)
	if err != nil {
		return MediaContent{}, fmt.Errorf("download provider media: %w", err)
	}
	if int64(len(downloaded.Data)) > messaging.MaxMediaBytes {
		return MediaContent{}, messaging.ErrMediaTooLarge
	}
	if downloaded.MIMEType != "" {
		media.MIMEType = downloaded.MIMEType
	}
	if downloaded.Filename != "" {
		media.Filename = filepath.Base(downloaded.Filename)
	}
	media.SizeBytes = int64(len(downloaded.Data))
	retention, err := messaging.RetentionForSize(media.SizeBytes)
	if err != nil {
		return MediaContent{}, err
	}
	media.ExpiresAt = time.Now().UTC().Add(retention)
	key, err := s.writeMediaFile(media.ID, downloaded.Data)
	if err != nil {
		return MediaContent{}, err
	}
	_, err = s.pool.Exec(ctx, `
		UPDATE media_objects
		SET storage_key = $1, filename = NULLIF($2, ''), mime_type = $3,
		    size_bytes = $4, expires_at = $5
		WHERE id = $6
	`, key, media.Filename, media.MIMEType, media.SizeBytes, media.ExpiresAt, media.ID)
	if err != nil {
		_ = os.Remove(s.mediaPath(key))
		return MediaContent{}, fmt.Errorf("cache provider media: %w", err)
	}
	return MediaContent{MediaObject: media, Reader: io.NopCloser(bytes.NewReader(downloaded.Data))}, nil
}

func (s *Service) RunMediaCleanup(ctx context.Context) error {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		if err := s.CleanupExpiredMediaOnce(ctx); err != nil && ctx.Err() == nil {
			fmt.Printf("failed to clean expired media: %v\n", err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (s *Service) CleanupExpiredMediaOnce(ctx context.Context) error {
	rows, err := s.pool.Query(ctx, `
		SELECT id, storage_key
		FROM media_objects
		WHERE storage_key IS NOT NULL AND expires_at <= NOW()
		ORDER BY expires_at
		LIMIT 1000
	`)
	if err != nil {
		return fmt.Errorf("list expired media: %w", err)
	}
	defer rows.Close()
	type expiredObject struct {
		id  uuid.UUID
		key string
	}
	expired := make([]expiredObject, 0)
	for rows.Next() {
		var object expiredObject
		if err := rows.Scan(&object.id, &object.key); err != nil {
			return fmt.Errorf("scan expired media: %w", err)
		}
		expired = append(expired, object)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate expired media: %w", err)
	}
	rows.Close()
	for _, object := range expired {
		if err := os.Remove(s.mediaPath(object.key)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove expired media: %w", err)
		}
		if _, err := s.pool.Exec(ctx, `
			UPDATE media_objects SET storage_key = NULL
			WHERE id = $1 AND storage_key = $2 AND expires_at <= NOW()
		`, object.id, object.key); err != nil {
			return fmt.Errorf("clear expired media: %w", err)
		}
	}
	return nil
}

func (s *Service) writeMediaFile(id uuid.UUID, data []byte) (string, error) {
	s.mediaMu.RLock()
	root := s.mediaRoot
	s.mediaMu.RUnlock()
	if root == "" {
		return "", errors.New("media cache is not configured")
	}
	key := id.String()
	temporary, err := os.CreateTemp(root, ".media-*")
	if err != nil {
		return "", fmt.Errorf("create media cache file: %w", err)
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return "", fmt.Errorf("secure media cache file: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return "", fmt.Errorf("write media cache file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return "", fmt.Errorf("close media cache file: %w", err)
	}
	if err := os.Rename(temporaryName, s.mediaPath(key)); err != nil {
		return "", fmt.Errorf("publish media cache file: %w", err)
	}
	return key, nil
}

func (s *Service) mediaPath(key string) string {
	s.mediaMu.RLock()
	root := s.mediaRoot
	s.mediaMu.RUnlock()
	return filepath.Join(root, filepath.Base(key))
}

package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/whatfunnel/whatfunnel/packages/go-common/messaging"
	"github.com/whatfunnel/whatfunnel/packages/go-common/types"
	"github.com/whatfunnel/whatfunnel/services/conversation-svc/internal/mediastore"
)

type ProviderMedia struct {
	Data     []byte
	MIMEType string
	Filename string
}

type ProviderMediaFetcher interface {
	Download(context.Context, string, string) (ProviderMedia, error)
}

type MediaService struct {
	pool          *pgxpool.Pool
	store         mediastore.Store
	mediaRoot     string
	mediaFetchers map[messaging.Provider]ProviderMediaFetcher
	mediaMu       sync.RWMutex
}

func NewMediaService(pool *pgxpool.Pool) *MediaService {
	return &MediaService{
		pool:          pool,
		mediaFetchers: make(map[messaging.Provider]ProviderMediaFetcher),
	}
}

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

// SafeInlineMIMETypes contains MIME types that can safely be rendered inline in browsers
// without risk of script execution (Stored XSS). Active or dangerous content types such as
// image/svg+xml, text/html, text/xml, application/pdf, etc. are excluded.
var SafeInlineMIMETypes = map[string]struct{}{
	"image/jpeg":  {},
	"image/png":   {},
	"image/webp":  {},
	"image/gif":   {},
	"audio/ogg":   {},
	"audio/mpeg":  {},
	"audio/mp4":   {},
	"audio/wav":   {},
	"audio/webm":  {},
	"audio/aac":   {},
	"audio/flac":  {},
	"audio/x-m4a": {},
	"video/mp4":   {},
	"video/webm":  {},
	"video/ogg":   {},
}

// IsSafeInlineMIMEType reports whether rawMIME is safe to serve with Content-Disposition: inline.
func IsSafeInlineMIMEType(rawMIME string) bool {
	mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(rawMIME))
	if err != nil {
		return false
	}
	_, ok := SafeInlineMIMETypes[strings.ToLower(mediaType)]
	return ok
}

// MediaContentDisposition returns the appropriate Content-Disposition header value
// for a given media MIME type and filename. Dangerous or unknown types always receive
// "attachment", while safe raster images and audio/video receive "inline".
func MediaContentDisposition(rawMIME, filename string) string {
	disposition := "attachment"
	if IsSafeInlineMIMEType(rawMIME) {
		disposition = "inline"
	}
	filename = filepath.Base(strings.TrimSpace(filename))
	if filename != "" && filename != "." {
		return mime.FormatMediaType(disposition, map[string]string{"filename": filename})
	}
	return disposition
}

// ConfigureMediaStore sets the backing media store (disk, S3/MinIO, etc.).
func (s *MediaService) ConfigureMediaStore(store mediastore.Store) {
	s.mediaMu.Lock()
	defer s.mediaMu.Unlock()
	s.store = store
}

// ConfigureMediaCache configures a local disk-backed media store.
func (s *MediaService) ConfigureMediaCache(root string) error {
	root = strings.TrimSpace(root)
	if root == "" {
		return errors.New("media cache path is required")
	}
	store, err := mediastore.NewDiskStore(root)
	if err != nil {
		return err
	}
	s.mediaMu.Lock()
	s.store = store
	s.mediaRoot = root
	s.mediaMu.Unlock()
	return nil
}

func (s *MediaService) RegisterProviderMediaFetcher(provider messaging.Provider, fetcher ProviderMediaFetcher) {
	s.mediaMu.Lock()
	defer s.mediaMu.Unlock()
	if fetcher == nil {
		delete(s.mediaFetchers, provider)
		return
	}
	s.mediaFetchers[provider] = fetcher
}

// MediaViewer identifies the authenticated user asking to read media so the
// download can be authorised against the conversation the media belongs to.
type MediaViewer struct {
	AccountID uuid.UUID
	UserID    uuid.UUID
	Role      string
}

// SaveOutboundMedia stores an upload for a conversation the caller can see.
func (s *MediaService) SaveOutboundMedia(
	ctx context.Context,
	viewer MediaViewer,
	conversationID uuid.UUID,
	filename, mimeType string,
	source io.Reader,
) (MediaObject, error) {
	if source == nil {
		return MediaObject{}, invalidf("media file is required")
	}
	accountID := viewer.AccountID
	if err := canSeeConversation(ctx, s.pool, accountID, viewer.UserID, conversationID, viewer.Role); err != nil {
		return MediaObject{}, err
	}
	var channelID uuid.UUID
	if err := s.pool.QueryRow(ctx, `
		SELECT channel_id FROM conversations WHERE id = $1 AND account_id = $2
	`, conversationID, accountID).Scan(&channelID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return MediaObject{}, notFoundf("conversation not found")
		}
		return MediaObject{}, fmt.Errorf("resolve media conversation: %w", err)
	}

	data, err := io.ReadAll(io.LimitReader(source, messaging.MaxMediaBytes+1))
	if err != nil {
		return MediaObject{}, fmt.Errorf("read media upload: %w", err)
	}
	if len(data) == 0 {
		return MediaObject{}, invalidf("media file is empty")
	}
	if int64(len(data)) > messaging.MaxMediaBytes {
		return MediaObject{}, messaging.ErrMediaTooLarge
	}
	if strings.TrimSpace(mimeType) == "" || mimeType == "application/octet-stream" {
		mimeType = http.DetectContentType(data)
	}
	if parsed, _, err := mime.ParseMediaType(mimeType); err == nil {
		mimeType = parsed
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
			id, account_id, channel_id, conversation_id, uploaded_by_user_id,
			filename, mime_type, size_bytes, storage_key, expires_at
		)
		VALUES ($1, $2, $3, $4, $5, NULLIF($6, ''), $7, $8, $9, $10)
	`, media.ID, accountID, channelID, conversationID, viewer.UserID, media.Filename, media.MIMEType,
		media.SizeBytes, storageKey, media.ExpiresAt)
	if err != nil {
		s.mediaMu.RLock()
		store := s.store
		s.mediaMu.RUnlock()
		if store != nil {
			_ = store.Delete(ctx, storageKey)
		}
		return MediaObject{}, fmt.Errorf("save media upload: %w", err)
	}
	return media, nil
}

// OpenMedia returns the media's content. A nil viewer is the trusted internal
// (adapter) caller. Otherwise the media must belong to the viewer's account and
// to a conversation the viewer may see; media not tied to any conversation yet
// is readable only by its uploader or a manager.
func (s *MediaService) OpenMedia(ctx context.Context, viewer *MediaViewer, mediaID uuid.UUID) (MediaContent, error) {
	var (
		media          MediaObject
		accountID      uuid.UUID
		channelID      uuid.UUID
		provider       messaging.Provider
		providerRef    *string
		storageKey     *string
		conversationID *uuid.UUID
		uploadedBy     *uuid.UUID
	)
	err := s.pool.QueryRow(ctx, `
		SELECT media.id, COALESCE(media.filename, ''), media.mime_type, media.size_bytes,
		       media.expires_at, media.account_id, media.channel_id, channel.provider,
		       media.provider_ref, media.storage_key,
		       COALESCE(media.conversation_id, message.conversation_id), media.uploaded_by_user_id
		FROM media_objects AS media
		JOIN channels AS channel ON channel.id = media.channel_id
		LEFT JOIN messages AS message ON message.id = media.message_id
		WHERE media.id = $1`, mediaID).Scan(
		&media.ID, &media.Filename, &media.MIMEType, &media.SizeBytes,
		&media.ExpiresAt, &accountID, &channelID, &provider, &providerRef, &storageKey,
		&conversationID, &uploadedBy,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return MediaContent{}, notFoundf("media not found")
	}
	if err != nil {
		return MediaContent{}, fmt.Errorf("load media: %w", err)
	}
	if viewer != nil {
		if err := s.authorizeMediaViewer(ctx, *viewer, accountID, conversationID, uploadedBy); err != nil {
			return MediaContent{}, err
		}
	}

	s.mediaMu.RLock()
	store := s.store
	s.mediaMu.RUnlock()
	if store == nil {
		return MediaContent{}, errors.New("media store is not configured")
	}

	if storageKey != nil && media.ExpiresAt.After(time.Now()) {
		reader, err := store.Get(ctx, *storageKey)
		if err == nil {
			return MediaContent{MediaObject: media, Reader: reader}, nil
		}
		if !errors.Is(err, mediastore.ErrNotFound) {
			return MediaContent{}, fmt.Errorf("open cached media: %w", err)
		}
	}
	if providerRef == nil || *providerRef == "" {
		return MediaContent{}, notFoundf("media expired")
	}

	s.mediaMu.RLock()
	fetcher := s.mediaFetchers[provider]
	s.mediaMu.RUnlock()
	if fetcher == nil {
		return MediaContent{}, notFoundf("media provider unavailable")
	}
	downloaded, err := fetcher.Download(ctx, channelID.String(), *providerRef)
	if err != nil {
		// The provider no longer serves the file (expired, revoked, or the
		// session is gone). Surface it as unavailable media, but keep the cause.
		slog.WarnContext(ctx, "download provider media failed", "media_id", media.ID, "provider", provider, "error", err)
		return MediaContent{}, notFoundf("media is unavailable from the provider")
	}
	if int64(len(downloaded.Data)) > messaging.MaxMediaBytes {
		return MediaContent{}, messaging.ErrMediaTooLarge
	}
	if downloaded.MIMEType != "" {
		media.MIMEType = downloaded.MIMEType
	}
	if strings.TrimSpace(media.MIMEType) == "" || media.MIMEType == "application/octet-stream" {
		media.MIMEType = http.DetectContentType(downloaded.Data)
	}
	if parsed, _, err := mime.ParseMediaType(media.MIMEType); err == nil {
		media.MIMEType = parsed
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
		_ = store.Delete(ctx, key)
		return MediaContent{}, fmt.Errorf("cache provider media: %w", err)
	}
	return MediaContent{MediaObject: media, Reader: io.NopCloser(bytes.NewReader(downloaded.Data))}, nil
}

func (s *MediaService) authorizeMediaViewer(
	ctx context.Context,
	viewer MediaViewer,
	mediaAccountID uuid.UUID,
	conversationID, uploadedBy *uuid.UUID,
) error {
	if mediaAccountID != viewer.AccountID {
		return notFoundf("media not found")
	}
	if conversationID != nil {
		if err := canSeeConversation(ctx, s.pool, viewer.AccountID, viewer.UserID, *conversationID, viewer.Role); err != nil {
			if errors.Is(err, ErrNotFound) {
				return notFoundf("media not found")
			}
			return err
		}
		return nil
	}
	if viewer.Role == types.RoleManager || (uploadedBy != nil && *uploadedBy == viewer.UserID) {
		return nil
	}
	return notFoundf("media not found")
}

func (s *MediaService) RunMediaCleanup(ctx context.Context) error {
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

func (s *MediaService) CleanupExpiredMediaOnce(ctx context.Context) error {
	s.mediaMu.RLock()
	store := s.store
	s.mediaMu.RUnlock()
	if store == nil {
		return errors.New("media store is not configured")
	}

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
		if err := store.Delete(ctx, object.key); err != nil {
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

func (s *MediaService) writeMediaFile(id uuid.UUID, data []byte) (string, error) {
	s.mediaMu.RLock()
	store := s.store
	s.mediaMu.RUnlock()
	if store == nil {
		return "", errors.New("media cache is not configured")
	}
	key := id.String()
	if err := store.Put(context.Background(), key, bytes.NewReader(data), int64(len(data)), "application/octet-stream"); err != nil {
		return "", fmt.Errorf("write media: %w", err)
	}
	return key, nil
}

func (s *MediaService) mediaPath(key string) string {
	s.mediaMu.RLock()
	root := s.mediaRoot
	s.mediaMu.RUnlock()
	return filepath.Join(root, filepath.Base(key))
}

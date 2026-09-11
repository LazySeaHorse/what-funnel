package session

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/whatfunnel/whatfunnel/packages/go-common/messaging"
	"go.mau.fi/whatsmeow"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

type MediaFile struct {
	Data     []byte
	MIMEType string
	Filename string
}

type MediaSource interface {
	Fetch(context.Context, string) (MediaFile, error)
}

type messageClient interface {
	Upload(context.Context, []byte, whatsmeow.MediaType) (whatsmeow.UploadResponse, error)
	SendMessage(context.Context, types.JID, *waE2E.Message, ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error)
}

func (m *Manager) Send(ctx context.Context, command messaging.Command) error {
	if err := command.Validate(); err != nil {
		return fmt.Errorf("validate whatsapp command: %w", err)
	}
	if command.Provider != messaging.ProviderWhatsApp || command.Kind != messaging.CommandSendMessage {
		return errors.New("whatsapp session: unsupported command")
	}

	session, err := m.get(command.ChannelID)
	if err != nil {
		return err
	}
	if !session.client.IsConnected() || !session.client.IsLoggedIn() {
		return ErrNotConnected
	}
	session.sendMu.Lock()
	defer session.sendMu.Unlock()

	completed, err := m.commandCompleted(ctx, command.ID)
	if err != nil {
		return err
	}
	if completed {
		return nil
	}

	to, err := types.ParseJID(command.Message.ExternalThreadID)
	if err != nil {
		return fmt.Errorf("parse whatsapp destination: %w", err)
	}
	if to.Server != types.DefaultUserServer && to.Server != types.HiddenUserServer {
		return errors.New("whatsapp session: only one-to-one chats are supported")
	}

	message, err := buildMessage(ctx, session.client, m.mediaSource, *command.Message)
	if err != nil {
		return err
	}
	sendCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	providerMessageID := stableMessageID(command.ID)
	response, err := session.client.SendMessage(sendCtx, to, message, whatsmeow.SendRequestExtra{ID: providerMessageID})
	if err != nil {
		return fmt.Errorf("send whatsapp message: %w", err)
	}
	if response.ID == "" {
		response.ID = providerMessageID
	}
	normalized := *command.Message
	normalized.ProviderMessageID = string(response.ID)
	normalized.ProviderTimestamp = response.Timestamp
	event := messaging.Event{
		SchemaVersion: messaging.SchemaVersion,
		ID:            fmt.Sprintf("whatsapp:%s:message.created:%s", command.ChannelID, response.ID),
		Kind:          messaging.EventMessageCreated,
		Provider:      messaging.ProviderWhatsApp,
		ChannelID:     command.ChannelID,
		CorrelationID: command.MessageID,
		OccurredAt:    response.Timestamp,
		Message:       &normalized,
	}
	return m.completeCommandWithEvent(ctx, command.ID, string(response.ID), event)
}

// stableMessageID ensures a command redelivered after either process crashes
// uses the same WhatsApp message ID. WhatsApp's web format is 3EB0 followed by
// 18 uppercase hexadecimal characters.
func stableMessageID(commandID string) types.MessageID {
	hash := sha256.Sum256([]byte(commandID))
	return whatsmeow.WebMessageIDPrefix + strings.ToUpper(hex.EncodeToString(hash[:9]))
}

func (m *Manager) commandCompleted(ctx context.Context, commandID string) (bool, error) {
	var exists int
	err := m.db.QueryRowContext(ctx, `SELECT 1 FROM adapter_commands WHERE command_id = ?`, commandID).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check completed whatsapp command: %w", err)
	}
	return true, nil
}

func (m *Manager) completeCommandWithEvent(
	ctx context.Context,
	commandID, providerMessageID string,
	event messaging.Event,
) error {
	if err := event.Validate(); err != nil {
		return fmt.Errorf("validate completed whatsapp event: %w", err)
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal completed whatsapp event: %w", err)
	}
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin completed whatsapp command: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO adapter_commands (command_id, provider_message_id, completed_at)
		VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(command_id) DO NOTHING
	`, commandID, providerMessageID); err != nil {
		return fmt.Errorf("record completed whatsapp command: %w", err)
	}
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO adapter_event_outbox (event_id, payload, available_at)
		VALUES (?, ?, ?)
		ON CONFLICT(event_id) DO NOTHING
	`, event.ID, payload, time.Now().UnixMilli()); err != nil {
		return fmt.Errorf("record completed whatsapp event: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit completed whatsapp command: %w", err)
	}
	select {
	case m.eventWake <- struct{}{}:
	default:
	}
	return nil
}

func buildMessage(
	ctx context.Context,
	client messageClient,
	mediaSource MediaSource,
	message messaging.Message,
) (*waE2E.Message, error) {
	contextInfo := replyContext(message.ReplyToProviderID)
	switch message.ContentType {
	case messaging.ContentText:
		if contextInfo == nil {
			return &waE2E.Message{Conversation: proto.String(message.Text)}, nil
		}
		return &waE2E.Message{ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text:        proto.String(message.Text),
			ContextInfo: contextInfo,
		}}, nil
	case messaging.ContentImage, messaging.ContentVideo, messaging.ContentAudio, messaging.ContentDocument:
		return buildMediaMessage(ctx, client, mediaSource, message, contextInfo)
	default:
		return nil, fmt.Errorf("whatsapp session: unsupported content type %q", message.ContentType)
	}
}

func buildMediaMessage(
	ctx context.Context,
	client messageClient,
	mediaSource MediaSource,
	message messaging.Message,
	contextInfo *waE2E.ContextInfo,
) (*waE2E.Message, error) {
	if mediaSource == nil || message.Media == nil || strings.TrimSpace(message.Media.ID) == "" {
		return nil, errors.New("whatsapp session: outbound media is unavailable")
	}
	file, err := mediaSource.Fetch(ctx, message.Media.ID)
	if err != nil {
		return nil, fmt.Errorf("fetch outbound media: %w", err)
	}
	if len(file.Data) > int(messaging.MaxMediaBytes) {
		return nil, messaging.ErrMediaTooLarge
	}

	mediaType, err := providerMediaType(message.ContentType)
	if err != nil {
		return nil, err
	}
	uploadCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	upload, err := client.Upload(uploadCtx, file.Data, mediaType)
	if err != nil {
		return nil, fmt.Errorf("upload whatsapp media: %w", err)
	}
	if upload.FileLength > math.MaxInt64 {
		return nil, messaging.ErrMediaTooLarge
	}

	mimeType := file.MIMEType
	if mimeType == "" {
		mimeType = http.DetectContentType(file.Data)
	}
	switch message.ContentType {
	case messaging.ContentImage:
		return &waE2E.Message{ImageMessage: &waE2E.ImageMessage{
			URL:           proto.String(upload.URL),
			DirectPath:    proto.String(upload.DirectPath),
			MediaKey:      upload.MediaKey,
			FileEncSHA256: upload.FileEncSHA256,
			FileSHA256:    upload.FileSHA256,
			FileLength:    proto.Uint64(upload.FileLength),
			Mimetype:      proto.String(mimeType),
			Caption:       proto.String(message.Text),
			ContextInfo:   contextInfo,
		}}, nil
	case messaging.ContentVideo:
		return &waE2E.Message{VideoMessage: &waE2E.VideoMessage{
			URL:           proto.String(upload.URL),
			DirectPath:    proto.String(upload.DirectPath),
			MediaKey:      upload.MediaKey,
			FileEncSHA256: upload.FileEncSHA256,
			FileSHA256:    upload.FileSHA256,
			FileLength:    proto.Uint64(upload.FileLength),
			Mimetype:      proto.String(mimeType),
			Caption:       proto.String(message.Text),
			ContextInfo:   contextInfo,
		}}, nil
	case messaging.ContentAudio:
		return &waE2E.Message{AudioMessage: &waE2E.AudioMessage{
			URL:           proto.String(upload.URL),
			DirectPath:    proto.String(upload.DirectPath),
			MediaKey:      upload.MediaKey,
			FileEncSHA256: upload.FileEncSHA256,
			FileSHA256:    upload.FileSHA256,
			FileLength:    proto.Uint64(upload.FileLength),
			Mimetype:      proto.String(mimeType),
			ContextInfo:   contextInfo,
		}}, nil
	case messaging.ContentDocument:
		filename := file.Filename
		if filename == "" {
			filename = message.Media.Filename
		}
		return &waE2E.Message{DocumentMessage: &waE2E.DocumentMessage{
			URL:           proto.String(upload.URL),
			DirectPath:    proto.String(upload.DirectPath),
			MediaKey:      upload.MediaKey,
			FileEncSHA256: upload.FileEncSHA256,
			FileSHA256:    upload.FileSHA256,
			FileLength:    proto.Uint64(upload.FileLength),
			Mimetype:      proto.String(mimeType),
			FileName:      proto.String(filename),
			Caption:       proto.String(message.Text),
			ContextInfo:   contextInfo,
		}}, nil
	default:
		return nil, fmt.Errorf("whatsapp session: unsupported media type %q", message.ContentType)
	}
}

func replyContext(providerMessageID string) *waE2E.ContextInfo {
	if strings.TrimSpace(providerMessageID) == "" {
		return nil
	}
	return &waE2E.ContextInfo{StanzaID: proto.String(providerMessageID)}
}

func providerMediaType(contentType messaging.ContentType) (whatsmeow.MediaType, error) {
	switch contentType {
	case messaging.ContentImage:
		return whatsmeow.MediaImage, nil
	case messaging.ContentVideo:
		return whatsmeow.MediaVideo, nil
	case messaging.ContentAudio:
		return whatsmeow.MediaAudio, nil
	case messaging.ContentDocument:
		return whatsmeow.MediaDocument, nil
	default:
		return "", fmt.Errorf("whatsapp session: unsupported media type %q", contentType)
	}
}

type HTTPMediaSource struct {
	baseURL string
	secret  string
	client  *http.Client
}

func NewHTTPMediaSource(baseURL, secret string) (*HTTPMediaSource, error) {
	parsed, err := url.Parse(strings.TrimRight(baseURL, "/"))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, errors.New("whatsapp media: invalid base url")
	}
	if strings.TrimSpace(secret) == "" {
		return nil, errors.New("whatsapp media: empty shared secret")
	}
	return &HTTPMediaSource{
		baseURL: parsed.String(),
		secret:  secret,
		client: &http.Client{
			Timeout: 65 * time.Second,
		},
	}, nil
}

func (s *HTTPMediaSource) Fetch(ctx context.Context, mediaID string) (MediaFile, error) {
	requestURL := s.baseURL + "/internal/media/" + url.PathEscape(mediaID)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return MediaFile{}, fmt.Errorf("create media request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+s.secret)
	response, err := s.client.Do(request)
	if err != nil {
		return MediaFile{}, fmt.Errorf("request media: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return MediaFile{}, fmt.Errorf("request media: status %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, messaging.MaxMediaBytes+1))
	if err != nil {
		return MediaFile{}, fmt.Errorf("read media: %w", err)
	}
	if len(data) > int(messaging.MaxMediaBytes) {
		return MediaFile{}, messaging.ErrMediaTooLarge
	}
	filename := ""
	if _, params, err := mime.ParseMediaType(response.Header.Get("Content-Disposition")); err == nil {
		filename = filepath.Base(params["filename"])
	}
	return MediaFile{
		Data:     data,
		MIMEType: response.Header.Get("Content-Type"),
		Filename: filename,
	}, nil
}

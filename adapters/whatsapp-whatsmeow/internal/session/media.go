package session

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/whatfunnel/whatfunnel/packages/go-common/messaging"
	"go.mau.fi/whatsmeow"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
)

func (m *Manager) Download(ctx context.Context, channelID, providerRef string) (MediaFile, error) {
	session, err := m.get(channelID)
	if err != nil {
		return MediaFile{}, err
	}
	if !session.client.IsLoggedIn() {
		return MediaFile{}, ErrNotConnected
	}

	var kind string
	var payload []byte
	err = m.db.QueryRowContext(ctx, `
		SELECT media_kind, media_message
		FROM adapter_media
		WHERE channel_id = ? AND provider_ref = ?
	`, channelID, providerRef).Scan(&kind, &payload)
	if errors.Is(err, sql.ErrNoRows) {
		return MediaFile{}, ErrNotFound
	}
	if err != nil {
		return MediaFile{}, fmt.Errorf("load whatsapp media reference: %w", err)
	}

	downloadable, mimeType, filename, size, err := unmarshalDownloadable(kind, payload)
	if err != nil {
		return MediaFile{}, err
	}
	if size > uint64(messaging.MaxMediaBytes) {
		return MediaFile{}, messaging.ErrMediaTooLarge
	}

	downloadCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	data, err := session.client.Download(downloadCtx, downloadable)
	if err != nil {
		return MediaFile{}, fmt.Errorf("download whatsapp media: %w", err)
	}
	if len(data) > int(messaging.MaxMediaBytes) {
		return MediaFile{}, messaging.ErrMediaTooLarge
	}
	return MediaFile{Data: data, MIMEType: mimeType, Filename: filename}, nil
}

func unmarshalDownloadable(kind string, payload []byte) (whatsmeow.DownloadableMessage, string, string, uint64, error) {
	switch kind {
	case "image":
		message := &waE2E.ImageMessage{}
		if err := proto.Unmarshal(payload, message); err != nil {
			return nil, "", "", 0, fmt.Errorf("unmarshal whatsapp image: %w", err)
		}
		return message, message.GetMimetype(), "", message.GetFileLength(), nil
	case "video":
		message := &waE2E.VideoMessage{}
		if err := proto.Unmarshal(payload, message); err != nil {
			return nil, "", "", 0, fmt.Errorf("unmarshal whatsapp video: %w", err)
		}
		return message, message.GetMimetype(), "", message.GetFileLength(), nil
	case "audio":
		message := &waE2E.AudioMessage{}
		if err := proto.Unmarshal(payload, message); err != nil {
			return nil, "", "", 0, fmt.Errorf("unmarshal whatsapp audio: %w", err)
		}
		return message, message.GetMimetype(), "", message.GetFileLength(), nil
	case "document":
		message := &waE2E.DocumentMessage{}
		if err := proto.Unmarshal(payload, message); err != nil {
			return nil, "", "", 0, fmt.Errorf("unmarshal whatsapp document: %w", err)
		}
		return message, message.GetMimetype(), message.GetFileName(), message.GetFileLength(), nil
	default:
		return nil, "", "", 0, fmt.Errorf("unsupported whatsapp media kind %q", kind)
	}
}

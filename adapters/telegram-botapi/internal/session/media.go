package session

import (
	"context"
	"fmt"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/whatfunnel/whatfunnel/packages/go-common/messaging"
)

func (m *Manager) Download(ctx context.Context, channelID, providerRef string) (MediaFile, error) {
	session, err := m.get(channelID)
	if err != nil {
		return MediaFile{}, err
	}
	if session.copySnapshot().State != messaging.ConnectionConnected {
		return MediaFile{}, ErrNotConnected
	}
	fileCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	file, err := m.api.GetFile(fileCtx, session.token, providerRef)
	if err != nil {
		return MediaFile{}, fmt.Errorf("get telegram file: %w", err)
	}
	if file.FileSize > messaging.MaxMediaBytes {
		return MediaFile{}, messaging.ErrMediaTooLarge
	}
	if strings.TrimSpace(file.FilePath) == "" {
		return MediaFile{}, ErrNotFound
	}
	data, err := m.api.Download(fileCtx, session.token, file.FilePath, messaging.MaxMediaBytes)
	if err != nil {
		return MediaFile{}, fmt.Errorf("download telegram file: %w", err)
	}
	filename := filepath.Base(file.FilePath)
	mimeType := mime.TypeByExtension(filepath.Ext(filename))
	if mimeType == "" && len(data) > 0 {
		mimeType = http.DetectContentType(data)
	}
	return MediaFile{Data: data, MIMEType: mimeType, Filename: filename}, nil
}

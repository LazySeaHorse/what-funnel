package session

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
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

type HTTPMediaSource struct {
	baseURL string
	secret  string
	client  *http.Client
}

func NewHTTPMediaSource(baseURL, secret string) (*HTTPMediaSource, error) {
	parsed, err := url.Parse(strings.TrimRight(baseURL, "/"))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, errors.New("telegram media: invalid base url")
	}
	if strings.TrimSpace(secret) == "" {
		return nil, errors.New("telegram media: empty shared secret")
	}
	return &HTTPMediaSource{baseURL: parsed.String(), secret: secret, client: &http.Client{Timeout: 30 * time.Second}}, nil
}

func (s *HTTPMediaSource) Fetch(ctx context.Context, mediaID string) (MediaFile, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, s.baseURL+"/internal/media/"+url.PathEscape(mediaID), nil)
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
	return MediaFile{Data: data, MIMEType: response.Header.Get("Content-Type"), Filename: filename}, nil
}

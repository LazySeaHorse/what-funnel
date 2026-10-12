package adapterkit

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

// MediaSource fetches outbound media by ID so an adapter can upload it to its provider.
type MediaSource interface {
	Fetch(context.Context, string) (MediaFile, error)
}

// HTTPMediaSource fetches outbound media from the conversation service's internal media endpoint.
type HTTPMediaSource struct {
	baseURL string
	secret  string
	client  *http.Client
}

// NewHTTPMediaSource validates the conversation service URL and shared secret and returns a source
// whose requests give up after timeout.
func NewHTTPMediaSource(baseURL, secret string, timeout time.Duration) (*HTTPMediaSource, error) {
	parsed, err := url.Parse(strings.TrimRight(baseURL, "/"))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, errors.New("adapter media: invalid base url")
	}
	if strings.TrimSpace(secret) == "" {
		return nil, errors.New("adapter media: empty shared secret")
	}
	return &HTTPMediaSource{baseURL: parsed.String(), secret: secret, client: &http.Client{Timeout: timeout}}, nil
}

// Fetch downloads one media item, rejecting anything larger than messaging.MaxMediaBytes.
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
		// Drain a bounded amount so the connection can be reused.
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
	return MediaFile{Data: data, MIMEType: response.Header.Get("Content-Type"), Filename: filename}, nil
}

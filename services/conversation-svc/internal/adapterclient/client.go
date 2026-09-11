package adapterclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/whatfunnel/whatfunnel/services/conversation-svc/internal/service"
)

const maxResponseBytes = 1024 * 1024

type Client struct {
	baseURL string
	secret  string
	client  *http.Client
}

func New(baseURL, secret string) (*Client, error) {
	parsed, err := url.Parse(strings.TrimRight(baseURL, "/"))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, errors.New("adapter client: invalid base url")
	}
	if strings.TrimSpace(secret) == "" {
		return nil, errors.New("adapter client: empty shared secret")
	}
	return &Client{
		baseURL: parsed.String(),
		secret:  secret,
		client: &http.Client{
			Timeout: 20 * time.Second,
		},
	}, nil
}

func (c *Client) Create(ctx context.Context, channelID, credential string) (service.AdapterSnapshot, error) {
	payload := map[string]string{"channel_id": channelID}
	if credential != "" {
		payload["credential"] = credential
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return service.AdapterSnapshot{}, fmt.Errorf("encode create connection: %w", err)
	}
	var snapshot service.AdapterSnapshot
	if err := c.request(ctx, http.MethodPost, "/v1/connections", bytes.NewReader(body), &snapshot); err != nil {
		return service.AdapterSnapshot{}, err
	}
	return snapshot, nil
}

func (c *Client) Retry(ctx context.Context, channelID, credential string) (service.AdapterSnapshot, error) {
	payload := map[string]string{}
	if credential != "" {
		payload["credential"] = credential
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return service.AdapterSnapshot{}, fmt.Errorf("encode retry connection: %w", err)
	}
	var snapshot service.AdapterSnapshot
	path := "/v1/connections/" + url.PathEscape(channelID) + "/retry"
	if err := c.request(ctx, http.MethodPost, path, bytes.NewReader(body), &snapshot); err != nil {
		return service.AdapterSnapshot{}, err
	}
	return snapshot, nil
}

func (c *Client) Snapshot(ctx context.Context, channelID string) (service.AdapterSnapshot, error) {
	var snapshot service.AdapterSnapshot
	path := "/v1/connections/" + url.PathEscape(channelID)
	if err := c.request(ctx, http.MethodGet, path, nil, &snapshot); err != nil {
		return service.AdapterSnapshot{}, err
	}
	return snapshot, nil
}

func (c *Client) Logout(ctx context.Context, channelID string) error {
	return c.request(ctx, http.MethodDelete, "/v1/connections/"+url.PathEscape(channelID), nil, nil)
}

func (c *Client) Download(ctx context.Context, channelID, providerRef string) (service.ProviderMedia, error) {
	path := "/v1/media/" + url.PathEscape(channelID) + "/" + url.PathEscape(providerRef)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return service.ProviderMedia{}, fmt.Errorf("create adapter media request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+c.secret)
	response, err := c.client.Do(request)
	if err != nil {
		return service.ProviderMedia{}, fmt.Errorf("call adapter media: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return service.ProviderMedia{}, fmt.Errorf("call adapter media: status %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 20*1024*1024+1))
	if err != nil {
		return service.ProviderMedia{}, fmt.Errorf("read adapter media: %w", err)
	}
	if len(data) > 20*1024*1024 {
		return service.ProviderMedia{}, errors.New("adapter media exceeds 20 MiB")
	}
	filename := ""
	if _, params, err := mime.ParseMediaType(response.Header.Get("Content-Disposition")); err == nil {
		filename = filepath.Base(params["filename"])
	}
	return service.ProviderMedia{
		Data: data, MIMEType: response.Header.Get("Content-Type"), Filename: filename,
	}, nil
}

func (c *Client) request(
	ctx context.Context,
	method, path string,
	body io.Reader,
	result any,
) error {
	request, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return fmt.Errorf("create adapter request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+c.secret)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := c.client.Do(request)
	if err != nil {
		return fmt.Errorf("call adapter: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return fmt.Errorf("call adapter: status %d", response.StatusCode)
	}
	if result == nil || response.StatusCode == http.StatusNoContent {
		return nil
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, maxResponseBytes))
	if err := decoder.Decode(result); err != nil {
		return fmt.Errorf("decode adapter response: %w", err)
	}
	return nil
}

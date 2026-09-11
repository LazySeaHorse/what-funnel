package botapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const maxResponseBytes = 2 * 1024 * 1024

// Error is a sanitized Telegram API failure. It deliberately excludes the
// request URL because Telegram URLs contain the bot token.
type Error struct {
	Method      string
	StatusCode  int
	ErrorCode   int
	Description string
	RetryAfter  int
}

func (e *Error) Error() string {
	if e.ErrorCode != 0 {
		return fmt.Sprintf("telegram api %s failed with code %d: %s", e.Method, e.ErrorCode, e.Description)
	}
	return fmt.Sprintf("telegram api %s failed with http status %d", e.Method, e.StatusCode)
}

func (e *Error) Permanent() bool {
	return e.ErrorCode >= 400 && e.ErrorCode < 500 && e.ErrorCode != http.StatusTooManyRequests
}

type Client struct {
	baseURL string
	http    *http.Client
}

func New(baseURL string, client *http.Client) (*Client, error) {
	parsed, err := url.Parse(strings.TrimRight(baseURL, "/"))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, errors.New("telegram api: invalid base url")
	}
	if client == nil {
		client = &http.Client{Timeout: 40 * time.Second}
	}
	return &Client{baseURL: parsed.String(), http: client}, nil
}

func (c *Client) GetMe(ctx context.Context, token string) (User, error) {
	var user User
	err := c.call(ctx, token, "getMe", struct{}{}, &user)
	return user, err
}

func (c *Client) DeleteWebhook(ctx context.Context, token string, dropPending bool) error {
	return c.call(ctx, token, "deleteWebhook", map[string]bool{"drop_pending_updates": dropPending}, nil)
}

func (c *Client) GetUpdates(ctx context.Context, token string, offset int64) ([]Update, error) {
	var updates []Update
	err := c.call(ctx, token, "getUpdates", map[string]any{
		"offset": offset, "limit": 100, "timeout": 30,
		"allowed_updates": []string{"message", "edited_message", "message_reaction", "my_chat_member"},
	}, &updates)
	return updates, err
}

func (c *Client) SendJSON(ctx context.Context, token, method string, params any) (Message, error) {
	var message Message
	err := c.call(ctx, token, method, params, &message)
	return message, err
}

func (c *Client) Call(ctx context.Context, token, method string, params any) error {
	return c.call(ctx, token, method, params, nil)
}

func (c *Client) SendMedia(
	ctx context.Context,
	token, method, field, filename, contentType string,
	data []byte,
	fields map[string]string,
) (Message, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for name, value := range fields {
		if err := writer.WriteField(name, value); err != nil {
			return Message{}, fmt.Errorf("telegram api %s: encode field: %w", method, err)
		}
	}
	partHeader := make(textproto.MIMEHeader)
	partHeader.Set("Content-Disposition", fmt.Sprintf(`form-data; name=%q; filename=%q`, field, filename))
	partHeader.Set("Content-Type", contentType)
	part, err := writer.CreatePart(partHeader)
	if err != nil {
		return Message{}, fmt.Errorf("telegram api %s: encode media: %w", method, err)
	}
	if _, err := part.Write(data); err != nil {
		return Message{}, fmt.Errorf("telegram api %s: write media: %w", method, err)
	}
	if err := writer.Close(); err != nil {
		return Message{}, fmt.Errorf("telegram api %s: finish media: %w", method, err)
	}

	request, err := c.request(ctx, token, method, &body)
	if err != nil {
		return Message{}, err
	}
	request.Header.Set("Content-Type", writer.FormDataContentType())
	var message Message
	if err := c.do(request, method, token, &message); err != nil {
		return Message{}, err
	}
	return message, nil
}

func (c *Client) GetFile(ctx context.Context, token, fileID string) (File, error) {
	var file File
	err := c.call(ctx, token, "getFile", map[string]string{"file_id": fileID}, &file)
	return file, err
}

func (c *Client) Download(ctx context.Context, token, filePath string, maxBytes int64) ([]byte, error) {
	requestURL := c.baseURL + "/file/bot" + token + "/" + strings.TrimLeft(filePath, "/")
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, errors.New("telegram api download: create request")
	}
	response, err := c.http.Do(request)
	if err != nil {
		return nil, errors.New("telegram api download: request failed")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return nil, &Error{Method: "download", StatusCode: response.StatusCode}
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxBytes+1))
	if err != nil {
		return nil, errors.New("telegram api download: read response")
	}
	if int64(len(data)) > maxBytes {
		return nil, errors.New("telegram api download: media exceeds limit")
	}
	return data, nil
}

func (c *Client) call(ctx context.Context, token, method string, params any, result any) error {
	body, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("telegram api %s: encode request: %w", method, err)
	}
	request, err := c.request(ctx, token, method, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	return c.do(request, method, token, result)
}

func (c *Client) request(ctx context.Context, token, method string, body io.Reader) (*http.Request, error) {
	if strings.TrimSpace(token) == "" || strings.ContainsAny(token, "/?#") {
		return nil, errors.New("telegram api: invalid bot token")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/bot"+token+"/"+method, body)
	if err != nil {
		return nil, fmt.Errorf("telegram api %s: create request", method)
	}
	return request, nil
}

func (c *Client) do(request *http.Request, method, token string, result any) error {
	response, err := c.http.Do(request)
	if err != nil {
		return fmt.Errorf("telegram api %s: request failed", method)
	}
	defer response.Body.Close()
	var envelope struct {
		OK          bool            `json:"ok"`
		Result      json.RawMessage `json:"result"`
		ErrorCode   int             `json:"error_code"`
		Description string          `json:"description"`
		Parameters  struct {
			RetryAfter int `json:"retry_after"`
		} `json:"parameters"`
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, maxResponseBytes))
	if err := decoder.Decode(&envelope); err != nil {
		return fmt.Errorf("telegram api %s: decode response", method)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 || !envelope.OK {
		description := strings.ReplaceAll(envelope.Description, token, "[REDACTED]")
		description = strings.ReplaceAll(description, url.PathEscape(token), "[REDACTED]")
		return &Error{Method: method, StatusCode: response.StatusCode, ErrorCode: envelope.ErrorCode, Description: description, RetryAfter: envelope.Parameters.RetryAfter}
	}
	if result == nil || len(envelope.Result) == 0 || string(envelope.Result) == "true" {
		return nil
	}
	if err := json.Unmarshal(envelope.Result, result); err != nil {
		return fmt.Errorf("telegram api %s: decode result", method)
	}
	return nil
}

func Int64(value int64) string { return strconv.FormatInt(value, 10) }

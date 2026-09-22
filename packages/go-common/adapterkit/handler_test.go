package adapterkit

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/whatfunnel/whatfunnel/packages/go-common/messaging"
)

type fakeController struct {
	createdChannel    string
	createdCredential string
	createErr         error

	retriedChannel    string
	retriedCredential string
	retryErr          error

	snapshotResult Snapshot
	snapshotErr    error

	logoutChannel string
	logoutErr     error

	downloadFile MediaFile
	downloadErr  error
}

func (f *fakeController) Create(_ context.Context, channelID, credential string) (Snapshot, error) {
	f.createdChannel = channelID
	f.createdCredential = credential
	if f.createErr != nil {
		return Snapshot{}, f.createErr
	}
	return Snapshot{
		ChannelID:       channelID,
		State:           messaging.ConnectionConnected,
		RemoteAccountID: "@bot",
	}, nil
}

func (f *fakeController) Retry(_ context.Context, channelID, credential string) (Snapshot, error) {
	f.retriedChannel = channelID
	f.retriedCredential = credential
	if f.retryErr != nil {
		return Snapshot{}, f.retryErr
	}
	return Snapshot{
		ChannelID: channelID,
		State:     messaging.ConnectionAwaitingScan,
		QRData:    "qr-123",
	}, nil
}

func (f *fakeController) Snapshot(channelID string) (Snapshot, error) {
	if f.snapshotErr != nil {
		return Snapshot{}, f.snapshotErr
	}
	res := f.snapshotResult
	res.ChannelID = channelID
	return res, nil
}

func (f *fakeController) Logout(_ context.Context, channelID string) error {
	f.logoutChannel = channelID
	return f.logoutErr
}

func (f *fakeController) Download(_ context.Context, _, _ string) (MediaFile, error) {
	if f.downloadErr != nil {
		return MediaFile{}, f.downloadErr
	}
	return f.downloadFile, nil
}

func TestHandler_Authentication(t *testing.T) {
	handler, err := NewHandler(&fakeController{}, HandlerConfig{
		ProviderName: "Telegram",
		SharedSecret: "top-secret",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 1. Missing auth header
	req := httptest.NewRequest(http.MethodGet, "/v1/connections/ch-1", nil)
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}

	// 2. Wrong auth token
	req = httptest.NewRequest(http.MethodGet, "/v1/connections/ch-1", nil)
	req.Header.Set("Authorization", "Bearer wrong-secret")
	rec = httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}

	// 3. Valid auth token
	req = httptest.NewRequest(http.MethodGet, "/v1/connections/ch-1", nil)
	req.Header.Set("Authorization", "Bearer top-secret")
	rec = httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestHandler_CreateSuccess(t *testing.T) {
	ctrl := &fakeController{}
	handler, err := NewHandler(ctrl, HandlerConfig{
		ProviderName: "Telegram",
		SharedSecret: "secret",
	})
	if err != nil {
		t.Fatal(err)
	}

	body := `{"channel_id":"ch-100","credential":"bot-token-123"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/connections", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusAccepted, rec.Body.String())
	}
	if ctrl.createdChannel != "ch-100" || ctrl.createdCredential != "bot-token-123" {
		t.Errorf("unexpected controller input: ch=%s, cred=%s", ctrl.createdChannel, ctrl.createdCredential)
	}
	// Verify credential not leaked in response
	if strings.Contains(rec.Body.String(), "bot-token-123") {
		t.Errorf("response exposed bot token: %s", rec.Body.String())
	}
}

func TestHandler_CreateConflict(t *testing.T) {
	ctrl := &fakeController{createErr: ErrAlreadyExists}
	handler, _ := NewHandler(ctrl, HandlerConfig{ProviderName: "WhatsApp", SharedSecret: "secret"})

	req := httptest.NewRequest(http.MethodPost, "/v1/connections", strings.NewReader(`{"channel_id":"ch-1"}`))
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusConflict)
	}
}

func TestHandler_CreateErrorRedaction(t *testing.T) {
	ctrl := &fakeController{createErr: errors.New("upstream failed: secret-token-leak")}
	handler, _ := NewHandler(ctrl, HandlerConfig{ProviderName: "Telegram", SharedSecret: "secret"})

	req := httptest.NewRequest(http.MethodPost, "/v1/connections", strings.NewReader(`{"channel_id":"ch-1","credential":"secret-token-leak"}`))
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadGateway)
	}
	if strings.Contains(rec.Body.String(), "secret-token-leak") {
		t.Errorf("error leaked sensitive token: %s", rec.Body.String())
	}
}

func TestHandler_Retry(t *testing.T) {
	ctrl := &fakeController{}
	handler, _ := NewHandler(ctrl, HandlerConfig{ProviderName: "WhatsApp", SharedSecret: "secret"})

	req := httptest.NewRequest(http.MethodPost, "/v1/connections/ch-1/retry", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusAccepted, rec.Body.String())
	}
	if ctrl.retriedChannel != "ch-1" {
		t.Errorf("retriedChannel = %q, want ch-1", ctrl.retriedChannel)
	}
	if !strings.Contains(rec.Body.String(), "qr-123") {
		t.Errorf("expected qr-123 in response: %s", rec.Body.String())
	}
}

func TestHandler_GetNotFound(t *testing.T) {
	ctrl := &fakeController{snapshotErr: ErrNotFound}
	handler, _ := NewHandler(ctrl, HandlerConfig{ProviderName: "Telegram", SharedSecret: "secret"})

	req := httptest.NewRequest(http.MethodGet, "/v1/connections/missing", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandler_Delete(t *testing.T) {
	ctrl := &fakeController{}
	handler, _ := NewHandler(ctrl, HandlerConfig{ProviderName: "Telegram", SharedSecret: "secret"})

	req := httptest.NewRequest(http.MethodDelete, "/v1/connections/ch-1", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if ctrl.logoutChannel != "ch-1" {
		t.Errorf("logoutChannel = %q, want ch-1", ctrl.logoutChannel)
	}
}

func TestHandler_Download(t *testing.T) {
	ctrl := &fakeController{
		downloadFile: MediaFile{
			Filename: "photo.jpg",
			MIMEType: "image/jpeg",
			Data:     []byte("binary-jpeg-data"),
		},
	}
	handler, _ := NewHandler(ctrl, HandlerConfig{ProviderName: "Telegram", SharedSecret: "secret"})

	req := httptest.NewRequest(http.MethodGet, "/v1/media/ch-1/ref-1", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if rec.Header().Get("Content-Type") != "image/jpeg" {
		t.Errorf("Content-Type = %q, want image/jpeg", rec.Header().Get("Content-Type"))
	}
	if !strings.Contains(rec.Header().Get("Content-Disposition"), "photo.jpg") {
		t.Errorf("Content-Disposition = %q, want photo.jpg", rec.Header().Get("Content-Disposition"))
	}
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q, want nosniff", rec.Header().Get("X-Content-Type-Options"))
	}
	if rec.Body.String() != "binary-jpeg-data" {
		t.Errorf("body = %q, want binary-jpeg-data", rec.Body.String())
	}
}

func TestHandler_DisallowUnknownFields(t *testing.T) {
	ctrl := &fakeController{}
	handler, _ := NewHandler(ctrl, HandlerConfig{ProviderName: "Telegram", SharedSecret: "secret"})

	req := httptest.NewRequest(http.MethodPost, "/v1/connections", strings.NewReader(`{"channel_id":"ch-1","unknown_param":"foo"}`))
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d (bad request for unknown fields)", rec.Code, http.StatusBadRequest)
	}
}

func TestNewHandler_Validation(t *testing.T) {
	if _, err := NewHandler(nil, HandlerConfig{SharedSecret: "secret"}); err == nil {
		t.Fatal("expected error for nil controller")
	}
	if _, err := NewHandler(&fakeController{}, HandlerConfig{SharedSecret: ""}); err == nil {
		t.Fatal("expected error for empty secret")
	}
}

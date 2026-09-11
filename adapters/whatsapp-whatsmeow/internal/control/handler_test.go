package control

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/whatfunnel/whatfunnel/adapters/whatsapp-whatsmeow/internal/session"
	"github.com/whatfunnel/whatfunnel/packages/go-common/messaging"
)

type fakeController struct {
	snapshot session.Snapshot
	err      error
}

func (f *fakeController) Create(_ context.Context, channelID string) (session.Snapshot, error) {
	f.snapshot.ChannelID = channelID
	return f.snapshot, f.err
}

func (f *fakeController) Snapshot(_ string) (session.Snapshot, error) {
	return f.snapshot, f.err
}

func (f *fakeController) Logout(_ context.Context, _ string) error {
	return f.err
}

func (f *fakeController) Download(_ context.Context, _, _ string) (session.MediaFile, error) {
	return session.MediaFile{Data: []byte("media"), MIMEType: "image/png"}, f.err
}

func TestHandlerAuthentication(t *testing.T) {
	t.Parallel()

	handler, err := NewHandler(&fakeController{}, "secret-value")
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}
	request := httptest.NewRequest(http.MethodGet, "/v1/connections/channel-1", nil)
	recorder := httptest.NewRecorder()

	handler.Routes().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
}

func TestHandlerCreate(t *testing.T) {
	t.Parallel()

	controller := &fakeController{snapshot: session.Snapshot{State: messaging.ConnectionAwaitingScan, QRData: "qr-data"}}
	handler, err := NewHandler(controller, "secret-value")
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/connections", strings.NewReader(`{"channel_id":"channel-1"}`))
	request.Header.Set("Authorization", "Bearer secret-value")
	recorder := httptest.NewRecorder()

	handler.Routes().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusAccepted, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"channel_id":"channel-1"`) {
		t.Errorf("body = %s, want channel id", recorder.Body.String())
	}
}

func TestHandlerGetNotFound(t *testing.T) {
	t.Parallel()

	handler, err := NewHandler(&fakeController{err: session.ErrNotFound}, "secret-value")
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}
	request := httptest.NewRequest(http.MethodGet, "/v1/connections/missing", nil)
	request.Header.Set("Authorization", "Bearer secret-value")
	recorder := httptest.NewRecorder()

	handler.Routes().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
}

func TestHandlerDownload(t *testing.T) {
	t.Parallel()

	handler, err := NewHandler(&fakeController{}, "secret-value")
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}
	request := httptest.NewRequest(http.MethodGet, "/v1/media/channel-1/provider-1", nil)
	request.Header.Set("Authorization", "Bearer secret-value")
	recorder := httptest.NewRecorder()

	handler.Routes().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if recorder.Header().Get("Content-Type") != "image/png" {
		t.Errorf("content type = %q, want image/png", recorder.Header().Get("Content-Type"))
	}
	if recorder.Body.String() != "media" {
		t.Errorf("body = %q, want media", recorder.Body.String())
	}
}

func TestNewHandler(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		controller Controller
		secret     string
	}{
		{name: "nil controller", secret: "secret"},
		{name: "empty secret", controller: &fakeController{}},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := NewHandler(test.controller, test.secret); err == nil {
				t.Fatal("NewHandler() error = nil, want error")
			}
		})
	}
}

func TestHandlerDeleteFailure(t *testing.T) {
	t.Parallel()

	handler, err := NewHandler(&fakeController{err: errors.New("upstream unavailable")}, "secret-value")
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}
	request := httptest.NewRequest(http.MethodDelete, "/v1/connections/channel-1", nil)
	request.Header.Set("Authorization", "Bearer secret-value")
	recorder := httptest.NewRecorder()

	handler.Routes().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadGateway)
	}
}

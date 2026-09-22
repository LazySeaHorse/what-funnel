package proxy_test

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/whatfunnel/whatfunnel/services/api-gateway/internal/proxy"
)

func TestHTTPProxy(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Custom-Echo", "echoed")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "upstream body")
	}))
	defer upstream.Close()

	u, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatalf("parse upstream url: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := proxy.HTTP(u, logger)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/hello", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if rec.Header().Get("X-Custom-Echo") != "echoed" {
		t.Fatalf("expected X-Custom-Echo header, got %q", rec.Header().Get("X-Custom-Echo"))
	}
	if rec.Body.String() != "upstream body" {
		t.Fatalf("expected 'upstream body', got %q", rec.Body.String())
	}
}

func TestHTTPProxy_StripsInternalHeaders(t *testing.T) {
	var capturedHeader http.Header
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHeader = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	u, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatalf("parse upstream url: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := proxy.HTTP(u, logger)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-Internal-Token", "secret-token")
	req.Header.Set("X-Account-ID", "00000000-0000-0000-0000-000000000001")
	req.Header.Set("X-User-ID", "00000000-0000-0000-0000-000000000002")
	req.Header.Set("X-User-Role", "admin")
	req.Header.Set("X-Internal-Custom", "injected")
	req.Header.Set("X-Legit-Header", "allowed")
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if capturedHeader.Get("X-Internal-Token") != "" {
		t.Errorf("expected X-Internal-Token to be stripped, got %q", capturedHeader.Get("X-Internal-Token"))
	}
	if capturedHeader.Get("X-Account-ID") != "" {
		t.Errorf("expected X-Account-ID to be stripped, got %q", capturedHeader.Get("X-Account-ID"))
	}
	if capturedHeader.Get("X-User-ID") != "" {
		t.Errorf("expected X-User-ID to be stripped, got %q", capturedHeader.Get("X-User-ID"))
	}
	if capturedHeader.Get("X-User-Role") != "" {
		t.Errorf("expected X-User-Role to be stripped, got %q", capturedHeader.Get("X-User-Role"))
	}
	if capturedHeader.Get("X-Internal-Custom") != "" {
		t.Errorf("expected X-Internal-Custom to be stripped, got %q", capturedHeader.Get("X-Internal-Custom"))
	}
	if capturedHeader.Get("X-Legit-Header") != "allowed" {
		t.Errorf("expected X-Legit-Header to be preserved, got %q", capturedHeader.Get("X-Legit-Header"))
	}
}

func TestWebSocketProxy_NonUpgrade(t *testing.T) {
	u, _ := url.Parse("http://127.0.0.1:8084")
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := proxy.WebSocket(u, logger)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for non-upgrade request, got %d", rec.Code)
	}
}

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

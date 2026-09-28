package server

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestServer_CheckOrigin(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	t.Run("Production mode blocks unauthorized cross-origin", func(t *testing.T) {
		s := NewServer(nil, nil, logger, []string{"https://app.whatfunnel.com"}, true)

		// Block malicious cross-origin
		req := httptest.NewRequest(http.MethodGet, "https://api.whatfunnel.com/ws", nil)
		req.Host = "api.whatfunnel.com"
		req.Header.Set("Origin", "http://evil-attacker.com")
		assert.False(t, s.CheckOrigin(req))

		// Allow whitelisted origin
		req = httptest.NewRequest(http.MethodGet, "https://api.whatfunnel.com/ws", nil)
		req.Host = "api.whatfunnel.com"
		req.Header.Set("Origin", "https://app.whatfunnel.com")
		assert.True(t, s.CheckOrigin(req))

		// Allow same-host origin
		req = httptest.NewRequest(http.MethodGet, "https://api.whatfunnel.com/ws", nil)
		req.Host = "api.whatfunnel.com"
		req.Header.Set("Origin", "https://api.whatfunnel.com")
		assert.True(t, s.CheckOrigin(req))

		// Allow client without Origin header (direct/native app)
		req = httptest.NewRequest(http.MethodGet, "https://api.whatfunnel.com/ws", nil)
		assert.True(t, s.CheckOrigin(req))
	})

	t.Run("Non-production mode allows local origins", func(t *testing.T) {
		s := NewServer(nil, nil, logger, nil, false)

		// Allow localhost:5173
		req := httptest.NewRequest(http.MethodGet, "http://localhost:8080/ws", nil)
		req.Host = "localhost:8080"
		req.Header.Set("Origin", "http://localhost:5173")
		assert.True(t, s.CheckOrigin(req))

		// Allow 127.0.0.1:3000
		req = httptest.NewRequest(http.MethodGet, "http://localhost:8080/ws", nil)
		req.Host = "localhost:8080"
		req.Header.Set("Origin", "http://127.0.0.1:3000")
		assert.True(t, s.CheckOrigin(req))

		// Block unknown non-local domain in non-prod when not whitelisted
		req = httptest.NewRequest(http.MethodGet, "http://localhost:8080/ws", nil)
		req.Host = "localhost:8080"
		req.Header.Set("Origin", "http://evil-attacker.com")
		assert.False(t, s.CheckOrigin(req))
	})
}

func TestCheckOrigin_SpoofedForwardedHost(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	t.Run("Rejects spoofed X-Forwarded-Host in production", func(t *testing.T) {
		s := NewServer(nil, nil, logger, []string{"https://app.whatfunnel.com"}, true)

		req := httptest.NewRequest(http.MethodGet, "https://api.whatfunnel.com/ws", nil)
		req.Host = "api.whatfunnel.com"
		req.Header.Set("X-Forwarded-Host", "evil-attacker.com")
		req.Header.Set("Origin", "http://evil-attacker.com")

		assert.False(t, s.CheckOrigin(req), "spoofed X-Forwarded-Host must be rejected")
	})

	t.Run("Rejects spoofed X-Forwarded-Host even in non-production", func(t *testing.T) {
		s := NewServer(nil, nil, logger, nil, false)

		req := httptest.NewRequest(http.MethodGet, "http://localhost:8080/ws", nil)
		req.Host = "localhost:8080"
		req.Header.Set("X-Forwarded-Host", "evil-attacker.com")
		req.Header.Set("Origin", "http://evil-attacker.com")

		assert.False(t, s.CheckOrigin(req), "spoofed X-Forwarded-Host in non-prod must be rejected")
	})

	t.Run("Strict port handling blocks mismatched ports", func(t *testing.T) {
		s := NewServer(nil, nil, logger, []string{"https://app.whatfunnel.com"}, true)

		req := httptest.NewRequest(http.MethodGet, "https://api.whatfunnel.com:8080/ws", nil)
		req.Host = "api.whatfunnel.com:8080"
		req.Header.Set("Origin", "https://api.whatfunnel.com:9999")

		assert.False(t, s.CheckOrigin(req), "mismatched port must be rejected")
	})

	t.Run("Strict subdomain handling blocks domain suffix tricks", func(t *testing.T) {
		s := NewServer(nil, nil, logger, []string{"https://app.whatfunnel.com"}, true)

		req := httptest.NewRequest(http.MethodGet, "https://api.whatfunnel.com/ws", nil)
		req.Host = "api.whatfunnel.com"
		req.Header.Set("Origin", "https://api.whatfunnel.com.evil.com")

		assert.False(t, s.CheckOrigin(req), "subdomain spoofing must be rejected")
	})

	t.Run("Allows trusted proxy X-Forwarded-Host matching allowed origin", func(t *testing.T) {
		s := NewServer(nil, nil, logger, []string{"https://app.whatfunnel.com"}, true)

		req := httptest.NewRequest(http.MethodGet, "http://notification-svc:8084/ws", nil)
		req.Host = "notification-svc:8084"
		req.Header.Set("X-Forwarded-Host", "app.whatfunnel.com")
		req.Header.Set("Origin", "https://app.whatfunnel.com")

		assert.True(t, s.CheckOrigin(req), "trusted forwarded host in allowed origins must be accepted")
	})
}

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

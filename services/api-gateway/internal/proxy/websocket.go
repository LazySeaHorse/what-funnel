package proxy

import (
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// WebSocket proxies WebSocket upgrade requests to the upstream server.
func WebSocket(upstreamURL *url.URL, logger *slog.Logger) http.Handler {
	reverse := newReverseProxy(upstreamURL, logger, nil)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !headerHasToken(r.Header, "Connection", "upgrade") ||
			!strings.EqualFold(strings.TrimSpace(r.Header.Get("Upgrade")), "websocket") {
			http.Error(w, "websocket proxy: not an upgrade request", http.StatusBadRequest)
			return
		}
		// The server's read/write timeouts must not tear down a long-lived tunnel.
		rc := http.NewResponseController(w)
		_ = rc.SetReadDeadline(time.Time{})
		_ = rc.SetWriteDeadline(time.Time{})
		reverse.ServeHTTP(w, r)
	})
}

// headerHasToken reports whether any value of the comma-separated header contains token (case-insensitive).
func headerHasToken(header http.Header, name, token string) bool {
	for _, value := range header.Values(name) {
		for _, part := range strings.Split(value, ",") {
			if strings.EqualFold(strings.TrimSpace(part), token) {
				return true
			}
		}
	}
	return false
}

package proxy

import (
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// IsInternalHeader reports whether the given header key is an internal service-to-service header
// that must never be forwarded from untrusted client requests.
func IsInternalHeader(key string) bool {
	lower := strings.ToLower(key)
	return lower == "x-internal-token" ||
		lower == "x-account-id" ||
		lower == "x-user-id" ||
		lower == "x-user-role" ||
		strings.HasPrefix(lower, "x-internal-")
}

// HTTP returns an http.Handler that forwards requests to the given upstream base URL.
// Headers (including Cookie for session) are forwarded; Host is rewritten to the upstream.
// Untrusted client-supplied internal headers are stripped to prevent privilege escalation.
func HTTP(upstream *url.URL, logger *slog.Logger) http.Handler {
	client := &http.Client{
		Timeout: 25 * time.Second,
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		target := *upstream
		target.Path = strings.TrimRight(target.Path, "/") + r.URL.Path
		target.RawQuery = r.URL.RawQuery

		req, err := http.NewRequestWithContext(r.Context(), r.Method, target.String(), r.Body)
		if err != nil {
			logger.Error("proxy: create request", "error", err)
			http.Error(w, "gateway error", http.StatusBadGateway)
			return
		}

		// Forward headers (excluding internal identity/token headers)
		for key, vals := range r.Header {
			if IsInternalHeader(key) {
				continue
			}
			for _, v := range vals {
				req.Header.Add(key, v)
			}
		}
		req.Header.Set("X-Forwarded-For", r.RemoteAddr)
		req.Header.Set("X-Real-IP", r.RemoteAddr)

		resp, err := client.Do(req)
		if err != nil {
			logger.Error("proxy: upstream error", "target", target.String(), "error", err)
			http.Error(w, "gateway error: upstream unavailable", http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()

		// Forward response headers (including Set-Cookie for sessions)
		for key, vals := range resp.Header {
			for _, v := range vals {
				w.Header().Add(key, v)
			}
		}
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body) //nolint:errcheck
	})
}

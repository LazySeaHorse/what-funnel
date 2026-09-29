package proxy

import (
	"log/slog"
	"net/http"
	"net/url"
	"strings"
)

// IsInternalHeader reports whether the given header key is an internal service-to-service header
// that must never be forwarded from untrusted client requests.
func IsInternalHeader(key string) bool {
	lower := strings.ToLower(key)
	return lower == "x-internal-token" ||
		lower == "x-account-id" ||
		lower == "x-user-id" ||
		lower == "x-user-role" ||
		lower == "x-forwarded-host" ||
		strings.HasPrefix(lower, "x-internal-")
}

// HTTP returns an http.Handler that forwards requests to the given upstream base URL.
// Headers (including Cookie for session) are forwarded; Host is rewritten to the upstream.
// Untrusted client-supplied internal headers are stripped to prevent privilege escalation.
// Response bodies are streamed without an overall deadline, and upstream redirects are
// passed through to the client rather than followed.
func HTTP(upstream *url.URL, logger *slog.Logger) http.Handler {
	return newReverseProxy(upstream, logger, nil)
}

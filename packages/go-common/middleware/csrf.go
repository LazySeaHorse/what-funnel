package middleware

import (
	"net/http"
	"net/url"
	"os"
	"strings"
)

const (
	defaultSessionCookie = "whatfunnel_session"
	defaultCSRFCookie    = "csrf_token"
	headerCSRFToken      = "X-CSRF-Token"
	headerXSRFToken      = "X-XSRF-Token"
	headerRequestedWith  = "X-Requested-With"
)

// CSRFProtection returns a middleware that defends against Cross-Site Request Forgery (CSRF).
// It verifies double-submit CSRF tokens and request origins for cookie-authenticated mutating requests.
func CSRFProtection() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 1. Safe HTTP methods do not alter state
			switch r.Method {
			case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
				next.ServeHTTP(w, r)
				return
			}

			// 2. Allow internal service-to-service calls using secret
			secret := os.Getenv("SESSION_SECRET")
			if secret == "" {
				secret = "change-me-in-production-at-least-32-chars"
			}
			if internalToken := r.Header.Get("X-Internal-Token"); internalToken != "" && internalToken == secret {
				next.ServeHTTP(w, r)
				return
			}

			// 3. Exclude unauthenticated public endpoints and webhooks
			p := r.URL.Path
			if p == "/healthz" ||
				p == "/auth/login" || p == "/api-gateway/auth/login" ||
				p == "/auth/signup" || p == "/api-gateway/auth/signup" ||
				strings.HasPrefix(p, "/webhooks") || strings.HasPrefix(p, "/api-gateway/webhooks") ||
				p == "/simulate-inbound" || p == "/api-gateway/simulate-inbound" {
				next.ServeHTTP(w, r)
				return
			}

			// 4. If request is not cookie-authenticated, CSRF via ambient credentials does not apply
			sessionCookie, err := r.Cookie(defaultSessionCookie)
			if err != nil || sessionCookie.Value == "" {
				next.ServeHTTP(w, r)
				return
			}

			// 5. Origin / Referer validation if present
			if origin := r.Header.Get("Origin"); origin != "" {
				if parsedOrigin, err := url.Parse(origin); err == nil && parsedOrigin.Host != "" {
					reqHost := r.Host
					if fwdHost := r.Header.Get("X-Forwarded-Host"); fwdHost != "" {
						reqHost = fwdHost
					}
					// Strip port for loose local/dev matching if needed, or compare host
					originHost := parsedOrigin.Host
					if !strings.EqualFold(originHost, reqHost) &&
						!strings.EqualFold(strings.Split(originHost, ":")[0], strings.Split(reqHost, ":")[0]) {
						writeJSON(w, http.StatusForbidden, map[string]string{
							"error": "forbidden: cross-origin request rejected",
						})
						return
					}
				}
			}

			// 6. Double Submit CSRF token validation
			csrfCookie, err := r.Cookie(defaultCSRFCookie)
			tokenHeader := r.Header.Get(headerCSRFToken)
			if tokenHeader == "" {
				tokenHeader = r.Header.Get(headerXSRFToken)
			}

			if err == nil && csrfCookie.Value != "" {
				if tokenHeader == "" || tokenHeader != csrfCookie.Value {
					writeJSON(w, http.StatusForbidden, map[string]string{
						"error": "forbidden: invalid or missing CSRF token",
					})
					return
				}
			} else {
				// If no CSRF cookie exists, require at least custom anti-CSRF or AJAX header
				if tokenHeader == "" && r.Header.Get(headerRequestedWith) == "" {
					writeJSON(w, http.StatusForbidden, map[string]string{
						"error": "forbidden: missing CSRF token or custom header",
					})
					return
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

package middleware

import (
	"net"
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
func CSRFProtection(allowedOrigins ...string) func(http.Handler) http.Handler {
	var explicitOrigins []string
	if len(allowedOrigins) > 0 {
		explicitOrigins = append(explicitOrigins, allowedOrigins...)
	} else if envOrigins := os.Getenv("ALLOWED_ORIGINS"); envOrigins != "" {
		for _, o := range strings.Split(envOrigins, ",") {
			if trimmed := strings.TrimSpace(o); trimmed != "" {
				explicitOrigins = append(explicitOrigins, trimmed)
			}
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 1. Safe HTTP methods do not alter state
			switch r.Method {
			case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
				next.ServeHTTP(w, r)
				return
			}

			// 2. Allow internal service-to-service calls using secret
			if internalToken := r.Header.Get("X-Internal-Token"); IsAuthorizedInternalCall(internalToken) {
				next.ServeHTTP(w, r)
				return
			}

			// 3. Exclude unauthenticated public endpoints and webhooks
			p := r.URL.Path
			if p == "/healthz" ||
				p == "/auth/login" || p == "/api-gateway/auth/login" ||
				p == "/auth/signup" || p == "/api-gateway/auth/signup" ||
				strings.HasPrefix(p, "/webhooks") || strings.HasPrefix(p, "/api-gateway/webhooks") ||
				((p == "/simulate-inbound" || p == "/api-gateway/simulate-inbound") && os.Getenv("APP_ENV") != "production") {
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
			originHeader := r.Header.Get("Origin")
			if originHeader == "" {
				originHeader = r.Header.Get("Referer")
			}
			if originHeader != "" {
				parsedOrigin, err := url.Parse(originHeader)
				if err != nil || parsedOrigin.Host == "" {
					writeJSON(w, http.StatusForbidden, map[string]string{
						"error": "forbidden: cross-origin request rejected",
					})
					return
				}

				if !validateRequestOrigin(parsedOrigin, r, explicitOrigins) {
					writeJSON(w, http.StatusForbidden, map[string]string{
						"error": "forbidden: cross-origin request rejected",
					})
					return
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

func splitHostPortStrict(rawHost string) (host string, port string) {
	rawHost = strings.TrimSpace(rawHost)
	if rawHost == "" {
		return "", ""
	}
	h, p, err := net.SplitHostPort(rawHost)
	if err != nil {
		return strings.ToLower(rawHost), ""
	}
	return strings.ToLower(h), p
}

func normalizeHost(rawHost string, scheme string) string {
	h, p := splitHostPortStrict(rawHost)
	if h == "" {
		return ""
	}
	scheme = strings.ToLower(scheme)
	if (p == "443" && (scheme == "https" || scheme == "wss")) ||
		(p == "80" && (scheme == "http" || scheme == "ws")) {
		p = ""
	}
	if p != "" {
		return net.JoinHostPort(h, p)
	}
	return h
}

func isHostMatching(originHost, originScheme, candidateHost string) bool {
	normOrigin := normalizeHost(originHost, originScheme)
	normCandidate := normalizeHost(candidateHost, originScheme)
	return normOrigin != "" && normOrigin == normCandidate
}

func validateRequestOrigin(parsedOrigin *url.URL, r *http.Request, explicitOrigins []string) bool {
	// 1. Check explicit whitelist
	for _, allowed := range explicitOrigins {
		allowed = strings.TrimSpace(allowed)
		if allowed == "" {
			continue
		}
		if allowed == "*" {
			return true
		}
		if strings.EqualFold(allowed, parsedOrigin.String()) {
			return true
		}
		if parsedAllowed, err := url.Parse(allowed); err == nil && parsedAllowed.Host != "" {
			if parsedAllowed.Scheme != "" && !strings.EqualFold(parsedAllowed.Scheme, parsedOrigin.Scheme) {
				continue
			}
			if isHostMatching(parsedOrigin.Host, parsedOrigin.Scheme, parsedAllowed.Host) {
				return true
			}
		} else if isHostMatching(parsedOrigin.Host, parsedOrigin.Scheme, allowed) {
			return true
		}
	}

	// 2. In non-production, allow local dev origins
	appEnv := strings.ToLower(os.Getenv("APP_ENV"))
	env := strings.ToLower(os.Getenv("ENV"))
	isProd := appEnv == "production" || appEnv == "prod" || env == "production" || env == "prod"
	if !isProd {
		hostOnly := strings.Split(parsedOrigin.Host, ":")[0]
		if hostOnly == "localhost" || hostOnly == "127.0.0.1" || hostOnly == "0.0.0.0" || strings.HasSuffix(hostOnly, ".local") {
			return true
		}
	}

	// 3. Same-host check against request's Host header
	// Never blindly trust client-supplied X-Forwarded-Host: client-controlled X-Forwarded-Host
	// must NOT be used to authenticate an untrusted origin.
	// Only trust X-Forwarded-Host if it matches r.Host or explicit allowed origins.
	trustedHost := r.Host
	if fwdHost := r.Header.Get("X-Forwarded-Host"); fwdHost != "" {
		if isHostMatching(fwdHost, "", r.Host) {
			trustedHost = fwdHost
		} else {
			for _, allowed := range explicitOrigins {
				if parsedAllowed, err := url.Parse(allowed); err == nil && parsedAllowed.Host != "" {
					if isHostMatching(fwdHost, "", parsedAllowed.Host) {
						trustedHost = fwdHost
						break
					}
				} else if isHostMatching(fwdHost, "", allowed) {
					trustedHost = fwdHost
					break
				}
			}
		}
	}

	return trustedHost != "" && isHostMatching(parsedOrigin.Host, parsedOrigin.Scheme, trustedHost)
}

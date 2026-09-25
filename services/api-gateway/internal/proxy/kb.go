package proxy

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// KB validates the session cookie with identity-svc, checks if the role is admin/manager,
// injects X-Account-ID and X-User-ID headers, and proxies the request to the KB compiler service.
func KB(kbBase, identityBase *url.URL, logger *slog.Logger) http.Handler {
	client := &http.Client{
		Timeout: 25 * time.Second,
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 1. Validate session against identity-svc/auth/me
		authMeURL := fmt.Sprintf("%s://%s/auth/me", identityBase.Scheme, identityBase.Host)
		authReq, err := http.NewRequestWithContext(r.Context(), http.MethodGet, authMeURL, nil)
		if err != nil {
			logger.Error("kbProxy: create auth request", "error", err)
			http.Error(w, "gateway error", http.StatusBadGateway)
			return
		}

		// Forward Cookie header
		if cookie := r.Header.Get("Cookie"); cookie != "" {
			authReq.Header.Set("Cookie", cookie)
		}

		authResp, err := client.Do(authReq)
		if err != nil {
			logger.Error("kbProxy: call identity-svc failed", "error", err)
			http.Error(w, "identity service unavailable", http.StatusBadGateway)
			return
		}
		defer authResp.Body.Close()

		if authResp.StatusCode != http.StatusOK {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, `{"error":"unauthenticated"}`)
			return
		}

		// Parse user details
		var authMe struct {
			UserID    string `json:"user_id"`
			AccountID string `json:"account_id"`
			Role      string `json:"role"`
		}
		if err := json.NewDecoder(authResp.Body).Decode(&authMe); err != nil {
			logger.Error("kbProxy: decode auth response", "error", err)
			http.Error(w, "invalid auth response", http.StatusInternalServerError)
			return
		}

		// 2. Enforce manager role
		if authMe.Role != "manager" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, `{"error":"forbidden: insufficient role"}`)
			return
		}

		// 3. Forward request to kb-compiler (rewriting /api/kb/ to /internal/kb/)
		targetPath := strings.Replace(r.URL.Path, "/api/kb/", "/internal/kb/", 1)
		target := *kbBase
		target.Path = strings.TrimRight(target.Path, "/") + targetPath
		target.RawQuery = r.URL.RawQuery

		req, err := http.NewRequestWithContext(r.Context(), r.Method, target.String(), r.Body)
		if err != nil {
			logger.Error("kbProxy: create forwarding request", "error", err)
			http.Error(w, "gateway error", http.StatusBadGateway)
			return
		}

		// Forward headers (excluding untrusted client internal headers)
		for key, vals := range r.Header {
			if IsInternalHeader(key) {
				continue
			}
			for _, v := range vals {
				req.Header.Add(key, v)
			}
		}

		// Inject trusted tenant, user, and internal service auth headers
		req.Header.Set("X-Account-ID", authMe.AccountID)
		req.Header.Set("X-User-ID", authMe.UserID)
		internalSecret := os.Getenv("INTERNAL_SERVICE_TOKEN")
		if internalSecret == "" {
			internalSecret = os.Getenv("SESSION_SECRET")
		}
		if internalSecret == "" {
			internalSecret = "change-me-in-production-at-least-32-chars"
		}
		req.Header.Set("X-Internal-Token", internalSecret)

		resp, err := client.Do(req)
		if err != nil {
			logger.Error("kbProxy: upstream error", "target", target.String(), "error", err)
			http.Error(w, "gateway error: upstream unavailable", http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()

		for key, vals := range resp.Header {
			for _, v := range vals {
				w.Header().Add(key, v)
			}
		}
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
	})
}

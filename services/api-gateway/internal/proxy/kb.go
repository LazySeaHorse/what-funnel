package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/whatfunnel/whatfunnel/packages/go-common/middleware"
)

type kbIdentityKey struct{}

type kbIdentity struct{ UserID, AccountID string }

// KB validates the session cookie with identity-svc, requires the manager role,
// injects X-Account-ID and X-User-ID headers, and proxies the request to the KB compiler service.
func KB(kbBase, identityBase *url.URL, logger *slog.Logger) http.Handler {
	// Session validation is a small request; bound it and never follow redirects.
	authClient := &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	authMeURL := *identityBase
	authMeURL.Path = strings.TrimRight(authMeURL.Path, "/") + "/auth/me"
	authMeURL.RawPath = ""
	authMeURL.RawQuery = ""

	// The trusted identity reaches the rewrite through the request context.
	reverse := newReverseProxy(kbBase, logger, func(pr *httputil.ProxyRequest) {
		identity, _ := pr.In.Context().Value(kbIdentityKey{}).(kbIdentity)
		// Rewrite /api/kb/ to /internal/kb/.
		pr.Out.URL.Path = strings.Replace(pr.Out.URL.Path, "/api/kb/", "/internal/kb/", 1)
		pr.Out.URL.RawPath = ""
		// Inject trusted tenant, user, and internal service auth headers.
		pr.Out.Header.Set("X-Account-ID", identity.AccountID)
		pr.Out.Header.Set("X-User-ID", identity.UserID)
		if internalSecret := middleware.InternalServiceSecret(); internalSecret != "" {
			pr.Out.Header.Set("X-Internal-Token", internalSecret)
		}
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 1. Validate session against identity-svc/auth/me
		authReq, err := http.NewRequestWithContext(r.Context(), http.MethodGet, authMeURL.String(), nil)
		if err != nil {
			logger.Error("kbProxy: create auth request", "error", err)
			http.Error(w, "gateway error", http.StatusBadGateway)
			return
		}
		if cookie := r.Header.Get("Cookie"); cookie != "" {
			authReq.Header.Set("Cookie", cookie)
		}
		authResp, err := authClient.Do(authReq)
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

		// 3. Forward the request to kb-compiler.
		ctx := context.WithValue(r.Context(), kbIdentityKey{}, kbIdentity{UserID: authMe.UserID, AccountID: authMe.AccountID})
		reverse.ServeHTTP(w, r.WithContext(ctx))
	})
}

package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCSRFProtection_SafeMethodsAllowed(t *testing.T) {
	mw := CSRFProtection()
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for _, method := range []string{http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace} {
		req := httptest.NewRequest(method, "/workspace/account", nil)
		req.AddCookie(&http.Cookie{Name: "whatfunnel_session", Value: "valid-session"})
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code, "method %s should be allowed", method)
	}
}

func TestCSRFProtection_PublicEndpointsAllowed(t *testing.T) {
	mw := CSRFProtection()
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	publicPaths := []string{
		"/healthz",
		"/auth/login",
		"/auth/signup",
		"/webhooks/whatsapp",
		"/simulate-inbound",
	}

	for _, path := range publicPaths {
		req := httptest.NewRequest(http.MethodPost, path, nil)
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code, "path %s should be allowed without csrf", path)
	}
}

func TestCSRFProtection_NoSessionCookieAllowed(t *testing.T) {
	mw := CSRFProtection()
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Mutating request without session cookie (e.g. unauthenticated, to be handled by auth mw)
	req := httptest.NewRequest(http.MethodPost, "/workspace/account", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestCSRFProtection_SessionCookieWithoutCSRFRejected(t *testing.T) {
	mw := CSRFProtection()
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Mutating request with session cookie and csrf_token cookie, but no CSRF header
	req := httptest.NewRequest(http.MethodPost, "/workspace/account", nil)
	req.AddCookie(&http.Cookie{Name: "whatfunnel_session", Value: "valid-session"})
	req.AddCookie(&http.Cookie{Name: "csrf_token", Value: "token-abc"})
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusForbidden, rr.Code)
	assert.Contains(t, rr.Body.String(), "invalid or missing CSRF token")
}

func TestCSRFProtection_SessionCookieWithMismatchedCSRFRejected(t *testing.T) {
	mw := CSRFProtection()
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/workspace/account", nil)
	req.AddCookie(&http.Cookie{Name: "whatfunnel_session", Value: "valid-session"})
	req.AddCookie(&http.Cookie{Name: "csrf_token", Value: "token-abc"})
	req.Header.Set("X-CSRF-Token", "wrong-token")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusForbidden, rr.Code)
}

func TestCSRFProtection_SessionCookieWithValidCSRFAllowed(t *testing.T) {
	mw := CSRFProtection()
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/workspace/account", nil)
	req.AddCookie(&http.Cookie{Name: "whatfunnel_session", Value: "valid-session"})
	req.AddCookie(&http.Cookie{Name: "csrf_token", Value: "token-abc"})
	req.Header.Set("X-CSRF-Token", "token-abc")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestCSRFProtection_CrossOriginOriginRejected(t *testing.T) {
	mw := CSRFProtection()
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "http://localhost:8080/workspace/account", nil)
	req.Host = "localhost:8080"
	req.AddCookie(&http.Cookie{Name: "whatfunnel_session", Value: "valid-session"})
	req.AddCookie(&http.Cookie{Name: "csrf_token", Value: "token-abc"})
	req.Header.Set("X-CSRF-Token", "token-abc")
	req.Header.Set("Origin", "http://evil-attacker.com")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusForbidden, rr.Code)
	assert.Contains(t, rr.Body.String(), "cross-origin request rejected")
}

func TestCSRFProtection_InternalTokenAllowed(t *testing.T) {
	mw := CSRFProtection()
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/workspace/account", nil)
	req.AddCookie(&http.Cookie{Name: "whatfunnel_session", Value: "valid-session"})
	req.Header.Set("X-Internal-Token", "change-me-in-production-at-least-32-chars")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)
}

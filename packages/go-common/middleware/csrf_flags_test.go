package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func csrfSimulateStatus(t *testing.T) int {
	t.Helper()
	handler := CSRFProtection()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodPost, "/simulate-inbound", nil)
	req.AddCookie(&http.Cookie{Name: "whatfunnel_session", Value: "valid-session"})
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	return rr.Code
}

func TestCSRFProtection_SimulateInboundExemptOnlyWithFlag(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("ENABLE_SIMULATION_ROUTES", "")
	assert.Equal(t, http.StatusForbidden, csrfSimulateStatus(t))

	t.Setenv("ENABLE_SIMULATION_ROUTES", "true")
	assert.Equal(t, http.StatusOK, csrfSimulateStatus(t))
}

func TestCSRFProtection_WildcardOriginRejectedInProduction(t *testing.T) {
	do := func() int {
		handler := CSRFProtection("*")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		req := httptest.NewRequest(http.MethodPost, "/workspace/account", nil)
		req.Host = "app.example.com"
		req.AddCookie(&http.Cookie{Name: "whatfunnel_session", Value: "s"})
		req.Header.Set("Origin", "https://evil.example.org")
		req.Header.Set("X-Requested-With", "x")
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		return rr.Code
	}
	t.Setenv("APP_ENV", "production")
	assert.Equal(t, http.StatusForbidden, do())
	t.Setenv("APP_ENV", "development")
	assert.Equal(t, http.StatusOK, do())
}

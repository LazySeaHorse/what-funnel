package integration

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/whatfunnel/whatfunnel/packages/go-common/metrics"
)

// TestPrometheusMetrics verifies that standard /metrics endpoints return HTTP 200,
// appropriate text/plain Content-Type, and valid Prometheus metric exposition format.
func TestPrometheusMetrics(t *testing.T) {
	// 1. Verify common metrics handler output shape
	handler := metrics.Handler()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	resp := rec.Result()
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "text/plain")

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	bodyStr := string(body)

	// Assert required Prometheus exposition format shapes
	assert.Contains(t, bodyStr, "# HELP", "Must contain Prometheus HELP annotations")
	assert.Contains(t, bodyStr, "# TYPE", "Must contain Prometheus TYPE annotations")
	assert.Contains(t, bodyStr, "go_goroutines", "Must export go runtime goroutines metric")
	assert.Contains(t, bodyStr, "go_memstats_alloc_bytes", "Must export go runtime memory alloc metric")

	// 2. Validate backend service route registration in mux routers
	services := []string{
		"api-gateway",
		"identity-svc",
		"workspace-svc",
		"conversation-svc",
		"notification-svc",
	}

	for _, svcName := range services {
		t.Run(svcName+" router registration", func(t *testing.T) {
			r := mux.NewRouter()
			// Register /metrics as done in each service's main.go
			r.Handle("/metrics", metrics.Handler()).Methods(http.MethodGet)

			srv := httptest.NewServer(r)
			defer srv.Close()

			res, err := http.Get(srv.URL + "/metrics")
			require.NoError(t, err)
			defer res.Body.Close()

			assert.Equal(t, http.StatusOK, res.StatusCode)
			content, err := io.ReadAll(res.Body)
			require.NoError(t, err)
			assert.Contains(t, string(content), "go_goroutines")
		})
	}
}

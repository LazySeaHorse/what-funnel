package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGatewayMetricsEndpoint(t *testing.T) {
	gw := httptest.NewServer(buildRouter(t, "http://fake-id", "http://fake-kb"))
	defer gw.Close()

	resp, err := http.Get(gw.URL + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "text/plain") {
		t.Errorf("expected text/plain Content-Type, got: %s", contentType)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read body: %v", err)
	}
	bodyStr := string(body)

	if !strings.Contains(bodyStr, "go_goroutines") {
		t.Errorf("expected go_goroutines metric in output")
	}
	if !strings.Contains(bodyStr, "# HELP") || !strings.Contains(bodyStr, "# TYPE") {
		t.Errorf("expected Prometheus format header (# HELP, # TYPE)")
	}
}

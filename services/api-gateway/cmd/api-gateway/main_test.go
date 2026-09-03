package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

// startFakeIdentity returns a fake identity-svc responding to GET /auth/me.
// When authed is true it returns a valid session with the given role.
func startFakeIdentity(t *testing.T, authed bool, role string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/auth/me" {
			http.NotFound(w, r)
			return
		}
		if !authed {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, `{"error":"unauthenticated"}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w,
			`{"user_id":"00000000-0000-0000-0000-000000000001","account_id":"00000000-0000-0000-0000-000000000002","role":%q}`,
			role,
		)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// startFakeKB returns a fake KB compiler that records every path it receives.
func startFakeKB(t *testing.T) (*httptest.Server, *[]string) {
	t.Helper()
	var received []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received = append(received, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"concepts":[]}`)
	}))
	t.Cleanup(srv.Close)
	return srv, &received
}

// buildRouter wires up the same mux as main(), pointing at the given fake upstreams.
func buildRouter(t *testing.T, identityURL, kbURL string) http.Handler {
	t.Helper()
	logger := newTestLogger()

	identityBase, err := url.Parse(identityURL)
	if err != nil {
		t.Fatal(err)
	}
	kbBase, err := url.Parse(kbURL)
	if err != nil {
		t.Fatal(err)
	}

	// kbProxy and newRouter are defined in main.go (same package).
	return newRouter(kbBase, identityBase, identityBase, identityBase, identityBase, logger)
}

// ---------------------------------------------------------------------------
// routing tests
// ---------------------------------------------------------------------------

// TestBareKBPathReturns404 verifies that /kb/* — the old broken prefix —
// hits the catch-all 404 and is never forwarded to the KB compiler.
func TestBareKBPathReturns404(t *testing.T) {
	identitySrv := startFakeIdentity(t, true, "manager")
	kbSrv, received := startFakeKB(t)
	gw := httptest.NewServer(buildRouter(t, identitySrv.URL, kbSrv.URL))
	t.Cleanup(gw.Close)

	resp, err := http.Get(gw.URL + "/kb/concepts")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("want 404 for /kb/concepts, got %d", resp.StatusCode)
	}
	if len(*received) != 0 {
		t.Errorf("want 0 requests to kb-compiler for /kb/concepts, got %v", *received)
	}
}

// TestAPIKBPathRoutesToKBCompiler verifies that /api/kb/* is forwarded after auth.
func TestAPIKBPathRoutesToKBCompiler(t *testing.T) {
	identitySrv := startFakeIdentity(t, true, "manager")
	kbSrv, received := startFakeKB(t)
	gw := httptest.NewServer(buildRouter(t, identitySrv.URL, kbSrv.URL))
	t.Cleanup(gw.Close)

	resp, err := http.Get(gw.URL + "/api/kb/concepts")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("want 200 for /api/kb/concepts, got %d", resp.StatusCode)
	}
	if len(*received) == 0 {
		t.Fatal("want kb-compiler to receive a request, got none")
	}
	if !strings.HasPrefix((*received)[0], "/internal/kb/") {
		t.Errorf("want path rewritten to /internal/kb/..., got %q", (*received)[0])
	}
}

// TestPathRewrites checks every KB URL the frontend uses is correctly rewritten.
func TestPathRewrites(t *testing.T) {
	cases := []struct{ in, want string }{
		{"/api/kb/concepts", "/internal/kb/concepts"},
		{"/api/kb/patterns", "/internal/kb/patterns"},
		{"/api/kb/suggestions", "/internal/kb/suggestions"},
		{"/api/kb/mining-runs/latest", "/internal/kb/mining-runs/latest"},
		{"/api/kb/compile-paste", "/internal/kb/compile-paste"},
		{"/api/kb/mine/trigger", "/internal/kb/mine/trigger"},
	}

	identitySrv := startFakeIdentity(t, true, "manager")
	kbSrv, received := startFakeKB(t)
	gw := httptest.NewServer(buildRouter(t, identitySrv.URL, kbSrv.URL))
	t.Cleanup(gw.Close)

	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			*received = nil
			resp, err := http.Get(gw.URL + tc.in)
			if err != nil {
				t.Fatalf("request error: %v", err)
			}
			resp.Body.Close()
			if len(*received) == 0 {
				t.Fatal("kb-compiler received no request")
			}
			if (*received)[0] != tc.want {
				t.Errorf("want %q, got %q", tc.want, (*received)[0])
			}
		})
	}
}

// TestAPIKBRequiresAdmin verifies non-admin sessions are rejected with 403.
func TestAPIKBRequiresAdmin(t *testing.T) {
	identitySrv := startFakeIdentity(t, true, "agent")
	kbSrv, received := startFakeKB(t)
	gw := httptest.NewServer(buildRouter(t, identitySrv.URL, kbSrv.URL))
	t.Cleanup(gw.Close)

	resp, err := http.Get(gw.URL + "/api/kb/concepts")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("want 403 for non-admin, got %d", resp.StatusCode)
	}
	if len(*received) != 0 {
		t.Errorf("want 0 requests to kb-compiler for non-admin, got %v", *received)
	}
}

// TestAPIKBRequiresAuth verifies unauthenticated requests are rejected with 401.
func TestAPIKBRequiresAuth(t *testing.T) {
	identitySrv := startFakeIdentity(t, false, "")
	kbSrv, received := startFakeKB(t)
	gw := httptest.NewServer(buildRouter(t, identitySrv.URL, kbSrv.URL))
	t.Cleanup(gw.Close)

	resp, err := http.Get(gw.URL + "/api/kb/concepts")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("want 401 for unauthenticated, got %d", resp.StatusCode)
	}
	if len(*received) != 0 {
		t.Errorf("want 0 requests to kb-compiler for unauthenticated, got %v", *received)
	}
}

// TestKBProxyInjectsTenantHeaders verifies X-Account-ID and X-User-ID are
// injected from the identity-svc response into the upstream request.
func TestKBProxyInjectsTenantHeaders(t *testing.T) {
	identitySrv := startFakeIdentity(t, true, "manager")

	var gotAccountID, gotUserID string
	kbSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAccountID = r.Header.Get("X-Account-ID")
		gotUserID = r.Header.Get("X-User-ID")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"concepts":[]}`)
	}))
	t.Cleanup(kbSrv.Close)

	gw := httptest.NewServer(buildRouter(t, identitySrv.URL, kbSrv.URL))
	t.Cleanup(gw.Close)

	resp, err := http.Get(gw.URL + "/api/kb/concepts")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if gotAccountID != "00000000-0000-0000-0000-000000000002" {
		t.Errorf("want X-Account-ID injected, got %q", gotAccountID)
	}
	if gotUserID != "00000000-0000-0000-0000-000000000001" {
		t.Errorf("want X-User-ID injected, got %q", gotUserID)
	}
}

// ---------------------------------------------------------------------------
// Response-shape contract test
// Verifies the gateway passes through the KB compiler's JSON without mangling
// field names, and that the actual field names match what the frontend expects.
// ---------------------------------------------------------------------------

func TestGatewayForwardsKBResponseShape(t *testing.T) {
	// The exact JSON shape the fixed frontend now reads.
	kbBody := `{"concepts":[{
		"id":"abc","slug":"test","type":"faq","title":"T",
		"tags":[],"body_text":"B","source":"owner_pasted",
		"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"
	}]}`

	identitySrv := startFakeIdentity(t, true, "manager")
	kbSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, kbBody)
	}))
	t.Cleanup(kbSrv.Close)

	gw := httptest.NewServer(buildRouter(t, identitySrv.URL, kbSrv.URL))
	t.Cleanup(gw.Close)

	resp, err := http.Get(gw.URL + "/api/kb/concepts")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	var out struct {
		Concepts []map[string]any `json:"concepts"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(out.Concepts) == 0 {
		t.Fatal("want at least one concept in response")
	}

	c := out.Concepts[0]
	for _, field := range []string{"id", "slug", "type", "title", "body_text", "source"} {
		if _, ok := c[field]; !ok {
			t.Errorf("concept response missing expected field %q", field)
		}
	}
	// Confirm the old phantom field names are NOT present.
	for _, phantom := range []string{"concept_type", "content", "source_type"} {
		if _, ok := c[phantom]; ok {
			t.Errorf("concept response contains phantom field %q that frontend no longer uses", phantom)
		}
	}
}

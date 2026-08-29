package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGatewayCSRFProtection(t *testing.T) {
	identitySrv := startFakeIdentity(t, true, "manager")
	kbSrv, received := startFakeKB(t)
	gw := httptest.NewServer(buildRouter(t, identitySrv.URL, kbSrv.URL))
	t.Cleanup(gw.Close)

	t.Run("Mutating request with session and csrf cookie without header is blocked", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodPost, gw.URL+"/api/kb/compile-paste", strings.NewReader(`{"text":"test"}`))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "whatfunnel_session", Value: "valid-session"})
		req.AddCookie(&http.Cookie{Name: "csrf_token", Value: "valid-csrf-token"})

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("want 403 Forbidden, got %d", resp.StatusCode)
		}
	})

	t.Run("Mutating request with session and matching CSRF token header succeeds", func(t *testing.T) {
		*received = nil
		req, _ := http.NewRequest(http.MethodPost, gw.URL+"/api/kb/compile-paste", strings.NewReader(`{"text":"test"}`))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "whatfunnel_session", Value: "valid-session"})
		req.AddCookie(&http.Cookie{Name: "csrf_token", Value: "valid-csrf-token"})
		req.Header.Set("X-CSRF-Token", "valid-csrf-token")

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("want 200 OK, got %d", resp.StatusCode)
		}
		if len(*received) == 0 {
			t.Fatal("expected request to reach kb-compiler")
		}
	})

	t.Run("Public unauthenticated routes bypass CSRF", func(t *testing.T) {
		// Mock upstream login handler
		loginMock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"status":"ok"}`)
		}))
		t.Cleanup(loginMock.Close)

		gwLogin := httptest.NewServer(buildRouter(t, loginMock.URL, kbSrv.URL))
		t.Cleanup(gwLogin.Close)

		req, _ := http.NewRequest(http.MethodPost, gwLogin.URL+"/auth/login", strings.NewReader(`{"email":"test@test.com","password":"pwd"}`))
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("want 200 OK for /auth/login without csrf, got %d", resp.StatusCode)
		}
	})
}

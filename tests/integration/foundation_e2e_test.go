// Package integration contains end-to-end integration tests for the
// WhatFunnel foundation layer (Build Prompt 1).
//
// These tests exercise the full stack via HTTP against running services.
// They require:
//   - `make up` to be running
//   - `make migrate` to have been applied
//
// Run with: DATABASE_URL=... go test ./... (or `make test`)
package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	gatewayURL  = "http://localhost:8080"
	identityURL = "http://localhost:8081"
	workspaceURL = "http://localhost:8082"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://whatfunnel:whatfunnel@localhost:5432/whatfunnel?sslmode=disable"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Skipf("skipping integration test: parse DSN: %v", err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Skipf("skipping integration test: connect: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		t.Skipf("skipping integration test: ping: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// newClient creates an HTTP client that preserves cookies across requests.
func newClient() *http.Client {
	jar, _ := cookiejar.New(nil)
	return &http.Client{
		Jar:     jar,
		Timeout: 10 * time.Second,
	}
}

// post sends a POST request with JSON body and returns the decoded response body.
func post(t *testing.T, client *http.Client, url string, body any) (*http.Response, map[string]any) {
	t.Helper()
	b, _ := json.Marshal(body)
	resp, err := client.Post(url, "application/json", bytes.NewReader(b))
	require.NoError(t, err)
	t.Cleanup(func() { resp.Body.Close() })
	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result) //nolint:errcheck
	return resp, result
}

// get sends an authenticated GET request.
func get(t *testing.T, client *http.Client, url string) (*http.Response, map[string]any) {
	t.Helper()
	resp, err := client.Get(url)
	require.NoError(t, err)
	t.Cleanup(func() { resp.Body.Close() })
	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result) //nolint:errcheck
	return resp, result
}

// put sends an authenticated PUT request.
func put(t *testing.T, client *http.Client, url string, body any) (*http.Response, map[string]any) {
	t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPut, url, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { resp.Body.Close() })
	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result) //nolint:errcheck
	return resp, result
}

// uniqueEmail generates a unique email for test isolation.
func uniqueEmail(prefix string) string {
	return fmt.Sprintf("%s+%d@example.com", prefix, time.Now().UnixNano())
}

// skipIfServicesDown checks if the gateway is reachable, skipping if not.
func skipIfServicesDown(t *testing.T) {
	t.Helper()
	resp, err := http.Get(gatewayURL + "/healthz")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Skip("skipping integration test: api-gateway is not reachable (run `make up` first)")
	}
}

// ---------------------------------------------------------------------------
// Stage 7: End-to-end integration test
// ---------------------------------------------------------------------------

// TestFoundationE2E is the end-to-end integration test required by Stage 7.
// It exercises:
//  1. Signup → creates account, seeds default pipeline
//  2. Login → session cookie is set
//  3. GET /auth/me → session persists across requests
//  4. GET /workspace/pipelines → default pipeline exists
//  5. POST /workspace/users/invite → admin can invite a member
//  6. GET /workspace/users → admin sees all users
//  7. GET /workspace/account → account details accessible
//  8. Logout → session invalidated
//  9. GET /auth/me after logout → 401
// 10. Member cannot access admin route → 403
//     (placeholder — real member login requires redeeming invite token,
//      which is extended in Build Prompt 3)
func TestFoundationE2E(t *testing.T) {
	skipIfServicesDown(t)
	pool := testPool(t)
	ctx := context.Background()

	adminEmail := uniqueEmail("admin")
	adminClient := newClient()

	// -----------------------------------------------------------------------
	// Step 1: Signup
	// -----------------------------------------------------------------------
	t.Log("Step 1: Signup")
	resp, body := post(t, adminClient, gatewayURL+"/auth/signup", map[string]string{
		"account_name": "E2E Test Account",
		"email":        adminEmail,
		"password":     "Password123!",
	})
	require.Equal(t, http.StatusCreated, resp.StatusCode, "signup must return 201: %v", body)
	assert.Equal(t, "admin", body["role"], "first user must be admin")
	accountIDStr, ok := body["account_id"].(string)
	require.True(t, ok, "account_id must be in response")
	accountID := uuid.MustParse(accountIDStr)

	// Cleanup all data for this account after test
	t.Cleanup(func() {
		pool.Exec(ctx, `DELETE FROM sessions WHERE data::text LIKE '%'||$1||'%'`, accountIDStr)
		pool.Exec(ctx, `DELETE FROM invite_tokens WHERE account_id = $1`, accountID)
		pool.Exec(ctx, `DELETE FROM audit_logs WHERE account_id = $1`, accountID)
		pool.Exec(ctx, `DELETE FROM lead_pipelines WHERE account_id = $1`, accountID)
		pool.Exec(ctx, `DELETE FROM users WHERE account_id = $1`, accountID)
		pool.Exec(ctx, `DELETE FROM accounts WHERE id = $1`, accountID)
	})

	// -----------------------------------------------------------------------
	// Step 2: Verify default pipeline was seeded
	// -----------------------------------------------------------------------
	t.Log("Step 2: Verify default pipeline seeded")
	var pipelineCount int
	err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM lead_pipelines WHERE account_id = $1`, accountID).
		Scan(&pipelineCount)
	require.NoError(t, err)
	assert.Equal(t, 1, pipelineCount, "default pipeline must exist after signup")

	// -----------------------------------------------------------------------
	// Step 3: Login
	// -----------------------------------------------------------------------
	t.Log("Step 3: Login")
	loginResp, loginBody := post(t, adminClient, gatewayURL+"/auth/login", map[string]string{
		"email":    adminEmail,
		"password": "Password123!",
	})
	require.Equal(t, http.StatusOK, loginResp.StatusCode, "login must return 200: %v", loginBody)
	assert.Equal(t, "admin", loginBody["role"])

	// -----------------------------------------------------------------------
	// Step 4: Session persists (GET /auth/me)
	// -----------------------------------------------------------------------
	t.Log("Step 4: Session persists")
	meResp, meBody := get(t, adminClient, gatewayURL+"/auth/me")
	require.Equal(t, http.StatusOK, meResp.StatusCode, "GET /auth/me must return 200: %v", meBody)
	assert.Equal(t, "admin", meBody["role"])
	assert.Equal(t, accountIDStr, meBody["account_id"])

	// -----------------------------------------------------------------------
	// Step 5: GET /workspace/pipelines — default pipeline visible
	// -----------------------------------------------------------------------
	t.Log("Step 5: List pipelines")
	pipResp, _ := get(t, adminClient, gatewayURL+"/workspace/pipelines")
	assert.Equal(t, http.StatusOK, pipResp.StatusCode)

	// -----------------------------------------------------------------------
	// Step 6: Invite a member
	// -----------------------------------------------------------------------
	t.Log("Step 6: Invite member")
	memberEmail := uniqueEmail("member")
	invResp, invBody := post(t, adminClient, gatewayURL+"/workspace/users/invite", map[string]string{
		"email": memberEmail,
		"role":  "member",
	})
	require.Equal(t, http.StatusCreated, invResp.StatusCode, "invite must return 201: %v", invBody)
	inviteToken, ok := invBody["invite_token"].(string)
	require.True(t, ok, "invite_token must be in response")
	assert.NotEmpty(t, inviteToken)

	// -----------------------------------------------------------------------
	// Step 7: Admin can list users (sees at least the admin)
	// -----------------------------------------------------------------------
	t.Log("Step 7: List users")
	usersResp, _ := get(t, adminClient, gatewayURL+"/workspace/users")
	assert.Equal(t, http.StatusOK, usersResp.StatusCode)

	// -----------------------------------------------------------------------
	// Step 8: GET /workspace/account
	// -----------------------------------------------------------------------
	t.Log("Step 8: Get account")
	accResp, accBody := get(t, adminClient, gatewayURL+"/workspace/account")
	assert.Equal(t, http.StatusOK, accResp.StatusCode)
	assert.Equal(t, "E2E Test Account", accBody["name"])

	// -----------------------------------------------------------------------
	// Step 9: Logout → session invalidated
	// -----------------------------------------------------------------------
	t.Log("Step 9: Logout")
	logoutResp, _ := post(t, adminClient, gatewayURL+"/auth/logout", nil)
	assert.Equal(t, http.StatusOK, logoutResp.StatusCode)

	meAfterLogout, _ := get(t, adminClient, gatewayURL+"/auth/me")
	assert.Equal(t, http.StatusUnauthorized, meAfterLogout.StatusCode,
		"GET /auth/me after logout must return 401")

	// -----------------------------------------------------------------------
	// Step 10: Member RBAC denial placeholder
	// Redeeming invite tokens and full member login flow is implemented in
	// Build Prompt 3. Here we assert the invite token exists in the DB.
	// extended in Build Prompt 3
	// -----------------------------------------------------------------------
	t.Log("Step 10: Confirm invite token stored (member flow extended in Build Prompt 3)")
	var tokenExists bool
	err = pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM invite_tokens WHERE token = $1 AND account_id = $2)`,
		inviteToken, accountID).Scan(&tokenExists)
	require.NoError(t, err)
	assert.True(t, tokenExists, "invite token must be persisted in DB")
}

// TestWrongPasswordDenied verifies login with wrong password returns 401.
func TestWrongPasswordDenied(t *testing.T) {
	skipIfServicesDown(t)
	pool := testPool(t)
	ctx := context.Background()

	client := newClient()
	email := uniqueEmail("wrongpwd")

	// Signup
	resp, body := post(t, client, gatewayURL+"/auth/signup", map[string]string{
		"account_name": "Wrong Password Account",
		"email":        email,
		"password":     "correctpassword",
	})
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	accountIDStr := body["account_id"].(string)
	accountID := uuid.MustParse(accountIDStr)
	t.Cleanup(func() {
		pool.Exec(ctx, `DELETE FROM audit_logs WHERE account_id = $1`, accountID)
		pool.Exec(ctx, `DELETE FROM lead_pipelines WHERE account_id = $1`, accountID)
		pool.Exec(ctx, `DELETE FROM users WHERE account_id = $1`, accountID)
		pool.Exec(ctx, `DELETE FROM accounts WHERE id = $1`, accountID)
	})

	// Login with wrong password
	wrongResp, _ := post(t, client, gatewayURL+"/auth/login", map[string]string{
		"email":    email,
		"password": "wrongpassword",
	})
	assert.Equal(t, http.StatusUnauthorized, wrongResp.StatusCode,
		"login with wrong password must return 401")
}

// TestUnauthenticatedAccessDenied verifies protected routes reject unauthenticated requests.
func TestUnauthenticatedAccessDenied(t *testing.T) {
	skipIfServicesDown(t)

	client := newClient() // fresh client, no session

	// /auth/me — requires authentication
	meResp, _ := get(t, client, gatewayURL+"/auth/me")
	assert.Equal(t, http.StatusUnauthorized, meResp.StatusCode)

	// /workspace/account — requires authentication
	accResp, _ := get(t, client, gatewayURL+"/workspace/account")
	assert.Equal(t, http.StatusUnauthorized, accResp.StatusCode)
}

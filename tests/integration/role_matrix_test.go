package integration

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type endpointTestCase struct {
	name          string
	method        string
	path          string
	payload       any
	anonStatus    int
	agentStatus   int
	managerStatus int
}

// TestFullRoleMatrix validates endpoint authorization and access control
// across Anonymous, Agent, and Manager roles.
func TestFullRoleMatrix(t *testing.T) {
	skipIfServicesDown(t)

	anonClient := newClient()
	managerClient := newClient()
	agentClient := newClient()

	managerEmail := uniqueEmail("matrix_mgr")
	password := "ManagerPass123!"

	// 1. Sign up Manager
	regResp, body := post(t, managerClient, gatewayURL+"/auth/signup", map[string]any{
		"account_name": "Role Matrix Test Co",
		"email":        managerEmail,
		"password":     password,
	})
	require.Equal(t, http.StatusCreated, regResp.StatusCode)
	accountID := body["account_id"].(string)

	pool := testPool(t)
	ctx := t.Context()
	t.Cleanup(func() {
		pool.Exec(ctx, `DELETE FROM sessions WHERE data::text LIKE '%'||$1||'%'`, accountID)
		pool.Exec(ctx, `DELETE FROM users WHERE account_id = $1`, accountID)
		pool.Exec(ctx, `DELETE FROM accounts WHERE id = $1`, accountID)
	})

	// 2. Manager logs in
	loginResp, _ := post(t, managerClient, gatewayURL+"/auth/login", map[string]any{
		"email":    managerEmail,
		"password": password,
	})
	require.Equal(t, http.StatusOK, loginResp.StatusCode)

	// Set slug
	slug := "matrix-ws-" + uuid.NewString()[:8]
	slugResp, _ := put(t, managerClient, gatewayURL+"/workspace/account/slug", map[string]string{
		"slug": slug,
	})
	require.Equal(t, http.StatusOK, slugResp.StatusCode)

	// 3. Create Agent
	agentUsername := "matrix_agent"
	agentPassword := "AgentPass123!"
	createResp, agentBody := post(t, managerClient, gatewayURL+"/workspace/users", map[string]any{
		"username": agentUsername,
		"password": agentPassword,
		"role":     "agent",
	})
	require.Equal(t, http.StatusCreated, createResp.StatusCode, "create agent: %v", agentBody)

	// Agent logs in
	agentLoginResp, _ := post(t, agentClient, gatewayURL+"/auth/login", map[string]any{
		"identifier": fmt.Sprintf("%s-%s", slug, agentUsername),
		"password":   agentPassword,
	})
	require.Equal(t, http.StatusOK, agentLoginResp.StatusCode)

	matrix := []endpointTestCase{
		// Public endpoints
		{
			name:          "GET /healthz",
			method:        http.MethodGet,
			path:          "/healthz",
			anonStatus:    http.StatusOK,
			agentStatus:   http.StatusOK,
			managerStatus: http.StatusOK,
		},
		// Protected Agent endpoints
		{
			name:          "GET /auth/me",
			method:        http.MethodGet,
			path:          "/auth/me",
			anonStatus:    http.StatusUnauthorized,
			agentStatus:   http.StatusOK,
			managerStatus: http.StatusOK,
		},
		{
			name:          "GET /workspace/account",
			method:        http.MethodGet,
			path:          "/workspace/account",
			anonStatus:    http.StatusUnauthorized,
			agentStatus:   http.StatusOK,
			managerStatus: http.StatusOK,
		},
		{
			name:          "GET /conversations",
			method:        http.MethodGet,
			path:          "/conversations",
			anonStatus:    http.StatusUnauthorized,
			agentStatus:   http.StatusOK,
			managerStatus: http.StatusOK,
		},
		// Manager-only endpoints
		{
			name:          "POST /workspace/users",
			method:        http.MethodPost,
			path:          "/workspace/users",
			payload:       map[string]any{"username": "another_user", "password": "Password123!", "role": "agent"},
			anonStatus:    http.StatusUnauthorized,
			agentStatus:   http.StatusForbidden,
			managerStatus: http.StatusCreated,
		},
		{
			name:          "PUT /workspace/account/settings",
			method:        http.MethodPut,
			path:          "/workspace/account/settings",
			payload:       map[string]any{"ai_enabled": true},
			anonStatus:    http.StatusUnauthorized,
			agentStatus:   http.StatusForbidden,
			managerStatus: http.StatusOK,
		},
		{
			name:          "POST /onboarding/apply-template",
			method:        http.MethodPost,
			path:          "/onboarding/apply-template",
			payload:       map[string]any{"business_type": "salon"},
			anonStatus:    http.StatusUnauthorized,
			agentStatus:   http.StatusForbidden,
			managerStatus: http.StatusOK,
		},
	}

	for _, tc := range matrix {
		t.Run(tc.name, func(t *testing.T) {
			targetURL := gatewayURL + tc.path

			executeReq := func(c *http.Client) int {
				switch tc.method {
				case http.MethodGet:
					resp, _ := get(t, c, targetURL)
					return resp.StatusCode
				case http.MethodPost:
					resp, _ := post(t, c, targetURL, tc.payload)
					return resp.StatusCode
				case http.MethodPut:
					resp, _ := put(t, c, targetURL, tc.payload)
					return resp.StatusCode
				default:
					t.Fatalf("unsupported method %s", tc.method)
					return 0
				}
			}

			// Anonymous role check
			anonCode := executeReq(anonClient)
			assert.Equal(t, tc.anonStatus, anonCode, "Anonymous status mismatch for %s", tc.name)

			// Agent role check
			agentCode := executeReq(agentClient)
			assert.Equal(t, tc.agentStatus, agentCode, "Agent status mismatch for %s", tc.name)

			// Manager role check
			mgrCode := executeReq(managerClient)
			assert.Equal(t, tc.managerStatus, mgrCode, "Manager status mismatch for %s", tc.name)
		})
	}
}

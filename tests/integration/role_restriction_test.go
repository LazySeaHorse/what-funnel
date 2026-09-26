package integration

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRoleRestriction_AgentDeniedManagerRoutes tests that logging in as an agent
// and attempting to access manager-only routes blocks access with 403 Forbidden.
func TestRoleRestriction_AgentDeniedManagerRoutes(t *testing.T) {
	skipIfServicesDown(t)

	managerClient := newClient()
	managerEmail := uniqueEmail("mgr_role")
	password := "ManagerPass123!"

	// 1. Register new account — default user is manager
	regResp, _ := post(t, managerClient, gatewayURL+"/auth/signup", map[string]any{
		"account_name": "Role Restriction Test Co",
		"email":        managerEmail,
		"password":     password,
	})
	require.Equal(t, http.StatusCreated, regResp.StatusCode)

	// 2. Manager logs in to establish session
	mgrLoginResp, _ := post(t, managerClient, gatewayURL+"/auth/login", map[string]any{
		"email":    managerEmail,
		"password": password,
	})
	require.Equal(t, http.StatusOK, mgrLoginResp.StatusCode)

	// Set workspace slug for username-based login
	workspaceSlug := "role-ws-" + uuid.NewString()[:8]
	slugResp, slugBody := put(t, managerClient, gatewayURL+"/workspace/account/slug", map[string]string{
		"slug": workspaceSlug,
	})
	require.Equal(t, http.StatusOK, slugResp.StatusCode, "setting slug failed: %v", slugBody)

	// 3. Manager creates an agent user
	agentUsername := "agent_alice"
	agentPassword := "AgentPass123!"
	createAgentResp, agentBody := post(t, managerClient, gatewayURL+"/workspace/users", map[string]any{
		"username": agentUsername,
		"password": agentPassword,
		"role":     "agent",
	})
	require.Equal(t, http.StatusCreated, createAgentResp.StatusCode, "Manager should be able to create agent user: %v", agentBody)

	// 4. Log in as the Agent in a separate client session
	agentClient := newClient()
	loginResp, _ := post(t, agentClient, gatewayURL+"/auth/login", map[string]any{
		"identifier": fmt.Sprintf("%s-%s", workspaceSlug, agentUsername),
		"password":   agentPassword,
	})
	require.Equal(t, http.StatusOK, loginResp.StatusCode, "Agent should log in successfully")

	// 5. Assert that Agent is blocked from Manager-only routes with 403 Forbidden
	t.Run("Agent blocked from GET /workspace/users (403)", func(t *testing.T) {
		resp, _ := get(t, agentClient, gatewayURL+"/workspace/users")
		assert.Equal(t, http.StatusForbidden, resp.StatusCode, "Agent must not be allowed to list all users")
	})

	t.Run("Agent blocked from POST /workspace/users (403)", func(t *testing.T) {
		resp, _ := post(t, agentClient, gatewayURL+"/workspace/users", map[string]any{
			"email":    uniqueEmail("agent_created"),
			"password": "SomePass123!",
			"role":     "agent",
		})
		assert.Equal(t, http.StatusForbidden, resp.StatusCode, "Agent must not be allowed to create users")
	})

	t.Run("Agent blocked from PATCH /workspace/account (403)", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodPatch, gatewayURL+"/workspace/account", nil)
		require.NoError(t, err)
		attachCSRF(req, agentClient)
		resp, err := agentClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusForbidden, resp.StatusCode, "Agent must not be allowed to modify account")
	})

	t.Run("Agent blocked from PUT /workspace/account/settings (403)", func(t *testing.T) {
		resp, _ := put(t, agentClient, gatewayURL+"/workspace/account/settings", map[string]any{
			"settings": map[string]any{"timezone": "America/New_York"},
		})
		assert.Equal(t, http.StatusForbidden, resp.StatusCode, "Agent must not be allowed to update settings")
	})

	t.Run("Agent blocked from POST /onboarding/apply-template (403)", func(t *testing.T) {
		resp, _ := post(t, agentClient, gatewayURL+"/onboarding/apply-template", map[string]any{
			"business_type": "ecommerce",
		})
		assert.Equal(t, http.StatusForbidden, resp.StatusCode, "Agent must not be allowed to apply onboarding templates")
	})

	// 6. Assert that Agent CAN access permitted agent routes
	t.Run("Agent allowed to access GET /workspace/account (200)", func(t *testing.T) {
		resp, body := get(t, agentClient, gatewayURL+"/workspace/account")
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.NotEmpty(t, body["id"])
	})

	t.Run("Agent allowed to access GET /users/me/reply-mode (200)", func(t *testing.T) {
		resp, _ := get(t, agentClient, gatewayURL+"/users/me/reply-mode")
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})
}

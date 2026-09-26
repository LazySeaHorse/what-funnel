package integration

import (
	"bytes"
	"fmt"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestInvalidHTTPInputValidation tests that invalid or empty payloads to POST/PUT routes
// return structured 400 or 422 error codes.
func TestInvalidHTTPInputValidation(t *testing.T) {
	skipIfServicesDown(t)
	client := newClient()

	// 1. Public Auth routes with invalid / empty inputs
	t.Run("POST /auth/signup with empty JSON", func(t *testing.T) {
		resp, body := post(t, client, gatewayURL+"/auth/signup", map[string]any{})
		assert.Contains(t, []int{http.StatusBadRequest, http.StatusUnprocessableEntity}, resp.StatusCode)
		assert.NotEmpty(t, body["error"], "Expected structured error field in response")
	})

	t.Run("POST /auth/signup with missing email", func(t *testing.T) {
		resp, body := post(t, client, gatewayURL+"/auth/signup", map[string]any{
			"account_name": "Test Co",
			"password":     "ValidPass123!",
		})
		assert.Contains(t, []int{http.StatusBadRequest, http.StatusUnprocessableEntity}, resp.StatusCode)
		assert.NotEmpty(t, body["error"])
	})

	t.Run("POST /auth/signup with empty password", func(t *testing.T) {
		resp, body := post(t, client, gatewayURL+"/auth/signup", map[string]any{
			"account_name": "Test Co",
			"email":        uniqueEmail("emptypass"),
			"password":     "",
		})
		assert.Contains(t, []int{http.StatusBadRequest, http.StatusUnprocessableEntity}, resp.StatusCode)
		assert.NotEmpty(t, body["error"])
	})

	t.Run("POST /auth/login with empty body", func(t *testing.T) {
		resp, body := post(t, client, gatewayURL+"/auth/login", map[string]any{})
		assert.Contains(t, []int{http.StatusBadRequest, http.StatusUnprocessableEntity}, resp.StatusCode)
		assert.NotEmpty(t, body["error"])
	})

	t.Run("POST /auth/login with malformed body", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodPost, gatewayURL+"/auth/login", bytes.NewReader([]byte("{invalid-json")))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		attachCSRF(req, client)
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	// 2. Authenticated routes with invalid / empty inputs
	email := uniqueEmail("inputval")
	password := "SecurePassword123!"
	regResp, _ := post(t, client, gatewayURL+"/auth/signup", map[string]any{
		"account_name": "Input Validation Account",
		"email":        email,
		"password":     password,
	})
	require.Equal(t, http.StatusCreated, regResp.StatusCode)

	loginResp, _ := post(t, client, gatewayURL+"/auth/login", map[string]any{
		"email":    email,
		"password": password,
	})
	require.Equal(t, http.StatusOK, loginResp.StatusCode)

	t.Run("POST /workspace/users with empty payload", func(t *testing.T) {
		resp, body := post(t, client, gatewayURL+"/workspace/users", map[string]any{})
		assert.Contains(t, []int{http.StatusBadRequest, http.StatusUnprocessableEntity}, resp.StatusCode)
		assert.NotEmpty(t, body["error"])
	})

	t.Run("POST /workspace/users with invalid role", func(t *testing.T) {
		resp, body := post(t, client, gatewayURL+"/workspace/users", map[string]any{
			"email":    uniqueEmail("badrole"),
			"password": "ValidPassword123!",
			"role":     "superadmin_invalid",
		})
		assert.Contains(t, []int{http.StatusBadRequest, http.StatusUnprocessableEntity}, resp.StatusCode)
		assert.NotEmpty(t, body["error"])
	})

	t.Run("POST /conversations/{id}/send with empty payload", func(t *testing.T) {
		dummyConvoID := uuid.New()
		sendURL := fmt.Sprintf("%s/conversations/%s/send", gatewayURL, dummyConvoID)
		resp, body := post(t, client, sendURL, map[string]any{})
		assert.Contains(t, []int{http.StatusBadRequest, http.StatusUnprocessableEntity, http.StatusNotFound}, resp.StatusCode)
		assert.NotEmpty(t, body["error"])
	})

	t.Run("PUT /workspace/account/settings with invalid JSON structure", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodPut, gatewayURL+"/workspace/account/settings", bytes.NewReader([]byte(`"not-an-object"`)))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		attachCSRF(req, client)
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Contains(t, []int{http.StatusBadRequest, http.StatusUnprocessableEntity}, resp.StatusCode)
	})
}

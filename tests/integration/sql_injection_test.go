package integration

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSQLInjection_SecurityHardening tests sending aggressive SQL injection payloads
// across multiple input vectors to ensure queries are strictly parameterized,
// preventing database corruption, information leakage, and unhandled server crashes.
func TestSQLInjection_SecurityHardening(t *testing.T) {
	skipIfServicesDown(t)

	pool := testPool(t)
	ctx := context.Background()

	client := newClient()
	legitEmail := uniqueEmail("sqli_victim")
	legitPass := "LegitPass123!"

	// Setup clean tenant account
	regResp, body := post(t, client, gatewayURL+"/auth/signup", map[string]any{
		"account_name": "SQLi Target Co",
		"email":        legitEmail,
		"password":     legitPass,
	})
	require.Equal(t, http.StatusCreated, regResp.StatusCode)
	accountIDStr := body["account_id"].(string)

	t.Cleanup(func() {
		pool.Exec(ctx, `DELETE FROM sessions WHERE data::text LIKE '%'||$1||'%'`, accountIDStr)
		pool.Exec(ctx, `DELETE FROM messages WHERE account_id = $1`, accountIDStr)
		pool.Exec(ctx, `DELETE FROM conversations WHERE account_id = $1`, accountIDStr)
		pool.Exec(ctx, `DELETE FROM users WHERE account_id = $1`, accountIDStr)
		pool.Exec(ctx, `DELETE FROM accounts WHERE id = $1`, accountIDStr)
	})

	// Login to get valid session
	loginResp, _ := post(t, client, gatewayURL+"/auth/login", map[string]any{
		"email":    legitEmail,
		"password": legitPass,
	})
	require.Equal(t, http.StatusOK, loginResp.StatusCode)

	sqlInjectionPayloads := []string{
		"' OR 1=1 --",
		"'; DROP TABLE messages; --",
		"admin'--",
		"' UNION SELECT null, null, null, null --",
		"1' AND 1=(SELECT 1 FROM pg_sleep(2)) --",
		`" OR ""="`,
		`\'; DROP TABLE accounts; --`,
		`' OR '1'='1' /*`,
	}

	// 1. Test Login Authentication bypass attempt
	t.Run("Auth Login SQLi Bypass", func(t *testing.T) {
		for _, payload := range sqlInjectionPayloads {
			anonClient := newClient()
			resp, _ := post(t, anonClient, gatewayURL+"/auth/login", map[string]any{
				"identifier": payload,
				"password":   payload,
			})
			assert.NotEqual(t, http.StatusInternalServerError, resp.StatusCode, "Login with SQLi must not cause 500 panic")
			assert.Contains(t, []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusUnprocessableEntity}, resp.StatusCode,
				"Login with SQLi payload %q must be rejected safely", payload)
		}
	})

	// 2. Test Signup Account and Email Fields
	t.Run("Auth Signup SQLi Sanitization", func(t *testing.T) {
		for _, payload := range sqlInjectionPayloads {
			anonClient := newClient()
			resp, _ := post(t, anonClient, gatewayURL+"/auth/signup", map[string]any{
				"account_name": payload,
				"email":        fmt.Sprintf("sqli_%s@test.com", uuid.NewString()[:8]),
				"password":     payload,
			})
			assert.NotEqual(t, http.StatusInternalServerError, resp.StatusCode)
		}
	})

	// 3. Test Search / Query Parameters in Conversations
	t.Run("Conversations Query Param SQLi", func(t *testing.T) {
		for _, payload := range sqlInjectionPayloads {
			escapedQuery := url.QueryEscape(payload)
			searchURL := fmt.Sprintf("%s/conversations?search=%s", gatewayURL, escapedQuery)

			resp, _ := get(t, client, searchURL)
			assert.NotEqual(t, http.StatusInternalServerError, resp.StatusCode,
				"Search with SQLi payload %q must not trigger 500 error", payload)
		}
	})

	// 4. Test User Invitation / Creation Fields
	t.Run("Workspace User Creation SQLi", func(t *testing.T) {
		for _, payload := range sqlInjectionPayloads {
			resp, _ := post(t, client, gatewayURL+"/workspace/users", map[string]any{
				"username": payload,
				"password": "Password123!",
				"role":     "agent",
			})
			assert.NotEqual(t, http.StatusInternalServerError, resp.StatusCode)
		}
	})

	// 5. Verify Critical Tables Intact
	t.Run("Verify Database Integrity and Zero Table Drops", func(t *testing.T) {
		criticalTables := []string{"messages", "conversations", "users", "accounts", "channels"}
		for _, table := range criticalTables {
			var count int
			err := pool.QueryRow(ctx, `
				SELECT count(*) FROM information_schema.tables 
				WHERE table_schema = 'public' AND table_name = $1;
			`, table).Scan(&count)
			require.NoError(t, err)
			assert.Equal(t, 1, count, "Table %s must remain intact after SQL injection attempts", table)
		}
	})
}

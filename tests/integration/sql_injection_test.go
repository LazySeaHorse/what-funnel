package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

// seedConversation inserts a contact and open conversation for the account and returns its ID.
func seedConversation(t *testing.T, pool *pgxpool.Pool, accountID uuid.UUID, channelID string) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	contactID, convoID := uuid.New(), uuid.New()
	_, err := pool.Exec(ctx, `
		INSERT INTO contacts (id, account_id, channel_id, external_identity, display_name)
		VALUES ($1, $2, $3, '+15550001111', 'SQLi Contact')`, contactID, accountID, channelID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `
		INSERT INTO conversations (id, account_id, channel_id, contact_id, status, external_thread_id)
		VALUES ($1, $2, $3, $4, 'open', '+15550001111')`, convoID, accountID, channelID, contactID)
	require.NoError(t, err)
	return convoID
}

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

	// Every account created below (including the ones the signup payloads create)
	// is removed afterwards; all account-owned tables cascade.
	createdEmails := []string{legitEmail}
	t.Cleanup(func() {
		for _, email := range createdEmails {
			cleanupAccountByEmail(t, email)
		}
	})
	accountID := uuid.MustParse(accountIDStr)
	channelID := createTestProviderChannel(t, pool, accountID, "SQLi Channel")
	convoID := seedConversation(t, pool, accountID, channelID)

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
			email := fmt.Sprintf("sqli_%s@test.com", uuid.NewString()[:8])
			createdEmails = append(createdEmails, email)
			resp, _ := post(t, anonClient, gatewayURL+"/auth/signup", map[string]any{
				"account_name": payload,
				"email":        email,
				"password":     payload,
			})
			assert.NotEqual(t, http.StatusInternalServerError, resp.StatusCode)
		}
	})

	// 3. Query parameters the conversation handlers actually read: `state` (lead state
	// filter, reaches SQL), `filter` (allow-listed), and the `before` message cursor.
	// A payload that were interpolated into SQL would either error (500), return rows
	// it should not (`OR 1=1`) or stall the request (pg_sleep).
	const sleepBudget = 1500 * time.Millisecond // pg_sleep payloads sleep 2s if they execute
	t.Run("Conversations state filter SQLi", func(t *testing.T) {
		for _, payload := range sqlInjectionPayloads {
			start := time.Now()
			resp, err := client.Get(gatewayURL + "/conversations?state=" + url.QueryEscape(payload))
			require.NoError(t, err)
			elapsed := time.Since(start)
			var list []any
			decodeErr := json.NewDecoder(resp.Body).Decode(&list)
			resp.Body.Close()

			assert.Equal(t, http.StatusOK, resp.StatusCode, "state=%q must be treated as an (unmatched) literal", payload)
			require.NoError(t, decodeErr, "state=%q must return a JSON array", payload)
			assert.Empty(t, list, "state=%q must match no leads (a non-empty list means the payload altered the query)", payload)
			assert.Less(t, elapsed, sleepBudget, "state=%q must not execute pg_sleep", payload)
		}
		// Control: the seeded conversation is listed with no filter, proving an empty result above is meaningful.
		resp, err := client.Get(gatewayURL + "/conversations")
		require.NoError(t, err)
		defer resp.Body.Close()
		var all []map[string]any
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&all))
		var found bool
		for _, c := range all {
			found = found || c["id"] == convoID.String()
		}
		assert.True(t, found, "seeded conversation must be listed without a filter")
	})

	t.Run("Conversations filter allow-list", func(t *testing.T) {
		for _, payload := range sqlInjectionPayloads {
			resp, err := client.Get(gatewayURL + "/conversations?filter=" + url.QueryEscape(payload))
			require.NoError(t, err)
			resp.Body.Close()
			assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "filter=%q must be rejected by the allow-list", payload)
		}
	})

	t.Run("Message cursor SQLi", func(t *testing.T) {
		for _, payload := range sqlInjectionPayloads {
			start := time.Now()
			resp, err := client.Get(fmt.Sprintf("%s/conversations/%s/messages?before=%s", gatewayURL, convoID, url.QueryEscape(payload)))
			require.NoError(t, err)
			resp.Body.Close()
			assert.Less(t, resp.StatusCode, http.StatusInternalServerError, "before=%q must not cause a server error", payload)
			assert.Less(t, time.Since(start), sleepBudget, "before=%q must not execute pg_sleep", payload)
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

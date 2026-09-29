package integration

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/whatfunnel/whatfunnel/packages/go-common/messaging"
	"github.com/whatfunnel/whatfunnel/packages/go-common/pubsub"
)

func TestChatbotOnlyE2E(t *testing.T) {
	skipIfServicesDown(t)
	pool := testPool(t)
	ctx := context.Background()

	adminEmail := uniqueEmail("chatbot-admin")
	adminClient := newClient()

	// 1. Sign up Admin with chatbot_only mode
	t.Log("E2E Step 1: Sign up Admin in chatbot_only mode")
	resp, body := post(t, adminClient, gatewayURL+"/auth/signup", map[string]any{
		"account_name": "E2E Chatbot Only Account",
		"email":        adminEmail,
		"password":     "AdminPassword123!",
		"product_mode": "chatbot_only",
	})
	require.Equal(t, http.StatusCreated, resp.StatusCode, "admin signup must return 201: %v", body)
	accountIDStr := body["account_id"].(string)
	accountID := uuid.MustParse(accountIDStr)

	// Clean up database records after test runs
	t.Cleanup(func() {
		pool.Exec(ctx, `DELETE FROM sessions WHERE data::text LIKE '%'||$1||'%'`, accountIDStr)
		pool.Exec(ctx, `DELETE FROM messages WHERE account_id = $1`, accountID)
		pool.Exec(ctx, `DELETE FROM conversations WHERE account_id = $1`, accountID)
		pool.Exec(ctx, `DELETE FROM contacts WHERE account_id = $1`, accountID)
		pool.Exec(ctx, `DELETE FROM channels WHERE account_id = $1`, accountID)
		pool.Exec(ctx, `DELETE FROM audit_logs WHERE account_id = $1`, accountID)
		pool.Exec(ctx, `DELETE FROM users WHERE account_id = $1`, accountID)
		pool.Exec(ctx, `DELETE FROM accounts WHERE id = $1`, accountID)
	})

	// Log in Admin
	t.Log("E2E Step 2: Log in Admin")
	loginResp, _ := post(t, adminClient, gatewayURL+"/auth/login", map[string]string{
		"email":    adminEmail,
		"password": "AdminPassword123!",
	})
	require.Equal(t, http.StatusOK, loginResp.StatusCode, "login must succeed")

	// 2. Verify account is chatbot_only and lead_tracking_enabled is false
	t.Log("E2E Step 3: Get workspace account details")
	accResp, accBody := get(t, adminClient, gatewayURL+"/workspace/account")
	require.Equal(t, http.StatusOK, accResp.StatusCode)
	assert.Equal(t, "chatbot_only", accBody["product_mode"])

	// Check settings json: lead_tracking_enabled must be false
	settings := extractSettings(t, accBody["settings"])
	assert.Equal(t, false, settings["lead_tracking_enabled"], "lead tracking must be false in chatbot_only mode")

	// 3. Verify lead/pipeline endpoints are gated with 403 Forbidden
	t.Log("E2E Step 4: Verify RBAC gating on lead endpoints")
	leadResp, _ := get(t, adminClient, gatewayURL+"/leads/00000000-0000-0000-0000-000000000000/notes")
	assert.Equal(t, http.StatusForbidden, leadResp.StatusCode, "GET /leads/{id}/notes must return 403 in chatbot_only mode")

	// 4. Update product mode to full_workspace
	t.Log("E2E Step 5: Update product mode to full_workspace")
	patchResp, patchBody := patch(t, adminClient, gatewayURL+"/workspace/account/product-mode", map[string]string{
		"product_mode": "full_workspace",
	})
	require.Equal(t, http.StatusOK, patchResp.StatusCode, "patch product mode should succeed: %v", patchBody)

	// Verify that lead_tracking_enabled is now true
	accResp2, accBody2 := get(t, adminClient, gatewayURL+"/workspace/account")
	require.Equal(t, http.StatusOK, accResp2.StatusCode)
	assert.Equal(t, "full_workspace", accBody2["product_mode"])

	settings2 := extractSettings(t, accBody2["settings"])
	assert.Equal(t, true, settings2["lead_tracking_enabled"], "lead tracking must be true after switching to full_workspace")

	// Switch back to chatbot_only to test external outbound replies and takeover in chatbot_only mode
	t.Log("E2E Step 6: Switch back to chatbot_only mode")
	patchResp2, _ := patch(t, adminClient, gatewayURL+"/workspace/account/product-mode", map[string]string{
		"product_mode": "chatbot_only",
	})
	require.Equal(t, http.StatusOK, patchResp2.StatusCode)

	// Seed a normalized provider channel; live pairing is tested manually.
	t.Log("E2E Step 7: Create WhatsApp test channel")
	channelIDStr := createTestProviderChannel(t, pool, accountID, "Chatbot test")

	// Inbound message to create contact & conversation
	redisAddr := "localhost:6379"
	ps, err := pubsub.NewClient(redisAddr)
	require.NoError(t, err)
	defer ps.Close()

	t.Log("E2E Step 8: Inbound message to create conversation")
	publishTestProviderMessage(t, ps, channelIDStr, "whatsapp-bot-1", "whatsapp-bot-1",
		"Customer Bob", "Hello, is this bot active?", "msg-inbound-bot-1",
		messaging.DirectionInbound)

	// Wait for ingestion to create the conversation and its AI state, then verify
	// it is not under human control. The answer worker may already have moved an
	// unconfigured workspace to review.
	var convoID uuid.UUID
	var aiState string
	require.Eventually(t, func() bool {
		return pool.QueryRow(ctx, `
			SELECT c.id, ais.state
			FROM conversations c
			JOIN conversation_ai_state ais ON ais.conversation_id = c.id
			JOIN contacts co ON c.contact_id = co.id
			WHERE c.channel_id = $1 AND co.external_identity = 'whatsapp-bot-1'
		`, uuid.MustParse(channelIDStr)).Scan(&convoID, &aiState) == nil
	}, 10*time.Second, 100*time.Millisecond, "inbound message must create a conversation with AI state")
	assert.NotEqual(t, "paused_human", aiState, "AI control must not start under human ownership")

	// 5. Ingest External Outbound Event (business owner replies from their phone)
	t.Log("E2E Step 9: Ingest external outbound reply")
	publishTestProviderMessage(t, ps, channelIDStr, "whatsapp-bot-1", "self",
		"", "I am taking over from my phone", "msg-external-bot-1",
		messaging.DirectionOutbound)

	// Verify takeover paused the durable AI control state.
	require.Eventually(t, func() bool {
		if err := pool.QueryRow(ctx, `SELECT state FROM conversation_ai_state WHERE conversation_id = $1`, convoID).Scan(&aiState); err != nil {
			return false
		}
		return aiState == "paused_human"
	}, 10*time.Second, 100*time.Millisecond, "AI must pause after external outbound reply (last state %q)", aiState)

	// Verify the external outbound message is persisted in DB
	var direction, senderType string
	var externalMsgID *string
	var contentRaw []byte
	err = pool.QueryRow(ctx, `
		SELECT direction, sender_type, external_message_id, content
		FROM messages
		WHERE conversation_id = $1 AND external_message_id = 'msg-external-bot-1'
	`, convoID).Scan(&direction, &senderType, &externalMsgID, &contentRaw)
	require.NoError(t, err)

	assert.Equal(t, "outbound", direction)
	assert.Equal(t, "human", senderType)
	require.NotNil(t, externalMsgID)
	assert.Equal(t, "msg-external-bot-1", *externalMsgID)

	var content map[string]any
	err = json.Unmarshal(contentRaw, &content)
	require.NoError(t, err)
	assert.Equal(t, "I am taking over from my phone", content["text"])
}

// Helper to base64 decode (mocking JS atob)
func atob(s string) (string, error) {
	decoded, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return "", err
	}
	return string(decoded), nil
}

func extractSettings(t *testing.T, raw any) map[string]any {
	switch v := raw.(type) {
	case map[string]any:
		return v
	case string:
		decoded, err := atob(v)
		require.NoError(t, err)
		var settings map[string]any
		err = json.Unmarshal([]byte(decoded), &settings)
		require.NoError(t, err)
		return settings
	default:
		t.Fatalf("unexpected settings type: %T", raw)
		return nil
	}
}

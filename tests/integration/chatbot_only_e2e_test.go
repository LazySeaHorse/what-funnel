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
	var leadTrackingEnabled bool
	if settingsStr, ok := accBody["settings"].(string); ok {
		decoded, err := atob(settingsStr)
		require.NoError(t, err)
		var settings map[string]any
		err = json.Unmarshal([]byte(decoded), &settings)
		require.NoError(t, err)
		if enabled, exists := settings["lead_tracking_enabled"]; exists {
			leadTrackingEnabled = enabled.(bool)
		}
	}
	assert.False(t, leadTrackingEnabled, "lead tracking must be false in chatbot_only mode")

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

	var leadTrackingEnabled2 bool
	if settingsStr, ok := accBody2["settings"].(string); ok {
		decoded, err := atob(settingsStr)
		require.NoError(t, err)
		var settings map[string]any
		err = json.Unmarshal([]byte(decoded), &settings)
		require.NoError(t, err)
		if enabled, exists := settings["lead_tracking_enabled"]; exists {
			leadTrackingEnabled2 = enabled.(bool)
		}
	}
	assert.True(t, leadTrackingEnabled2, "lead tracking must be true after switching to full_workspace")

	// Switch back to chatbot_only to test external outbound replies and takeover in chatbot_only mode
	t.Log("E2E Step 6: Switch back to chatbot_only mode")
	patchResp2, _ := patch(t, adminClient, gatewayURL+"/workspace/account/product-mode", map[string]string{
		"product_mode": "chatbot_only",
	})
	require.Equal(t, http.StatusOK, patchResp2.StatusCode)

	// Create Channel
	t.Log("E2E Step 7: Create WhatsApp Channel")
	chanResp, chanBody := post(t, adminClient, gatewayURL+"/channels", map[string]any{
		"type":            "matrix_whatsapp",
		"bridge_identity": "whatsapp-chatbot-bridge",
		"bridge_credentials": map[string]any{
			"homeserver_url": "mock",
			"user_id":        "@whatsapp_chatbot:localhost",
			"access_token":   "mock-token",
		},
	})
	require.Equal(t, http.StatusCreated, chanResp.StatusCode)
	channelIDStr := chanBody["id"].(string)

	// Inbound message to create contact & conversation
	redisAddr := "localhost:6379"
	ps, err := pubsub.NewClient(redisAddr)
	require.NoError(t, err)
	defer ps.Close()

	t.Log("E2E Step 8: Inbound message to create conversation")
	inboundMsg := map[string]any{
		"ChannelID":        channelIDStr,
		"ExternalThreadID": "whatsapp-bot-1",
		"Contact": map[string]any{
			"ExternalIdentity": "whatsapp-bot-1",
			"DisplayName":      "Customer Bob",
		},
		"Message": map[string]any{
			"ContentType":       "text",
			"Text":              "Hello, is this bot active?",
			"ExternalMessageID": "msg-inbound-bot-1",
		},
		"Timestamp": time.Now(),
	}
	_, err = ps.Publish(ctx, "messages.inbound", inboundMsg)
	require.NoError(t, err)

	// Wait for ingestion
	time.Sleep(500 * time.Millisecond)

	// Verify conversation exists and has AI mode active (true)
	var convoID uuid.UUID
	var aiModeActive bool
	err = pool.QueryRow(ctx, `
		SELECT c.id, c.ai_mode_active
		FROM conversations c
		JOIN contacts co ON c.contact_id = co.id
		WHERE c.channel_id = $1 AND co.external_identity = 'whatsapp-bot-1'
	`, uuid.MustParse(channelIDStr)).Scan(&convoID, &aiModeActive)
	require.NoError(t, err)
	assert.True(t, aiModeActive, "AI mode must be active initially")

	// 5. Ingest External Outbound Event (business owner replies from their phone)
	t.Log("E2E Step 9: Ingest external outbound reply")
	externalMsg := map[string]any{
		"ChannelID":        channelIDStr,
		"ExternalThreadID": "whatsapp-bot-1",
		"Message": map[string]any{
			"ContentType": "text",
			"Text":        "I am taking over from my phone",
		},
		"ExternalMessageID": "msg-external-bot-1",
		"Timestamp":         time.Now(),
	}
	_, err = ps.Publish(ctx, "messages.external_outbound", externalMsg)
	require.NoError(t, err)

	// Wait for ingestion
	time.Sleep(500 * time.Millisecond)

	// Verify AI mode is now false (takeover pause triggered!)
	err = pool.QueryRow(ctx, `SELECT ai_mode_active FROM conversations WHERE id = $1`, convoID).Scan(&aiModeActive)
	require.NoError(t, err)
	assert.False(t, aiModeActive, "AI mode must be deactivated (takeover paused) after external outbound reply")

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
	assert.Equal(t, true, content["external_origin"])
}

// Helper to base64 decode (mocking JS atob)
func atob(s string) (string, error) {
	decoded, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return "", err
	}
	return string(decoded), nil
}

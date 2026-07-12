package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/whatfunnel/whatfunnel/packages/go-common/pubsub"
)

func TestAIAnswerE2E(t *testing.T) {
	skipIfServicesDown(t)
	pool := testPool(t)
	ctx := context.Background()

	adminEmail := uniqueEmail("ai-admin")
	adminClient := newClient()

	// 1. Sign up admin
	t.Log("AI Answer E2E Step 1: Sign up Admin")
	resp, body := post(t, adminClient, gatewayURL+"/auth/signup", map[string]string{
		"account_name": "E2E AI Account",
		"email":        adminEmail,
		"password":     "AdminPassword123!",
	})
	require.Equal(t, http.StatusCreated, resp.StatusCode, "admin signup must return 201: %v", body)
	accountIDStr := body["account_id"].(string)
	accountID := uuid.MustParse(accountIDStr)
	adminUserIDStr := body["user_id"].(string)

	// Clean up database records after test runs
	t.Cleanup(func() {
		pool.Exec(ctx, `DELETE FROM sessions WHERE data::text LIKE '%'||$1||'%'`, accountIDStr)
		pool.Exec(ctx, `DELETE FROM invite_tokens WHERE account_id = $1`, accountID)
		pool.Exec(ctx, `DELETE FROM messages WHERE account_id = $1`, accountID)
		pool.Exec(ctx, `DELETE FROM conversations WHERE account_id = $1`, accountID)
		pool.Exec(ctx, `DELETE FROM contacts WHERE account_id = $1`, accountID)
		pool.Exec(ctx, `DELETE FROM channels WHERE account_id = $1`, accountID)
		pool.Exec(ctx, `DELETE FROM patterns WHERE account_id = $1`, accountID)
		pool.Exec(ctx, `DELETE FROM ai_answer_events WHERE account_id = $1`, accountID)
		pool.Exec(ctx, `DELETE FROM conversation_summaries WHERE account_id = $1`, accountID)
		pool.Exec(ctx, `DELETE FROM audit_logs WHERE account_id = $1`, accountID)
		pool.Exec(ctx, `DELETE FROM lead_pipelines WHERE account_id = $1`, accountID)
		pool.Exec(ctx, `DELETE FROM users WHERE account_id = $1`, accountID)
		pool.Exec(ctx, `DELETE FROM accounts WHERE id = $1`, accountID)
	})

	// Log in Admin
	t.Log("AI Answer E2E Step 2: Log in Admin")
	loginResp, _ := post(t, adminClient, gatewayURL+"/auth/login", map[string]string{
		"email":    adminEmail,
		"password": "AdminPassword123!",
	})
	require.Equal(t, http.StatusOK, loginResp.StatusCode, "login must succeed")

	// 2. Create WhatsApp Channel
	t.Log("AI Answer E2E Step 3: Create WhatsApp Channel")
	chanResp, chanBody := post(t, adminClient, gatewayURL+"/channels", map[string]any{
		"type":            "matrix_whatsapp",
		"bridge_identity": "whatsapp-bridge-user-ai",
		"bridge_credentials": map[string]any{
			"homeserver_url": "mock",
			"user_id":        "@whatsapp-ai:localhost",
			"access_token":   "mock-token-ai",
		},
	})
	require.Equal(t, http.StatusCreated, chanResp.StatusCode, "create channel must return 201: %v", chanBody)
	channelIDStr := chanBody["id"].(string)

	// 3. Connect Admin WebSocket
	t.Log("AI Answer E2E Step 4: Connect Admin WebSocket")
	u, _ := url.Parse(gatewayURL)
	cookies := adminClient.Jar.Cookies(u)
	header := http.Header{}
	for _, cookie := range cookies {
		header.Add("Cookie", cookie.String())
	}
	wsURL := "ws://localhost:18080/ws"
	adminWS, wsResp, err := websocket.DefaultDialer.Dial(wsURL, header)
	require.NoError(t, err, "admin websocket dial failed: %v", wsResp)
	defer adminWS.Close()

	// 4. Create a trigger phrase pattern in database
	patternID := uuid.New()
	_, err = pool.Exec(ctx, `
		INSERT INTO patterns (id, account_id, trigger_phrases, answer_markdown, canonical_question, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
	`, patternID, accountID, []string{"Do you offer house calls?"}, "Yes, we offer house calls.", "Do you offer house calls?")
	require.NoError(t, err)

	// 5. Simulate inbound message
	t.Log("AI Answer E2E Step 5: Simulate inbound WhatsApp message matching pattern")
	ps, err := pubsub.NewClient("localhost:6379")
	require.NoError(t, err)
	defer ps.Close()

	externalThreadID := "whatsapp-jid-ai-999"
	inboundMsg := map[string]any{
		"ChannelID":        channelIDStr,
		"ExternalThreadID": externalThreadID,
		"Contact": map[string]any{
			"ExternalIdentity": externalThreadID,
			"DisplayName":      "Bob Inbound",
			"AvatarURL":        "http://bob-avatar",
		},
		"Message": map[string]any{
			"ContentType":       "text",
			"Text":              "Do you offer house calls?",
			"MediaURL":          "",
			"ReplyToExternalID": "",
			"ExternalMessageID": "external-msg-ai-111",
		},
		"Timestamp": time.Now().Format(time.RFC3339),
	}

	_, err = ps.Publish(ctx, "messages.inbound", inboundMsg)
	require.NoError(t, err)

	// 6. Expect WebSocket notification of drafted reply
	t.Log("AI Answer E2E Step 6: Expect WebSocket notification for drafted reply")
	var draftWSMsg struct {
		Type           string `json:"type"`
		ConversationID string `json:"conversation_id"`
		Action         string `json:"action"`
		DraftText      string `json:"draft_text"`
		MessageID      string `json:"message_id"`
	}

	var convoIDStr string
	doneWS := make(chan struct{})
	go func() {
		defer close(doneWS)
		for {
			_, p, err := adminWS.ReadMessage()
			if err != nil {
				return
			}
			var ev struct {
				Type           string `json:"type"`
				ConversationID string `json:"conversation_id"`
				Action         string `json:"action"`
				DraftText      string `json:"draft_text"`
				MessageID      string `json:"message_id"`
			}
			_ = json.Unmarshal(p, &ev)
			if ev.Type == "ai.reply_ready" {
				draftWSMsg = ev
				return
			} else if ev.Type == "message.received" {
				convoIDStr = ev.ConversationID
			}
		}
	}()

	select {
	case <-doneWS:
		assert.Equal(t, "ai.reply_ready", draftWSMsg.Type)
		assert.Equal(t, "drafted", draftWSMsg.Action)
		assert.Equal(t, "Yes, we offer house calls.", draftWSMsg.DraftText)
	case <-time.After(10 * time.Second):
		t.Fatal("Timeout waiting for drafted ai.reply_ready websocket event")
	}

	require.NotEmpty(t, convoIDStr)
	convoID := uuid.MustParse(convoIDStr)

	// Assign first to enable overrides
	t.Log("AI Answer E2E Step 7: Assign Admin user to conversation")
	assignResp, assignBody := patch(t, adminClient, gatewayURL+"/conversations/"+convoIDStr+"/assign", map[string]any{
		"user_ids": []string{adminUserIDStr},
	})
	require.Equal(t, http.StatusOK, assignResp.StatusCode, "assign must succeed: %v", assignBody)

	// 7. Update User Reply Mode Override to auto_send
	t.Log("AI Answer E2E Step 8: Update user reply-mode override to auto_send")
	patchResp, patchBody := patch(t, adminClient, gatewayURL+"/users/me/reply-mode", map[string]any{
		"reply_mode": "auto_send",
	})
	require.Equal(t, http.StatusOK, patchResp.StatusCode, "update reply mode must succeed: %v", patchBody)

	// 8. Send second inbound message
	t.Log("AI Answer E2E Step 9: Simulate second inbound message to test auto_send")
	inboundMsg2 := map[string]any{
		"ChannelID":        channelIDStr,
		"ExternalThreadID": externalThreadID,
		"Contact": map[string]any{
			"ExternalIdentity": externalThreadID,
			"DisplayName":      "Bob Inbound",
			"AvatarURL":        "http://bob-avatar",
		},
		"Message": map[string]any{
			"ContentType":       "text",
			"Text":              "Do you offer house calls?",
			"MediaURL":          "",
			"ReplyToExternalID": "",
			"ExternalMessageID": "external-msg-ai-222",
		},
		"Timestamp": time.Now().Format(time.RFC3339),
	}

	// Reconnect admin WS to clear buffer
	adminWS.Close()
	adminWS, _, err = websocket.DefaultDialer.Dial(wsURL, header)
	require.NoError(t, err)
	defer adminWS.Close()

	_, err = ps.Publish(ctx, "messages.inbound", inboundMsg2)
	require.NoError(t, err)

	// Expect WebSocket notification for auto_sent reply
	t.Log("AI Answer E2E Step 10: Expect WebSocket notification for auto_sent reply")
	var autoSentWSMsg struct {
		Type           string `json:"type"`
		ConversationID string `json:"conversation_id"`
		Action         string `json:"action"`
		DraftText      string `json:"draft_text"`
		MessageID      string `json:"message_id"`
	}

	doneWS2 := make(chan struct{})
	go func() {
		defer close(doneWS2)
		for {
			_, p, err := adminWS.ReadMessage()
			if err != nil {
				return
			}
			var ev struct {
				Type           string `json:"type"`
				ConversationID string `json:"conversation_id"`
				Action         string `json:"action"`
				DraftText      string `json:"draft_text"`
				MessageID      string `json:"message_id"`
			}
			_ = json.Unmarshal(p, &ev)
			if ev.Type == "ai.reply_ready" {
				autoSentWSMsg = ev
				return
			}
		}
	}()

	select {
	case <-doneWS2:
		assert.Equal(t, "ai.reply_ready", autoSentWSMsg.Type)
		assert.Equal(t, "auto_sent", autoSentWSMsg.Action)
	case <-time.After(10 * time.Second):
		t.Fatal("Timeout waiting for auto_sent ai.reply_ready websocket event")
	}

	// 9. Verify that an outbound message with sender_type = 'ai' is logged in DB
	var outboundCount int
	err = pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM messages 
		WHERE conversation_id = $1 AND direction = 'outbound' AND sender_type = 'ai'
	`, convoID).Scan(&outboundCount)
	require.NoError(t, err)
	assert.Equal(t, 1, outboundCount, "an outbound message by AI should be stored")

	// 10. Human Takeover: Human replies to conversation
	t.Log("AI Answer E2E Step 11: Human replies to conversation to test auto-pause AI")
	sendResp, sendBody := post(t, adminClient, gatewayURL+"/internal/conversations/"+convoIDStr+"/send", map[string]any{
		"content_type":   "text",
		"text":           "Hello from human!",
		"sender_type":    "human",
		"sender_user_id": adminUserIDStr,
	})
	require.Equal(t, http.StatusOK, sendResp.StatusCode, "human send must succeed: %v", sendBody)

	// Verify that ai_mode_active is now false in DB
	var aiModeActive bool
	err = pool.QueryRow(ctx, `SELECT ai_mode_active FROM conversations WHERE id = $1`, convoID).Scan(&aiModeActive)
	require.NoError(t, err)
	assert.False(t, aiModeActive, "AI mode active should be paused (false) on human reply")

	// 11. Close Conversation to trigger AI-mode resumption and structured summary generation
	t.Log("AI Answer E2E Step 12: Close conversation to trigger summary and AI resumption")
	closeResp, closeBody := post(t, adminClient, gatewayURL+"/conversations/"+convoIDStr+"/close", nil)
	require.Equal(t, http.StatusOK, closeResp.StatusCode, "close must succeed: %v", closeBody)

	// Wait up to 5 seconds to let python background worker process the close event
	time.Sleep(3 * time.Second)

	// Verify that ai_mode_active flips back to true
	err = pool.QueryRow(ctx, `SELECT ai_mode_active FROM conversations WHERE id = $1`, convoID).Scan(&aiModeActive)
	require.NoError(t, err)
	assert.True(t, aiModeActive, "AI mode should resume (true) after close and idle debounce")
}

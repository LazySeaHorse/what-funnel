package integration

import (
	"bytes"
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

func TestInboxE2E(t *testing.T) {
	skipIfServicesDown(t)
	pool := testPool(t)
	ctx := context.Background()

	adminEmail := uniqueEmail("inbox-admin")
	adminClient := newClient()

	// 1. Sign up admin
	t.Log("E2E Step 1: Sign up Admin")
	resp, body := post(t, adminClient, gatewayURL+"/auth/signup", map[string]string{
		"account_name": "E2E Inbox Account",
		"email":        adminEmail,
		"password":     "AdminPassword123!",
	})
	require.Equal(t, http.StatusCreated, resp.StatusCode, "admin signup must return 201: %v", body)
	accountIDStr := body["account_id"].(string)
	accountID := uuid.MustParse(accountIDStr)

	// Clean up database records after test runs
	t.Cleanup(func() {
		pool.Exec(ctx, `DELETE FROM sessions WHERE data::text LIKE '%'||$1||'%'`, accountIDStr)
		pool.Exec(ctx, `DELETE FROM invite_tokens WHERE account_id = $1`, accountID)
		pool.Exec(ctx, `DELETE FROM messages WHERE account_id = $1`, accountID)
		pool.Exec(ctx, `DELETE FROM conversations WHERE account_id = $1`, accountID)
		pool.Exec(ctx, `DELETE FROM contacts WHERE account_id = $1`, accountID)
		pool.Exec(ctx, `DELETE FROM channels WHERE account_id = $1`, accountID)
		pool.Exec(ctx, `DELETE FROM audit_logs WHERE account_id = $1`, accountID)
		pool.Exec(ctx, `DELETE FROM lead_pipelines WHERE account_id = $1`, accountID)
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

	// 2. Create WhatsApp Channel (with HomeserverURL = "mock")
	t.Log("E2E Step 3: Create WhatsApp Channel with mock credentials")
	chanResp, chanBody := post(t, adminClient, gatewayURL+"/channels", map[string]any{
		"type":            "matrix_whatsapp",
		"bridge_identity": "whatsapp-bridge-user",
		"bridge_credentials": map[string]any{
			"homeserver_url": "mock",
			"user_id":        "@whatsapp:localhost",
			"access_token":   "mock-token",
		},
	})
	require.Equal(t, http.StatusCreated, chanResp.StatusCode, "create channel must return 201: %v", chanBody)
	channelIDStr := chanBody["id"].(string)

	// 3. Connect Admin WebSocket
	t.Log("E2E Step 4: Connect Admin WebSocket")
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

	// 4. Invite Member
	t.Log("E2E Step 5: Invite Member")
	memberEmail := uniqueEmail("inbox-member")
	invResp, invBody := post(t, adminClient, gatewayURL+"/workspace/users/invite", map[string]string{
		"email": memberEmail,
		"role":  "member",
	})
	require.Equal(t, http.StatusCreated, invResp.StatusCode, "invite must succeed: %v", invBody)
	inviteToken := invBody["invite_token"].(string)

	// 5. Member signup using invite token
	t.Log("E2E Step 6: Sign up Member with invite token")
	memberClient := newClient()
	signupResp, signupBody := post(t, memberClient, gatewayURL+"/auth/signup", map[string]string{
		"account_name": "Invited User",
		"email":        memberEmail,
		"password":     "MemberPassword123!",
		"invite_token": inviteToken,
	})
	require.Equal(t, http.StatusCreated, signupResp.StatusCode, "member signup must succeed: %v", signupBody)

	// Member Login
	t.Log("E2E Step 7: Log in Member")
	memLoginResp, _ := post(t, memberClient, gatewayURL+"/auth/login", map[string]string{
		"email":    memberEmail,
		"password": "MemberPassword123!",
	})
	require.Equal(t, http.StatusOK, memLoginResp.StatusCode)

	// Member WebSocket Connect
	t.Log("E2E Step 8: Connect Member WebSocket")
	memCookies := memberClient.Jar.Cookies(u)
	memHeader := http.Header{}
	for _, cookie := range memCookies {
		memHeader.Add("Cookie", cookie.String())
	}
	memberWS, _, err := websocket.DefaultDialer.Dial(wsURL, memHeader)
	require.NoError(t, err)
	defer memberWS.Close()

	// 6. TOGGLE VISIBILITY SETTINGS TO PRIVATE FIRST
	t.Log("E2E Step 9: Toggle visibility setting to unassigned private")
	putResp, _ := put(t, adminClient, gatewayURL+"/workspace/account/settings", map[string]any{
		"unassigned_conversations_visible_to_members": false,
	})
	require.Equal(t, http.StatusOK, putResp.StatusCode)

	// 7. Simulate inbound message via Redis Stream
	t.Log("E2E Step 10: Simulate inbound WhatsApp message")
	ps, err := pubsub.NewClient("localhost:6379")
	require.NoError(t, err)
	defer ps.Close()

	externalThreadID := "whatsapp-jid-5678"
	inboundMsg := map[string]any{
		"ChannelID":        channelIDStr,
		"ExternalThreadID": externalThreadID,
		"Contact": map[string]any{
			"ExternalIdentity": externalThreadID,
			"DisplayName":      "Alice Inbound",
			"AvatarURL":        "http://alice-avatar",
		},
		"Message": map[string]any{
			"ContentType":       "text",
			"Text":              "Hello from WhatsApp!",
			"MediaURL":          "",
			"ReplyToExternalID": "",
			"ExternalMessageID": "external-msg-xyz",
		},
		"Timestamp": time.Now().Format(time.RFC3339),
	}

	_, err = ps.Publish(ctx, "messages.inbound", inboundMsg)
	require.NoError(t, err)

	// 8. Verify Admin WebSocket receives message.received event
	t.Log("E2E Step 11: Verify Admin WS receives event")
	var adminWSEvent struct {
		Type           string `json:"type"`
		ConversationID string `json:"conversation_id"`
	}

	doneAdmin := make(chan struct{})
	go func() {
		defer close(doneAdmin)
		_, p, err := adminWS.ReadMessage()
		if err == nil {
			_ = json.Unmarshal(p, &adminWSEvent)
		}
	}()

	select {
	case <-doneAdmin:
		assert.Equal(t, "message.received", adminWSEvent.Type)
		assert.NotEmpty(t, adminWSEvent.ConversationID)
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for admin websocket event")
	}

	// 9. Verify Member CANNOT see conversation via REST when settings are private
	t.Log("E2E Step 12: Verify member gets no private conversation leaks (REST visibility check)")
	listResp, listBody := get(t, memberClient, gatewayURL+"/conversations?filter=all")
	require.Equal(t, http.StatusOK, listResp.StatusCode)
	convoArray, ok := listBody["conversations"].([]any)
	if ok {
		assert.Equal(t, 0, len(convoArray), "Member must not see unassigned conversations when visibility is disabled")
	} else {
		// If return format is different or null
		assert.Nil(t, listBody["conversations"])
	}

	// 10. Admin assigns conversation to member
	t.Log("E2E Step 13: Admin assigns conversation to Member")
	memberUserIDStr := signupBody["user_id"].(string)
	convoIDStr := adminWSEvent.ConversationID

	assignResp, assignBody := patch(t, adminClient, gatewayURL+"/conversations/"+convoIDStr+"/assign", map[string]any{
		"user_ids": []string{memberUserIDStr},
	})
	require.Equal(t, http.StatusOK, assignResp.StatusCode, "assignment failed: %v", assignBody)

	// 11. Verify Member WS receives the assignment event
	t.Log("E2E Step 14: Verify Member WS receives assignment event")
	var memberWSEvent struct {
		Type           string   `json:"type"`
		ConversationID string   `json:"conversation_id"`
		UserIDs        []string `json:"assigned_user_ids"`
	}

	doneMemberAssign := make(chan struct{})
	go func() {
		defer close(doneMemberAssign)
		_, p, err := memberWS.ReadMessage()
		if err == nil {
			_ = json.Unmarshal(p, &memberWSEvent)
		}
	}()

	select {
	case <-doneMemberAssign:
		assert.Equal(t, "conversation.assigned", memberWSEvent.Type)
		assert.Equal(t, convoIDStr, memberWSEvent.ConversationID)
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for member assignment websocket event")
	}

	// 12. Member sends reply to the conversation
	t.Log("E2E Step 15: Member replies to conversation")
	replyResp, replyBody := post(t, memberClient, gatewayURL+"/internal/conversations/"+convoIDStr+"/send", map[string]any{
		"content_type":   "text",
		"text":           "Replied by E2E member!",
		"sender_type":    "human",
		"sender_user_id": memberUserIDStr,
	})
	require.Equal(t, http.StatusOK, replyResp.StatusCode, "outbound send failed: %v", replyBody)

	// 13. Verify outbound message is saved and readable
	t.Log("E2E Step 16: Verify message thread contains member reply")
	msgsResp, msgsBody := get(t, memberClient, gatewayURL+"/conversations/"+convoIDStr+"/messages")
	require.Equal(t, http.StatusOK, msgsResp.StatusCode)
	msgsList, ok := msgsBody["messages"].([]any)
	require.True(t, ok)
	assert.GreaterOrEqual(t, len(msgsList), 2)
}

// helper patch function
func patch(t *testing.T, client *http.Client, url string, body any) (*http.Response, map[string]any) {
	t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPatch, url, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { resp.Body.Close() })
	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result) //nolint:errcheck
	return resp, result
}

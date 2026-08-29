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

func TestLeadManagementE2E(t *testing.T) {
	skipIfServicesDown(t)
	pool := testPool(t)
	ctx := context.Background()

	adminEmail := uniqueEmail("lead-admin")
	adminClient := newClient()

	// 1. Sign up Admin
	t.Log("E2E Step 1: Sign up Admin")
	resp, body := post(t, adminClient, gatewayURL+"/auth/signup", map[string]string{
		"account_name": "E2E Lead Account",
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
		pool.Exec(ctx, `DELETE FROM lead_state_history WHERE account_id = $1`, accountID)
		pool.Exec(ctx, `DELETE FROM lead_notes WHERE account_id = $1`, accountID)
		pool.Exec(ctx, `DELETE FROM leads WHERE account_id = $1`, accountID)
		pool.Exec(ctx, `DELETE FROM lead_pipelines WHERE account_id = $1`, accountID)
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

	// 2. Create WhatsApp Channel
	t.Log("E2E Step 3: Create WhatsApp Channel")
	chanResp, chanBody := post(t, adminClient, gatewayURL+"/channels", map[string]any{
		"type":            "matrix_whatsapp",
		"bridge_identity": "whatsapp-lead-bridge",
		"bridge_credentials": map[string]any{
			"homeserver_url": "mock",
			"user_id":        "@whatsapp_lead:localhost",
			"access_token":   "mock-token",
		},
	})
	require.Equal(t, http.StatusCreated, chanResp.StatusCode)
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

	// 4. Create Agent User directly
	t.Log("E2E Step 5: Create Agent User")
	createResp, createBody := post(t, adminClient, gatewayURL+"/workspace/users", map[string]string{
		"username": "lead_agent",
		"password": "MemberPassword123!",
		"role":     "agent",
	})
	require.Equal(t, http.StatusCreated, createResp.StatusCode, "create user must succeed: %v", createBody)

	// Set workspace slug
	slugResp, _ := put(t, adminClient, gatewayURL+"/workspace/account/slug", map[string]string{
		"slug": "lead-corp",
	})
	require.Equal(t, http.StatusOK, slugResp.StatusCode)

	// 5. Agent Login using slug-username
	t.Log("E2E Step 6: Log in Agent with slug-username")
	memberClient := newClient()
	memLoginResp, _ := post(t, memberClient, gatewayURL+"/auth/login", map[string]string{
		"identifier": "lead-corp-lead_agent",
		"password":   "MemberPassword123!",
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

	// Connect PubSub
	redisAddr := "localhost:6379"
	ps, err := pubsub.NewClient(redisAddr)
	require.NoError(t, err)
	defer ps.Close()

	// 6. Simulate inbound message to auto-create lead (default settings: lead tracking = true, unassigned visible = false)
	t.Log("E2E Step 9: Toggle visibility to private (unassigned conversations hidden from members)")
	putResp, _ := put(t, adminClient, gatewayURL+"/workspace/account/settings", map[string]any{
		"unassigned_conversations_visible_to_members": false,
		"lead_tracking_enabled":                       true,
	})
	require.Equal(t, http.StatusOK, putResp.StatusCode)

	t.Log("E2E Step 10: Ingest inbound message to trigger auto-lead creation")
	inboundMsg := map[string]any{
		"ChannelID":        channelIDStr,
		"ExternalThreadID": "external-thread-e2e-1",
		"Contact": map[string]any{
			"ExternalIdentity": "contacte2e1@s.whatsapp.net",
			"DisplayName":      "E2E Contact 1",
			"AvatarURL":        "",
		},
		"Message": map[string]any{
			"ContentType":       "text",
			"Text":              "Interested in buying",
			"MediaURL":          "",
			"ReplyToExternalID": "",
			"ExternalMessageID": "msg-e2e-1",
		},
		"Timestamp": time.Now().Format(time.RFC3339),
	}
	_, err = ps.Publish(ctx, "messages.inbound", inboundMsg)
	require.NoError(t, err)

	// Wait and verify admin WS receives the message event
	t.Log("E2E Step 11: Wait for WebSocket message broadcast")
	var msgReceived bool
	var convoIDStr string
	for i := 0; i < 20; i++ {
		adminWS.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		_, message, err := adminWS.ReadMessage()
		if err == nil {
			var wsMsg map[string]any
			if json.Unmarshal(message, &wsMsg) == nil {
				if wsMsg["type"] == "message.received" {
					msgReceived = true
					convoIDStr = wsMsg["conversation_id"].(string)
					break
				}
			}
		}
	}
	require.True(t, msgReceived, "Admin WS must receive message.received broadcast")
	convoID := uuid.MustParse(convoIDStr)

	// 7. Get conversation details and verify lead details
	t.Log("E2E Step 12: Verify auto-created lead details via REST API")
	getResp, getBody := get(t, adminClient, gatewayURL+"/conversations/"+convoIDStr)
	require.Equal(t, http.StatusOK, getResp.StatusCode)
	
	leadMap, ok := getBody["lead"].(map[string]any)
	require.True(t, ok, "conversation response must contain lead details")
	assert.Equal(t, "new", leadMap["current_state_key"])
	leadIDStr := leadMap["id"].(string)

	// 8. Lead state transition (PATCH /leads/{id}/state)
	t.Log("E2E Step 13: Transition lead state to 'won'")
	statePatchResp, statePatchBody := patch(t, adminClient, gatewayURL+"/leads/"+leadIDStr+"/state", map[string]any{
		"state_key": "won",
	})
	require.Equal(t, http.StatusOK, statePatchResp.StatusCode)
	assert.Equal(t, "won", statePatchBody["current_state_key"])

	// Verify WebSocket broadcast of lead.state_changed
	t.Log("E2E Step 14: Wait for lead.state_changed WebSocket broadcast")
	var stateChangedReceived bool
	for i := 0; i < 20; i++ {
		adminWS.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		_, message, err := adminWS.ReadMessage()
		if err == nil {
			var wsMsg map[string]any
			if json.Unmarshal(message, &wsMsg) == nil {
				if wsMsg["type"] == "lead.state_changed" {
					stateChangedReceived = true
					assert.Equal(t, "won", wsMsg["to_state"])
					break
				}
			}
		}
	}
	require.True(t, stateChangedReceived, "Admin WS must receive lead.state_changed broadcast")

	// 9. Reject invalid state transition (400 Bad Request)
	t.Log("E2E Step 15: Verify invalid state transition is rejected")
	badStateResp, _ := patch(t, adminClient, gatewayURL+"/leads/"+leadIDStr+"/state", map[string]any{
		"state_key": "invalid-state-key",
	})
	require.Equal(t, http.StatusBadRequest, badStateResp.StatusCode)

	// 10. Pipeline state deletion guard (409 Conflict)
	// Get current pipelines
	t.Log("E2E Step 16: Verify pipeline deletion guard preventing removal of in-use state")
	pipeGetResp, err := adminClient.Get(gatewayURL + "/workspace/pipelines")
	require.NoError(t, err)
	defer pipeGetResp.Body.Close()
	var pipelines []map[string]any
	json.NewDecoder(pipeGetResp.Body).Decode(&pipelines)
	require.NotEmpty(t, pipelines)
	pipelineID := pipelines[0]["id"].(string)

	// Try to update pipeline by removing the 'won' state (which our lead is currently in)
	badStates := []map[string]any{
		{"key": "new", "label": "New", "color": "#6366f1"},
		{"key": "lost", "label": "Lost", "color": "#ef4444"},
	}
	putPipeResp, putPipeBody := put(t, adminClient, gatewayURL+"/workspace/pipelines/"+pipelineID, map[string]any{
		"name":   "Default Pipeline",
		"states": badStates,
	})
	// Should return 409 Conflict because 'won' is in use by our lead
	require.Equal(t, http.StatusConflict, putPipeResp.StatusCode)
	assert.Equal(t, "in_use", putPipeBody["error"])

	// 11. Visibility Enforcement: member cannot view or mutate lead of unassigned private conversation
	t.Log("E2E Step 17: Verify Member visibility enforcement")
	memGetResp, _ := get(t, memberClient, gatewayURL+"/conversations/"+convoIDStr)
	require.Equal(t, http.StatusNotFound, memGetResp.StatusCode, "Member must not see unassigned conversation")

	memStateResp, _ := patch(t, memberClient, gatewayURL+"/leads/"+leadIDStr+"/state", map[string]any{
		"state_key": "lost",
	})
	require.Equal(t, http.StatusNotFound, memStateResp.StatusCode, "Member must receive 404 trying to mutate hidden lead")

	// Assign conversation to Member
	t.Log("E2E Step 18: Assign conversation to member")
	memberUserID := uuid.MustParse(createBody["id"].(string))

	_, err = pool.Exec(ctx, `UPDATE conversations SET assigned_user_ids = $1 WHERE id = $2`, []uuid.UUID{memberUserID}, convoID)
	require.NoError(t, err)

	// Member tries again — should now succeed
	t.Log("E2E Step 19: Verify Member can now access lead after assignment")
	memGetResp2, _ := get(t, memberClient, gatewayURL+"/conversations/"+convoIDStr)
	require.Equal(t, http.StatusOK, memGetResp2.StatusCode)

	memStateResp2, memStateBody2 := patch(t, memberClient, gatewayURL+"/leads/"+leadIDStr+"/state", map[string]any{
		"state_key": "lost",
	})
	require.Equal(t, http.StatusOK, memStateResp2.StatusCode)
	assert.Equal(t, "lost", memStateBody2["current_state_key"])

	// 12. Notes & Tags Operations
	t.Log("E2E Step 20: Test lead tags and append-only notes")
	// Update tags
	tagResp, tagBody := patch(t, memberClient, gatewayURL+"/leads/"+leadIDStr+"/tags", map[string]any{
		"tags": []string{"urgent", "high-value"},
	})
	require.Equal(t, http.StatusOK, tagResp.StatusCode)
	assert.ElementsMatch(t, []string{"urgent", "high-value"}, tagBody["tags"])

	// Add note
	noteResp, noteBody := post(t, memberClient, gatewayURL+"/leads/"+leadIDStr+"/notes", map[string]any{
		"body": "Spoke to customer, they want to proceed next week",
	})
	require.Equal(t, http.StatusCreated, noteResp.StatusCode)
	assert.Equal(t, "Spoke to customer, they want to proceed next week", noteBody["body"])
	assert.Equal(t, "lead_agent", noteBody["author_email"])

	// List notes
	listNotesResp, listNotes := getList(t, memberClient, gatewayURL+"/leads/"+leadIDStr+"/notes")
	require.Equal(t, http.StatusOK, listNotesResp.StatusCode)
	require.Len(t, listNotes, 1)
	assert.Equal(t, "Spoke to customer, they want to proceed next week", listNotes[0]["body"])

	// List history
	listHistResp, listHist := getList(t, memberClient, gatewayURL+"/leads/"+leadIDStr+"/history")
	require.Equal(t, http.StatusOK, listHistResp.StatusCode)
	// Transition: null -> new (auto-creation), new -> won (admin), won -> lost (member)
	require.Len(t, listHist, 3)
	assert.Nil(t, listHist[0]["from_state"])
	assert.Equal(t, "new", listHist[0]["to_state"])
	assert.Equal(t, "new", listHist[1]["from_state"])
	assert.Equal(t, "won", listHist[1]["to_state"])
}



// getList sends an authenticated GET request expecting a JSON array.
func getList(t *testing.T, client *http.Client, url string) (*http.Response, []map[string]any) {
	t.Helper()
	resp, err := client.Get(url)
	require.NoError(t, err)
	t.Cleanup(func() { resp.Body.Close() })
	var result []map[string]any
	json.NewDecoder(resp.Body).Decode(&result) //nolint:errcheck
	return resp, result
}

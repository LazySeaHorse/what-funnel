package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/whatfunnel/whatfunnel/packages/go-common/messaging"
	"github.com/whatfunnel/whatfunnel/packages/go-common/pubsub"
)

type userSession struct {
	username string
	role     string
	userID   string
	client   *http.Client
	ws       *websocket.Conn
	eventsMu sync.Mutex
	events   []wsIncomingEvent
}

type wsIncomingEvent struct {
	Type           string         `json:"type"`
	ConversationID string         `json:"conversation_id"`
	Action         string         `json:"action,omitempty"`
	Message        map[string]any `json:"message,omitempty"`
}

func (u *userSession) addEvent(ev wsIncomingEvent) {
	u.eventsMu.Lock()
	defer u.eventsMu.Unlock()
	u.events = append(u.events, ev)
}

func (u *userSession) countEventsByType(evType string) int {
	u.eventsMu.Lock()
	defer u.eventsMu.Unlock()
	count := 0
	for _, e := range u.events {
		if e.Type == evType {
			count++
		}
	}
	return count
}

func TestConcurrentScaleMultiAgentMixedAI_E2E(t *testing.T) {
	skipIfServicesDown(t)
	pool := testPool(t)
	ctx := context.Background()

	adminEmail := uniqueEmail("concurrent-scale-admin")
	adminClient := newClient()

	// 1. Sign up Manager 1 (Admin/Owner)
	t.Log("Scale E2E Step 1: Sign up Manager 1 (Admin)")
	resp, body := post(t, adminClient, gatewayURL+"/auth/signup", map[string]string{
		"account_name": "E2E Scale Concurrency Account",
		"email":        adminEmail,
		"password":     "AdminPassword123!",
	})
	require.Equal(t, http.StatusCreated, resp.StatusCode, "admin signup must return 201: %v", body)
	accountIDStr := body["account_id"].(string)
	accountID := uuid.MustParse(accountIDStr)
	manager1UserID := body["user_id"].(string)

	// Register comprehensive database cleanup
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

	// Log in Manager 1
	loginResp, _ := post(t, adminClient, gatewayURL+"/auth/login", map[string]string{
		"email":    adminEmail,
		"password": "AdminPassword123!",
	})
	require.Equal(t, http.StatusOK, loginResp.StatusCode, "manager 1 login must succeed")

	// Set slug
	workspaceSlug := fmt.Sprintf("scale-ws-%s", uuid.NewString()[:8])
	slugResp, slugBody := put(t, adminClient, gatewayURL+"/workspace/account/slug", map[string]string{
		"slug": workspaceSlug,
	})
	require.Equal(t, http.StatusOK, slugResp.StatusCode, "set slug: %v", slugBody)

	// Configure workspace settings:
	// - unassigned conversations visible to members
	// - AI enabled with default auto_send
	t.Log("Scale E2E Step 2: Configure Workspace Settings")
	putResp, putBody := put(t, adminClient, gatewayURL+"/workspace/account/settings", map[string]any{
		"unassigned_conversations_visible_to_members": true,
		"ai_enabled":                                 true,
		"ai_reply_mode_default":                      "auto_send",
		"lead_tracking_enabled":                      true,
	})
	require.Equal(t, http.StatusOK, putResp.StatusCode, "settings update: %v", putBody)

	// 2. Create WhatsApp channel
	t.Log("Scale E2E Step 3: Create WhatsApp channel")
	channelIDStr := createTestProviderChannel(t, pool, accountID, "Scale WhatsApp Channel")
	channelID := uuid.MustParse(channelIDStr)

	// 3. Seed AI trigger patterns in database
	t.Log("Scale E2E Step 4: Seed AI trigger patterns")
	pattern1ID := uuid.New()
	_, err := pool.Exec(ctx, `
		INSERT INTO patterns (id, account_id, trigger_phrases, answer_text, canonical_question, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
	`, pattern1ID, accountID, []string{"Do you offer house calls?"}, "Yes, we offer house calls anytime.", "Do you offer house calls?")
	require.NoError(t, err)

	pattern2ID := uuid.New()
	_, err = pool.Exec(ctx, `
		INSERT INTO patterns (id, account_id, trigger_phrases, answer_text, canonical_question, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
	`, pattern2ID, accountID, []string{"What are your working hours?"}, "We are open Monday to Friday 9am-6pm.", "What are your working hours?")
	require.NoError(t, err)

	// 4. Provision 2 Managers and 5 Agents (Total 7 Workspace Users)
	t.Log("Scale E2E Step 5: Provision Manager 2 and 5 Agents")
	sessions := make([]*userSession, 0, 7)

	// Manager 1 session
	manager1Session := &userSession{
		username: "manager_1_admin",
		role:     "manager",
		userID:   manager1UserID,
		client:   adminClient,
	}
	sessions = append(sessions, manager1Session)

	// Create Manager 2
	mgr2Resp, mgr2Body := post(t, adminClient, gatewayURL+"/workspace/users", map[string]string{
		"username": "manager_two",
		"password": "ManagerPassword123!",
		"role":     "manager",
	})
	require.Equal(t, http.StatusCreated, mgr2Resp.StatusCode, "manager 2 creation: %v", mgr2Body)
	mgr2UserID := mgr2Body["id"].(string)

	mgr2Client := newClient()
	loginM2, _ := post(t, mgr2Client, gatewayURL+"/auth/login", map[string]string{
		"identifier": fmt.Sprintf("%s-manager_two", workspaceSlug),
		"password":   "ManagerPassword123!",
	})
	require.Equal(t, http.StatusOK, loginM2.StatusCode, "manager 2 login must succeed")
	sessions = append(sessions, &userSession{
		username: "manager_two",
		role:     "manager",
		userID:   mgr2UserID,
		client:   mgr2Client,
	})

	// Create 5 Agents
	for i := 1; i <= 5; i++ {
		username := fmt.Sprintf("agent_%d", i)
		agResp, agBody := post(t, adminClient, gatewayURL+"/workspace/users", map[string]string{
			"username": username,
			"password": "AgentPassword123!",
			"role":     "agent",
		})
		require.Equal(t, http.StatusCreated, agResp.StatusCode, "agent creation: %v", agBody)
		agentUserID := agBody["id"].(string)

		agClient := newClient()
		loginAg, _ := post(t, agClient, gatewayURL+"/auth/login", map[string]string{
			"identifier": fmt.Sprintf("%s-%s", workspaceSlug, username),
			"password":   "AgentPassword123!",
		})
		require.Equal(t, http.StatusOK, loginAg.StatusCode, "agent login must succeed")
		sessions = append(sessions, &userSession{
			username: username,
			role:     "agent",
			userID:   agentUserID,
			client:   agClient,
		})
	}
	require.Len(t, sessions, 7, "must have exactly 2 managers and 5 agents")

	// 5. Connect WebSockets for all 7 users concurrently
	t.Log("Scale E2E Step 6: Connect WebSockets for all 7 workspace staff concurrently")
	u, _ := url.Parse(gatewayURL)
	wsURL := "ws://localhost:18080/ws"

	for _, sess := range sessions {
		cookies := sess.client.Jar.Cookies(u)
		header := http.Header{}
		for _, cookie := range cookies {
			header.Add("Cookie", cookie.String())
		}
		conn, wsResp, err := websocket.DefaultDialer.Dial(wsURL, header)
		require.NoError(t, err, "websocket dial failed for %s: %v", sess.username, wsResp)
		sess.ws = conn
		t.Cleanup(func() {
			sess.ws.Close()
		})

		// Start background listener goroutine for each user
		go func(s *userSession) {
			for {
				_, msgBytes, err := s.ws.ReadMessage()
				if err != nil {
					return
				}
				var ev wsIncomingEvent
				if err := json.Unmarshal(msgBytes, &ev); err == nil {
					s.addEvent(ev)
				}
			}
		}(sess)
	}

	// Give websockets a brief moment to register in notification-svc hub
	time.Sleep(100 * time.Millisecond)

	// 6. Setup 20 Customer definitions
	t.Log("Scale E2E Step 7: Configure 20 customers (Chats 1-7 human-only, Chats 8-20 AI-enabled)")
	ps, err := pubsub.NewClient("localhost:6379")
	require.NoError(t, err)
	defer ps.Close()

	type customerSpec struct {
		id       int
		threadID string
		name     string
		text     string
		category string // "agent", "manager", "ai_calls", "ai_hours", "ai_takeover", "ai_close"
	}

	customers := make([]customerSpec, 20)
	for i := 1; i <= 20; i++ {
		c := customerSpec{
			id:       i,
			threadID: fmt.Sprintf("cust-scale-thread-%02d", i),
			name:     fmt.Sprintf("Customer #%02d", i),
		}
		switch {
		case i <= 5:
			c.text = fmt.Sprintf("Inquiry from customer %d: Need assistance with order #%d", i, 1000+i)
			c.category = "agent"
		case i == 6 || i == 7:
			c.text = fmt.Sprintf("Inquiry from customer %d: Need supervisor assistance #%d", i, 2000+i)
			c.category = "manager"
		case i >= 8 && i <= 17:
			c.text = "Do you offer house calls?"
			c.category = "ai_calls"
		case i == 18:
			c.text = "What are your working hours?"
			c.category = "ai_hours"
		case i == 19:
			c.text = "Do you offer house calls?"
			c.category = "ai_takeover"
		case i == 20:
			c.text = "What are your working hours?"
			c.category = "ai_close"
		}
		customers[i-1] = c
	}

	// For Customers 1..7 (designated for human handling by the 5 agents & 2 managers),
	// pre-create their contact and conversation with reply_override = 'disabled' and assigned to their respective staff.
	for i := 1; i <= 7; i++ {
		cust := customers[i-1]
		var assignedUserID uuid.UUID
		if i <= 5 {
			assignedUserID = uuid.MustParse(sessions[1+i].userID) // Agent 1..5
		} else if i == 6 {
			assignedUserID = uuid.MustParse(sessions[0].userID) // Manager 1
		} else {
			assignedUserID = uuid.MustParse(sessions[1].userID) // Manager 2
		}

		var contactID, convoID uuid.UUID
		err = pool.QueryRow(ctx, `
			INSERT INTO contacts (account_id, channel_id, external_identity, display_name)
			VALUES ($1, $2, $3, $4)
			RETURNING id
		`, accountID, channelID, cust.threadID, cust.name).Scan(&contactID)
		require.NoError(t, err)

		err = pool.QueryRow(ctx, `
			INSERT INTO conversations (account_id, contact_id, channel_id, external_thread_id, status, assigned_user_ids)
			VALUES ($1, $2, $3, $4, 'open', $5)
			RETURNING id
		`, accountID, contactID, channelID, cust.threadID, []uuid.UUID{assignedUserID}).Scan(&convoID)
		require.NoError(t, err)

		// Set reply_override = 'disabled' so AI leaves these chats strictly to humans
		_, err = pool.Exec(ctx, `
			UPDATE conversation_ai_state
			SET reply_override = 'disabled'
			WHERE conversation_id = $1 AND account_id = $2
		`, convoID, accountID)
		require.NoError(t, err)
	}

	// Fire all 20 inbound messages concurrently
	t.Log("Scale E2E Step 8: Concurrently publish 20 inbound messages via Redis Streams")
	var publishWG sync.WaitGroup
	for _, cust := range customers {
		publishWG.Add(1)
		go func(c customerSpec) {
			defer publishWG.Done()
			publishTestProviderMessage(
				t, ps, channelIDStr, c.threadID, c.threadID,
				c.name, c.text, fmt.Sprintf("msg-inbound-%02d", c.id),
				messaging.DirectionInbound,
			)
		}(cust)
	}
	publishWG.Wait()

	// 7. Verify all 20 conversations exist in database
	t.Log("Scale E2E Step 9: Verify all 20 conversations exist in PostgreSQL")
	require.Eventually(t, func() bool {
		var count int
		_ = pool.QueryRow(ctx, `SELECT COUNT(*) FROM conversations WHERE account_id = $1`, accountID).Scan(&count)
		return count == 20
	}, 10*time.Second, 100*time.Millisecond, "expected exactly 20 conversations created for 20 customers")

	// Map each customer thread to its conversation ID
	threadToConvo := make(map[string]uuid.UUID)
	rows, err := pool.Query(ctx, `
		SELECT co.external_identity, c.id
		FROM conversations c
		JOIN contacts co ON c.contact_id = co.id
		WHERE c.account_id = $1 AND c.channel_id = $2
	`, accountID, channelID)
	require.NoError(t, err)
	defer rows.Close()

	for rows.Next() {
		var threadID string
		var cID uuid.UUID
		require.NoError(t, rows.Scan(&threadID, &cID))
		threadToConvo[threadID] = cID
	}
	require.Len(t, threadToConvo, 20, "each of the 20 customers must have a conversation")

	// 8. Concurrent Human Interactions:
	// - 5 Agents reply to Customers 1..5 concurrently
	// - 2 Managers reply to Customers 6..7 concurrently
	t.Log("Scale E2E Step 9: 5 Agents and 2 Managers concurrently send replies to their 7 chats")
	var humanReplyWG sync.WaitGroup

	// Agents 1..5 reply to Customers 1..5
	for agentIdx := 1; agentIdx <= 5; agentIdx++ {
		humanReplyWG.Add(1)
		agentSess := sessions[1+agentIdx] // sessions[0]=Mgr1, sessions[1]=Mgr2, sessions[2..6]=Agents1..5
		cust := customers[agentIdx-1]
		convoID := threadToConvo[cust.threadID]

		go func(sess *userSession, cID uuid.UUID, idx int) {
			defer humanReplyWG.Done()
			sendResp, sendBody := post(t, sess.client, fmt.Sprintf("%s/internal/conversations/%s/send", gatewayURL, cID), map[string]any{
				"content_type":   "text",
				"text":           fmt.Sprintf("Hello! Agent %d responding to your inquiry.", idx),
				"sender_type":    "human",
				"sender_user_id": sess.userID,
			})
			assert.Equal(t, http.StatusOK, sendResp.StatusCode, "agent %d send failed: %v", idx, sendBody)
		}(agentSess, convoID, agentIdx)
	}

	// Managers 1 and 2 reply to Customers 6 and 7
	for mgrIdx := 1; mgrIdx <= 2; mgrIdx++ {
		humanReplyWG.Add(1)
		mgrSess := sessions[mgrIdx-1]
		cust := customers[5+mgrIdx-1]
		convoID := threadToConvo[cust.threadID]

		go func(sess *userSession, cID uuid.UUID, idx int) {
			defer humanReplyWG.Done()
			sendResp, sendBody := post(t, sess.client, fmt.Sprintf("%s/internal/conversations/%s/send", gatewayURL, cID), map[string]any{
				"content_type":   "text",
				"text":           fmt.Sprintf("Hello! Manager %d resolving your supervisor inquiry.", idx),
				"sender_type":    "human",
				"sender_user_id": sess.userID,
			})
			assert.Equal(t, http.StatusOK, sendResp.StatusCode, "manager %d send failed: %v", idx, sendBody)
		}(mgrSess, convoID, mgrIdx)
	}

	humanReplyWG.Wait()

	// 9. Concurrently verify AI auto-replies for Customers 8..18
	t.Log("Scale E2E Step 10: Verify AI auto-replies for Customers 8 to 18")
	for i := 8; i <= 18; i++ {
		cust := customers[i-1]
		convoID := threadToConvo[cust.threadID]

		expectedAnswer := "Yes, we offer house calls anytime."
		if cust.category == "ai_hours" {
			expectedAnswer = "We are open Monday to Friday 9am-6pm."
		}

		require.Eventually(t, func() bool {
			var count int
			var answerText string
			_ = pool.QueryRow(ctx, `
				SELECT COUNT(*), COALESCE(content->>'text', '')
				FROM messages
				WHERE conversation_id = $1 AND direction = 'outbound' AND sender_type = 'ai'
				GROUP BY content->>'text'
			`, convoID).Scan(&count, &answerText)
			return count >= 1 && answerText == expectedAnswer
		}, 15*time.Second, 200*time.Millisecond, "AI should auto-send expected reply for customer %d", i)
	}

	// 10. Customer 19: AI Auto-reply followed by Human Takeover
	t.Log("Scale E2E Step 11: Customer 19 - Verify Human Takeover pauses AI")
	cust19ConvoID := threadToConvo[customers[18].threadID]

	// Wait for AI auto-reply first
	require.Eventually(t, func() bool {
		var count int
		_ = pool.QueryRow(ctx, `
			SELECT COUNT(*) FROM messages
			WHERE conversation_id = $1 AND direction = 'outbound' AND sender_type = 'ai'
		`, cust19ConvoID).Scan(&count)
		return count >= 1
	}, 10*time.Second, 100*time.Millisecond)

	// Agent 3 jumps into Customer 19 conversation with human reply
	agent3Sess := sessions[4] // Agent 3
	takeoverResp, takeoverBody := post(t, agent3Sess.client, fmt.Sprintf("%s/internal/conversations/%s/send", gatewayURL, cust19ConvoID), map[string]any{
		"content_type":   "text",
		"text":           "Agent 3 taking over this chat directly.",
		"sender_type":    "human",
		"sender_user_id": agent3Sess.userID,
	})
	require.Equal(t, http.StatusOK, takeoverResp.StatusCode, "human takeover send failed: %v", takeoverBody)

	// Verify AI control state transitions to 'paused_human'
	var aiState19 string
	err = pool.QueryRow(ctx, `SELECT state FROM conversation_ai_state WHERE conversation_id = $1`, cust19ConvoID).Scan(&aiState19)
	require.NoError(t, err)
	assert.Equal(t, "paused_human", aiState19, "AI must transition to paused_human on human reply")

	// 11. Customer 20: AI Auto-reply followed by Manager Close & AI Resumption
	t.Log("Scale E2E Step 12: Customer 20 - Verify Manager Close resets AI state to active")
	cust20ConvoID := threadToConvo[customers[19].threadID]

	// Wait for AI auto-reply first
	require.Eventually(t, func() bool {
		var count int
		_ = pool.QueryRow(ctx, `
			SELECT COUNT(*) FROM messages
			WHERE conversation_id = $1 AND direction = 'outbound' AND sender_type = 'ai'
		`, cust20ConvoID).Scan(&count)
		return count >= 1
	}, 10*time.Second, 100*time.Millisecond)

	// Manager 2 closes Customer 20's conversation
	mgr2Sess := sessions[1]
	closeResp, closeBody := post(t, mgr2Sess.client, fmt.Sprintf("%s/conversations/%s/close", gatewayURL, cust20ConvoID), nil)
	require.Equal(t, http.StatusOK, closeResp.StatusCode, "close conversation failed: %v", closeBody)

	// Verify conversation status is closed
	var convo20Status string
	err = pool.QueryRow(ctx, `SELECT status FROM conversations WHERE id = $1`, cust20ConvoID).Scan(&convo20Status)
	require.NoError(t, err)
	assert.Equal(t, "closed", convo20Status)

	// 12. Final Database Integrity & Message Separation Checks
	t.Log("Scale E2E Step 13: Final Database Integrity Invariants")

	// Total messages in account:
	// 20 inbound messages
	// 5 agent replies
	// 2 manager replies
	// 11 AI auto-replies (Cust 8..18)
	// 1 AI auto-reply + 1 human takeover reply (Cust 19)
	// 1 AI auto-reply (Cust 20)
	// Total expected = 20 inbound + 8 human outbound + 13 AI outbound = 41 messages.
	var totalInbound, totalHumanOutbound, totalAIOutbound int
	err = pool.QueryRow(ctx, `SELECT COUNT(*) FROM messages WHERE account_id = $1 AND direction = 'inbound'`, accountID).Scan(&totalInbound)
	require.NoError(t, err)
	assert.Equal(t, 20, totalInbound, "exactly 20 inbound customer messages")

	err = pool.QueryRow(ctx, `SELECT COUNT(*) FROM messages WHERE account_id = $1 AND direction = 'outbound' AND sender_type = 'human'`, accountID).Scan(&totalHumanOutbound)
	require.NoError(t, err)
	assert.Equal(t, 8, totalHumanOutbound, "exactly 8 human outbound messages (5 agents + 2 managers + 1 takeover)")

	err = pool.QueryRow(ctx, `SELECT COUNT(*) FROM messages WHERE account_id = $1 AND direction = 'outbound' AND sender_type = 'ai'`, accountID).Scan(&totalAIOutbound)
	require.NoError(t, err)
	assert.Equal(t, 13, totalAIOutbound, "exactly 13 AI auto-replies generated")

	// 13. Verify all 7 staff received real-time WebSocket events
	t.Log("Scale E2E Step 14: Verify WebSocket events received across all 7 staff")
	for _, sess := range sessions {
		recvdCount := sess.countEventsByType("message.received")
		sentCount := sess.countEventsByType("message.sent")
		assert.GreaterOrEqual(t, recvdCount, 1, "%s must receive real-time inbound message events", sess.username)
		assert.GreaterOrEqual(t, sentCount, 1, "%s must receive real-time outbound message events", sess.username)
	}

	t.Log("Scale E2E Test completed successfully with 2 Managers, 5 Agents, and 20 Customers!")
}

// TestConcurrentScale100Users_E2E simulates 100 concurrent users pushing past
// the standard 25-user limit to verify concurrency handling, database integrity,
// and zero dropped events under 100-goroutine load.
func TestConcurrentScale100Users_E2E(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping 100-user concurrent scale test in short mode")
	}
	skipIfServicesDown(t)

	pool := testPool(t)
	ctx := context.Background()

	adminEmail := uniqueEmail("scale100-admin")
	adminClient := newClient()

	// 1. Sign up Admin
	resp, body := post(t, adminClient, gatewayURL+"/auth/signup", map[string]string{
		"account_name": "E2E Scale 100 Account",
		"email":        adminEmail,
		"password":     "AdminPassword123!",
	})
	require.Equal(t, http.StatusCreated, resp.StatusCode, "admin signup: %v", body)
	accountIDStr := body["account_id"].(string)
	accountID := uuid.MustParse(accountIDStr)

	t.Cleanup(func() {
		pool.Exec(ctx, `DELETE FROM sessions WHERE data::text LIKE '%'||$1||'%'`, accountIDStr)
		pool.Exec(ctx, `DELETE FROM messages WHERE account_id = $1`, accountID)
		pool.Exec(ctx, `DELETE FROM conversations WHERE account_id = $1`, accountID)
		pool.Exec(ctx, `DELETE FROM contacts WHERE account_id = $1`, accountID)
		pool.Exec(ctx, `DELETE FROM channels WHERE account_id = $1`, accountID)
		pool.Exec(ctx, `DELETE FROM users WHERE account_id = $1`, accountID)
		pool.Exec(ctx, `DELETE FROM accounts WHERE id = $1`, accountID)
	})

	// Log in Admin
	loginResp, _ := post(t, adminClient, gatewayURL+"/auth/login", map[string]string{
		"email":    adminEmail,
		"password": "AdminPassword123!",
	})
	require.Equal(t, http.StatusOK, loginResp.StatusCode)

	// Create channel
	channelID := createTestProviderChannel(t, pool, accountID, "Scale 100 Channel")

	ps, err := pubsub.NewClient("localhost:6379")
	require.NoError(t, err)
	defer ps.Close()

	const totalUsers = 100
	t.Logf("Scale E2E: Launching %d concurrent user goroutines", totalUsers)

	var wg sync.WaitGroup
	errCh := make(chan error, totalUsers)

	startBarrier := make(chan struct{})

	for i := 0; i < totalUsers; i++ {
		wg.Add(1)
		go func(userIdx int) {
			defer wg.Done()
			<-startBarrier

			customerPhone := fmt.Sprintf("+1555%07d", userIdx)
			providerMsgID := fmt.Sprintf("scale100_msg_%s_%d", uuid.NewString()[:8], userIdx)
			msgText := fmt.Sprintf("Scale load message from user %d", userIdx)

			now := time.Now().UTC()
			event := messaging.Event{
				SchemaVersion: messaging.SchemaVersion,
				ID:            fmt.Sprintf("scale100:ev:%d:%s", userIdx, uuid.NewString()),
				Kind:          messaging.EventMessageCreated,
				Provider:      messaging.ProviderWhatsApp,
				ChannelID:     channelID,
				OccurredAt:    now,
				Message: &messaging.Message{
					ProviderMessageID: providerMsgID,
					ExternalThreadID:  customerPhone,
					Direction:         messaging.DirectionInbound,
					Sender: messaging.Sender{
						ExternalID:  customerPhone,
						DisplayName: fmt.Sprintf("Customer %d", userIdx),
					},
					ContentType:       messaging.ContentText,
					Text:              msgText,
					ProviderTimestamp: now,
				},
			}

			if _, err := ps.Publish(ctx, "adapter.events", event); err != nil {
				errCh <- fmt.Errorf("user %d publish error: %w", userIdx, err)
			}
		}(i)
	}

	// Release all 100 goroutines simultaneously
	close(startBarrier)
	wg.Wait()
	close(errCh)

	for err := range errCh {
		require.NoError(t, err)
	}

	// Verify all 100 inbound messages are ingested and persisted
	t.Log("Scale E2E: Verifying all 100 messages are persisted")
	require.Eventually(t, func() bool {
		var count int
		_ = pool.QueryRow(ctx, `
			SELECT COUNT(*) FROM messages
			WHERE account_id = $1 AND direction = 'inbound'
		`, accountID).Scan(&count)
		return count == totalUsers
	}, 20*time.Second, 200*time.Millisecond, "expected all 100 messages to be processed")

	// Verify 100 distinct conversations were created
	var convoCount int
	err = pool.QueryRow(ctx, `SELECT COUNT(*) FROM conversations WHERE account_id = $1`, accountID).Scan(&convoCount)
	require.NoError(t, err)
	assert.Equal(t, totalUsers, convoCount, "expected exactly 100 conversations for 100 distinct users")

	t.Logf("Scale E2E: Successfully processed %d concurrent users without data loss or race conditions", totalUsers)
}


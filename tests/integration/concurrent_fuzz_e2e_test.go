package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/whatfunnel/whatfunnel/packages/go-common/messaging"
	"github.com/whatfunnel/whatfunnel/packages/go-common/pubsub"
)

// Curated fuzz dictionary for messages and customer names
var (
	fuzzAITriggerPhrases1 = []string{
		"Do you offer house calls?",
		"do you offer house calls???",
		"DO YOU OFFER HOUSE CALLS!",
		"Do you offer house call",
		"  do you offer house calls?  ",
		"do you offer house calls",
		"Do you offer House Calls?!",
	}

	fuzzAITriggerPhrases2 = []string{
		"What are your working hours?",
		"what are your working hours??",
		"WHAT ARE YOUR WORKING HOURS",
		"what are your working hours",
		"What are your working hours",
		"  what are your working hours?  ",
	}

	fuzzHumanInquiries = []string{
		"Can someone please help me with order #4829?",
		"Need to speak to a manager right now regarding billing!",
		"How can I return an item I purchased last week?",
		"Hello? Is anyone online to assist me with a customized quote?",
		"I have an urgent request regarding account cancellation.",
	}

	fuzzEdgeCaseTexts = []string{
		"Special symbols: !@#$%^&*()_+-=[]{}|;':\",./<>?`~",
		"Unicode emojis: 🚀💎⚡️🤖 🔥 👩‍👩‍👧‍👦",
		"Multi-byte international: ñ é ö ü ß 日本語 한글 مرحبا שלום",
		"XSS injection probe: <script>alert('xss-fuzz')</script>",
		"SQL injection probe: '; DROP TABLE messages; -- ' OR '1'='1",
		"Format string probe: %s%s%s%s%s %d%d%d %x%x",
		"Whitespace padding:   \n\t  Important inquiry  \r\n\t  ",
	}

	fuzzEmojis = []string{"🚀", "💎", "⚡️", "🤖", "🔥", "🌟", "✨", "🎯"}
)

func randomElement[T any](rng *rand.Rand, slice []T) T {
	return slice[rng.Intn(len(slice))]
}

type fuzzCustomerPlan struct {
	id          int
	threadID    string
	displayName string
	category    string // "human_agent", "human_manager", "ai_calls", "ai_hours", "ai_takeover", "ai_close", "edge_case"
	messages    []string
}

func TestConcurrentScaleMultiAgentFuzz_E2E(t *testing.T) {
	skipIfServicesDown(t)
	pool := testPool(t)
	ctx := context.Background()

	// 1. Seed Initialization: Support reproducible seeds via FUZZ_SEED env var
	seed := time.Now().UnixNano()
	if envSeed := os.Getenv("FUZZ_SEED"); envSeed != "" {
		if parsed, err := strconv.ParseInt(envSeed, 10, 64); err == nil {
			seed = parsed
		}
	}
	t.Logf("=== RUNNING MULTI-AGENT SCALE FUZZ TEST WITH SEED: %d ===", seed)
	t.Logf("(To reproduce this exact fuzz sequence, run: FUZZ_SEED=%d go test ...)", seed)
	rng := rand.New(rand.NewSource(seed))

	// 2. Setup Workspace & Manager 1 (Admin)
	adminEmail := uniqueEmail(fmt.Sprintf("fuzz-admin-%d", seed%10000))
	adminClient := newClient()

	t.Log("Fuzz Step 1: Sign up Manager 1 (Admin)")
	resp, body := post(t, adminClient, gatewayURL+"/auth/signup", map[string]string{
		"account_name": fmt.Sprintf("Fuzz Workspace %s", randomElement(rng, fuzzEmojis)),
		"email":        adminEmail,
		"password":     "AdminPassword123!",
	})
	require.Equal(t, http.StatusCreated, resp.StatusCode, "admin signup: %v", body)
	accountIDStr := body["account_id"].(string)
	accountID := uuid.MustParse(accountIDStr)
	manager1UserID := body["user_id"].(string)

	// Database cleanup hook
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

	loginResp, _ := post(t, adminClient, gatewayURL+"/auth/login", map[string]string{
		"email":    adminEmail,
		"password": "AdminPassword123!",
	})
	require.Equal(t, http.StatusOK, loginResp.StatusCode, "manager 1 login must succeed")

	workspaceSlug := fmt.Sprintf("fuzz-ws-%d-%s", seed%10000, uuid.NewString()[:6])
	slugResp, slugBody := put(t, adminClient, gatewayURL+"/workspace/account/slug", map[string]string{
		"slug": workspaceSlug,
	})
	require.Equal(t, http.StatusOK, slugResp.StatusCode, "set slug: %v", slugBody)

	// Workspace settings: auto_send default for pattern matches, visible unassigned
	t.Log("Fuzz Step 2: Configure Workspace Settings")
	putResp, putBody := put(t, adminClient, gatewayURL+"/workspace/account/settings", map[string]any{
		"unassigned_conversations_visible_to_members": true,
		"ai_enabled":                                 true,
		"ai_reply_mode_default":                      "auto_send",
		"lead_tracking_enabled":                      true,
	})
	require.Equal(t, http.StatusOK, putResp.StatusCode, "settings update: %v", putBody)

	// 3. Create WhatsApp channel
	t.Log("Fuzz Step 3: Create WhatsApp channel")
	channelIDStr := createTestProviderChannel(t, pool, accountID, "Fuzz WhatsApp Channel")
	channelID := uuid.MustParse(channelIDStr)

	// 4. Seed Canonical AI Patterns
	t.Log("Fuzz Step 4: Seed AI trigger patterns in database")
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

	// 5. Provision 2 Managers and 5 Agents (7 Staff Total)
	t.Log("Fuzz Step 5: Provision 2 Managers and 5 Agents")
	staffSessions := make([]*userSession, 0, 7)

	// Manager 1 session
	staffSessions = append(staffSessions, &userSession{
		username: "mgr_1_admin",
		role:     "manager",
		userID:   manager1UserID,
		client:   adminClient,
	})

	// Manager 2
	m2Resp, m2Body := post(t, adminClient, gatewayURL+"/workspace/users", map[string]string{
		"username": "mgr_two",
		"password": "ManagerPassword123!",
		"role":     "manager",
	})
	require.Equal(t, http.StatusCreated, m2Resp.StatusCode, "manager 2 creation: %v", m2Body)
	m2Client := newClient()
	loginM2, _ := post(t, m2Client, gatewayURL+"/auth/login", map[string]string{
		"identifier": fmt.Sprintf("%s-mgr_two", workspaceSlug),
		"password":   "ManagerPassword123!",
	})
	require.Equal(t, http.StatusOK, loginM2.StatusCode)
	staffSessions = append(staffSessions, &userSession{
		username: "mgr_two",
		role:     "manager",
		userID:   m2Body["id"].(string),
		client:   m2Client,
	})

	// 5 Agents
	for i := 1; i <= 5; i++ {
		username := fmt.Sprintf("fuzz_agent_%d", i)
		agResp, agBody := post(t, adminClient, gatewayURL+"/workspace/users", map[string]string{
			"username": username,
			"password": "AgentPassword123!",
			"role":     "agent",
		})
		require.Equal(t, http.StatusCreated, agResp.StatusCode, "agent creation: %v", agBody)
		agClient := newClient()
		loginAg, _ := post(t, agClient, gatewayURL+"/auth/login", map[string]string{
			"identifier": fmt.Sprintf("%s-%s", workspaceSlug, username),
			"password":   "AgentPassword123!",
		})
		require.Equal(t, http.StatusOK, loginAg.StatusCode)
		staffSessions = append(staffSessions, &userSession{
			username: username,
			role:     "agent",
			userID:   agBody["id"].(string),
			client:   agClient,
		})
	}
	require.Len(t, staffSessions, 7)

	// 6. Connect Concurrent WebSockets for all 7 Staff
	t.Log("Fuzz Step 6: Connect Concurrent WebSockets for all 7 Staff")
	u, _ := url.Parse(gatewayURL)
	wsURL := "ws://localhost:18080/ws"

	for _, sess := range staffSessions {
		cookies := sess.client.Jar.Cookies(u)
		header := http.Header{}
		for _, cookie := range cookies {
			header.Add("Cookie", cookie.String())
		}
		conn, wsResp, err := websocket.DefaultDialer.Dial(wsURL, header)
		require.NoError(t, err, "ws dial failed for %s: %v", sess.username, wsResp)
		sess.ws = conn
		t.Cleanup(func() { sess.ws.Close() })

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

	time.Sleep(100 * time.Millisecond)

	// 7. Generate Fuzz Plan for 20 Customers with Randomized Traffic
	t.Log("Fuzz Step 7: Generate randomized Fuzz Traffic Plan for 20 Customers")
	plans := make([]fuzzCustomerPlan, 20)
	var expectedTotalInboundMessages int32

	for i := 1; i <= 20; i++ {
		plan := fuzzCustomerPlan{
			id:          i,
			threadID:    fmt.Sprintf("cust-fuzz-thread-%d-%02d", seed%10000, i),
			displayName: fmt.Sprintf("Fuzzer #%02d %s", i, randomElement(rng, fuzzEmojis)),
		}

		// Distribute customer categories:
		// 1..5: human agents
		// 6..7: human managers
		// 8..17: fuzzy AI triggers
		// 18: fuzzy working hours
		// 19: AI takeover
		// 20: AI close
		switch {
		case i <= 5:
			plan.category = "human_agent"
			numMsgs := 1 + rng.Intn(2) // 1 to 2 messages
			for m := 0; m < numMsgs; m++ {
				plan.messages = append(plan.messages, fmt.Sprintf("%s [tag: %d-%d]", randomElement(rng, fuzzHumanInquiries), i, m))
			}
		case i == 6 || i == 7:
			plan.category = "human_manager"
			plan.messages = []string{fmt.Sprintf("%s [manager-urgent: %d]", randomElement(rng, fuzzHumanInquiries), i)}
		case i >= 8 && i <= 17:
			plan.category = "ai_calls"
			// Use random fuzzy variations of trigger 1 to test rapidfuzz pattern matching
			plan.messages = []string{randomElement(rng, fuzzAITriggerPhrases1)}
		case i == 18:
			plan.category = "ai_hours"
			// Use random fuzzy variations of trigger 2
			plan.messages = []string{randomElement(rng, fuzzAITriggerPhrases2)}
		case i == 19:
			plan.category = "ai_takeover"
			plan.messages = []string{randomElement(rng, fuzzAITriggerPhrases1)}
		case i == 20:
			plan.category = "ai_close"
			plan.messages = []string{randomElement(rng, fuzzAITriggerPhrases2)}
		}

		atomic.AddInt32(&expectedTotalInboundMessages, int32(len(plan.messages)))
		plans[i-1] = plan
	}

	// Pre-create Chats 1..7 for human handling with reply_override = 'disabled' and assign to staff
	for i := 1; i <= 7; i++ {
		p := plans[i-1]
		var assignedUserID uuid.UUID
		if i <= 5 {
			assignedUserID = uuid.MustParse(staffSessions[1+i].userID) // Agent 1..5
		} else if i == 6 {
			assignedUserID = uuid.MustParse(staffSessions[0].userID) // Manager 1
		} else {
			assignedUserID = uuid.MustParse(staffSessions[1].userID) // Manager 2
		}

		var contactID, convoID uuid.UUID
		err = pool.QueryRow(ctx, `
			INSERT INTO contacts (account_id, channel_id, external_identity, display_name)
			VALUES ($1, $2, $3, $4)
			RETURNING id
		`, accountID, channelID, p.threadID, p.displayName).Scan(&contactID)
		require.NoError(t, err)

		err = pool.QueryRow(ctx, `
			INSERT INTO conversations (account_id, contact_id, channel_id, external_thread_id, status, assigned_user_ids)
			VALUES ($1, $2, $3, $4, 'open', $5)
			RETURNING id
		`, accountID, contactID, channelID, p.threadID, []uuid.UUID{assignedUserID}).Scan(&convoID)
		require.NoError(t, err)

		_, err = pool.Exec(ctx, `
			UPDATE conversation_ai_state
			SET reply_override = 'disabled'
			WHERE conversation_id = $1 AND account_id = $2
		`, convoID, accountID)
		require.NoError(t, err)
	}

	// 8. Publish Fuzz Inbound Messages with Random Jitter & Shuffled Arrival
	t.Log("Fuzz Step 8: Fire randomized concurrent inbound messages via Redis Streams")
	ps, err := pubsub.NewClient("localhost:6379")
	require.NoError(t, err)
	defer ps.Close()

	var publishWG sync.WaitGroup
	for _, p := range plans {
		publishWG.Add(1)
		go func(plan fuzzCustomerPlan) {
			defer publishWG.Done()
			for idx, msgText := range plan.messages {
				// Random timing jitter between 0 and 40 milliseconds to simulate real network burst
				jitter := time.Duration(rng.Intn(40)) * time.Millisecond
				time.Sleep(jitter)

				publishTestProviderMessage(
					t, ps, channelIDStr, plan.threadID, plan.threadID,
					plan.displayName, msgText, fmt.Sprintf("fuzz-inbound-%02d-%d", plan.id, idx),
					messaging.DirectionInbound,
				)
			}
		}(p)
	}
	publishWG.Wait()

	// 9. Verify Inbound Ingest Invariant in Postgres
	t.Log("Fuzz Step 9: Verify 20 conversations created & all inbound messages ingested")
	require.Eventually(t, func() bool {
		var count int
		_ = pool.QueryRow(ctx, `SELECT COUNT(*) FROM conversations WHERE account_id = $1`, accountID).Scan(&count)
		return count == 20
	}, 10*time.Second, 100*time.Millisecond, "expected 20 conversations created")

	require.Eventually(t, func() bool {
		var count int
		_ = pool.QueryRow(ctx, `SELECT COUNT(*) FROM messages WHERE account_id = $1 AND direction = 'inbound'`, accountID).Scan(&count)
		return int32(count) == atomic.LoadInt32(&expectedTotalInboundMessages)
	}, 10*time.Second, 100*time.Millisecond, "expected all fuzzed inbound messages ingested")

	// Map threadID -> Conversation ID
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
	require.Len(t, threadToConvo, 20)

	// 10. Concurrently perform randomized Human Interactions with Fuzz Payloads
	t.Log("Fuzz Step 10: Concurrently perform randomized Human Interactions with Fuzz Payloads")
	var humanWG sync.WaitGroup
	var expectedHumanReplies int32

	// Agents 1..5 concurrently handle Chats 1..5 with random think times and edge-case text payloads
	for agentIdx := 1; agentIdx <= 5; agentIdx++ {
		humanWG.Add(1)
		atomic.AddInt32(&expectedHumanReplies, 1)

		agentSess := staffSessions[1+agentIdx]
		plan := plans[agentIdx-1]
		convoID := threadToConvo[plan.threadID]

		go func(sess *userSession, cID uuid.UUID, idx int) {
			defer humanWG.Done()
			// Random think jitter (10 - 80ms)
			time.Sleep(time.Duration(rng.Intn(70)+10) * time.Millisecond)

			// Randomly use a standard response or an edge-case unicode/emoji fuzz string
			var replyText string
			if rng.Float32() < 0.5 {
				replyText = fmt.Sprintf("Agent %d verified: %s", idx, randomElement(rng, fuzzEdgeCaseTexts))
			} else {
				replyText = fmt.Sprintf("Hello! Agent %d handling your request promptly.", idx)
			}

			sendResp, sendBody := post(t, sess.client, fmt.Sprintf("%s/internal/conversations/%s/send", gatewayURL, cID), map[string]any{
				"content_type":   "text",
				"text":           replyText,
				"sender_type":    "human",
				"sender_user_id": sess.userID,
			})
			assert.Equal(t, http.StatusOK, sendResp.StatusCode, "agent %d send error: %v", idx, sendBody)
		}(agentSess, convoID, agentIdx)
	}

	// Managers 1..2 handle Chats 6..7
	for mgrIdx := 1; mgrIdx <= 2; mgrIdx++ {
		humanWG.Add(1)
		atomic.AddInt32(&expectedHumanReplies, 1)

		mgrSess := staffSessions[mgrIdx-1]
		plan := plans[5+mgrIdx-1]
		convoID := threadToConvo[plan.threadID]

		go func(sess *userSession, cID uuid.UUID, idx int) {
			defer humanWG.Done()
			time.Sleep(time.Duration(rng.Intn(50)+10) * time.Millisecond)

			replyText := fmt.Sprintf("Manager %d supervisor review: %s", idx, randomElement(rng, fuzzEdgeCaseTexts))
			sendResp, sendBody := post(t, sess.client, fmt.Sprintf("%s/internal/conversations/%s/send", gatewayURL, cID), map[string]any{
				"content_type":   "text",
				"text":           replyText,
				"sender_type":    "human",
				"sender_user_id": sess.userID,
			})
			assert.Equal(t, http.StatusOK, sendResp.StatusCode, "manager %d send error: %v", idx, sendBody)
		}(mgrSess, convoID, mgrIdx)
	}

	humanWG.Wait()

	// 11. Concurrently Verify AI Auto-Replies for Chats 8..18 under fuzzy trigger matching
	t.Log("Fuzz Step 11: Verify fuzzy pattern matching & auto-send replies for Customers 8 to 18")
	for i := 8; i <= 18; i++ {
		plan := plans[i-1]
		convoID := threadToConvo[plan.threadID]

		expectedSubstring := "house calls"
		if plan.category == "ai_hours" {
			expectedSubstring = "Monday to Friday"
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
			return count >= 1 && strings.Contains(answerText, expectedSubstring)
		}, 15*time.Second, 200*time.Millisecond, "AI fuzzy matching failed for customer %d (prompt: %s)", i, plan.messages[0])
	}

	// 12. Human Takeover Race on Chat 19
	t.Log("Fuzz Step 12: Human Takeover Race on Chat 19")
	cust19ConvoID := threadToConvo[plans[18].threadID]

	// Wait for AI auto-reply
	require.Eventually(t, func() bool {
		var count int
		_ = pool.QueryRow(ctx, `SELECT COUNT(*) FROM messages WHERE conversation_id = $1 AND sender_type = 'ai'`, cust19ConvoID).Scan(&count)
		return count >= 1
	}, 10*time.Second, 100*time.Millisecond)

	// Agent 4 races in with human takeover message
	agent4Sess := staffSessions[5]
	takeoverResp, takeoverBody := post(t, agent4Sess.client, fmt.Sprintf("%s/internal/conversations/%s/send", gatewayURL, cust19ConvoID), map[string]any{
		"content_type":   "text",
		"text":           fmt.Sprintf("Agent 4 taking over: %s", randomElement(rng, fuzzEdgeCaseTexts)),
		"sender_type":    "human",
		"sender_user_id": agent4Sess.userID,
	})
	require.Equal(t, http.StatusOK, takeoverResp.StatusCode, "takeover send error: %v", takeoverBody)
	atomic.AddInt32(&expectedHumanReplies, 1)

	var aiState19 string
	err = pool.QueryRow(ctx, `SELECT state FROM conversation_ai_state WHERE conversation_id = $1`, cust19ConvoID).Scan(&aiState19)
	require.NoError(t, err)
	assert.Equal(t, "paused_human", aiState19, "AI must transition to paused_human on takeover")

	// 13. Manager Close on Chat 20
	t.Log("Fuzz Step 13: Manager Close & AI Resumption on Chat 20")
	cust20ConvoID := threadToConvo[plans[19].threadID]

	require.Eventually(t, func() bool {
		var count int
		_ = pool.QueryRow(ctx, `SELECT COUNT(*) FROM messages WHERE conversation_id = $1 AND sender_type = 'ai'`, cust20ConvoID).Scan(&count)
		return count >= 1
	}, 10*time.Second, 100*time.Millisecond)

	mgr1Sess := staffSessions[0]
	closeResp, closeBody := post(t, mgr1Sess.client, fmt.Sprintf("%s/conversations/%s/close", gatewayURL, cust20ConvoID), nil)
	require.Equal(t, http.StatusOK, closeResp.StatusCode, "close error: %v", closeBody)

	var convo20Status string
	err = pool.QueryRow(ctx, `SELECT status FROM conversations WHERE id = $1`, cust20ConvoID).Scan(&convo20Status)
	require.NoError(t, err)
	assert.Equal(t, "closed", convo20Status)

	// 14. Global Invariant Assertions
	t.Log("Fuzz Step 14: Global Invariant Assertions (No crashes, correct counts, safe storage)")

	var totalInbound, totalHumanOutbound, totalAIOutbound int
	err = pool.QueryRow(ctx, `SELECT COUNT(*) FROM messages WHERE account_id = $1 AND direction = 'inbound'`, accountID).Scan(&totalInbound)
	require.NoError(t, err)
	assert.Equal(t, int(atomic.LoadInt32(&expectedTotalInboundMessages)), totalInbound)

	err = pool.QueryRow(ctx, `SELECT COUNT(*) FROM messages WHERE account_id = $1 AND direction = 'outbound' AND sender_type = 'human'`, accountID).Scan(&totalHumanOutbound)
	require.NoError(t, err)
	assert.Equal(t, int(atomic.LoadInt32(&expectedHumanReplies)), totalHumanOutbound)

	err = pool.QueryRow(ctx, `SELECT COUNT(*) FROM messages WHERE account_id = $1 AND direction = 'outbound' AND sender_type = 'ai'`, accountID).Scan(&totalAIOutbound)
	require.NoError(t, err)
	assert.Equal(t, 13, totalAIOutbound, "exactly 13 AI auto-replies across chats 8..20")

	// 15. WebSocket Broadcast Invariant
	t.Log("Fuzz Step 15: Validate WebSocket broadcast delivery across all 7 staff")
	for _, sess := range staffSessions {
		recvd := sess.countEventsByType("message.received")
		sent := sess.countEventsByType("message.sent")
		assert.GreaterOrEqual(t, recvd, 1, "%s must receive real-time inbound events", sess.username)
		assert.GreaterOrEqual(t, sent, 1, "%s must receive real-time outbound events", sess.username)
	}

	t.Logf("=== FUZZ TEST PASSED WITH SEED %d (20 Customers, 7 Staff, %d inbound, %d human, %d AI) ===",
		seed, totalInbound, totalHumanOutbound, totalAIOutbound)
}

package service_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/whatfunnel/whatfunnel/packages/go-common/db"
	"github.com/whatfunnel/whatfunnel/packages/go-common/messaging"
	"github.com/whatfunnel/whatfunnel/packages/go-common/pubsub"
	"github.com/whatfunnel/whatfunnel/packages/go-common/types"
	"github.com/whatfunnel/whatfunnel/services/conversation-svc/internal/service"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://whatfunnel:whatfunnel@localhost:5432/whatfunnel?sslmode=disable"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := db.Connect(ctx, dsn)
	if err != nil {
		if os.Getenv("CI") != "" {
			t.Fatalf("integration test database is required in CI: %v", err)
		}
		t.Skipf("skipping integration test: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func testService(t *testing.T) (*service.Service, *pgxpool.Pool, *pubsub.Client) {
	t.Helper()
	pool := testPool(t)

	redisAddr := os.Getenv("REDIS_URL")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}
	ps, err := pubsub.NewClient(redisAddr)
	require.NoError(t, err)

	svc := service.New(pool, ps)
	t.Cleanup(func() {
		ps.Close()
	})
	return svc, pool, ps
}

func setupTestTenant(t *testing.T, pool *pgxpool.Pool, name string) (uuid.UUID, uuid.UUID) {
	t.Helper()
	ctx := context.Background()

	var accountID uuid.UUID
	err := pool.QueryRow(ctx, `INSERT INTO accounts (name) VALUES ($1) RETURNING id`, name).Scan(&accountID)
	require.NoError(t, err)

	statesJSON, _ := json.Marshal(types.DefaultPipelineStates)
	_, err = pool.Exec(ctx, `INSERT INTO lead_pipelines (account_id, name, states) VALUES ($1, 'Default', $2)`, accountID, statesJSON)
	require.NoError(t, err)

	var userID uuid.UUID
	err = pool.QueryRow(ctx, `INSERT INTO users (account_id, email, password_hash, role) VALUES ($1, $2, 'hash', 'manager') RETURNING id`, accountID, name+"@example.com").Scan(&userID)
	require.NoError(t, err)

	t.Cleanup(func() {
		pool.Exec(context.Background(), `DELETE FROM lead_state_history WHERE account_id = $1`, accountID)
		pool.Exec(context.Background(), `DELETE FROM lead_notes WHERE account_id = $1`, accountID)
		pool.Exec(context.Background(), `DELETE FROM leads WHERE account_id = $1`, accountID)
		pool.Exec(context.Background(), `DELETE FROM lead_pipelines WHERE account_id = $1`, accountID)
		pool.Exec(context.Background(), `DELETE FROM audit_logs WHERE account_id = $1`, accountID)
		pool.Exec(context.Background(), `DELETE FROM messages WHERE account_id = $1`, accountID)
		pool.Exec(context.Background(), `DELETE FROM conversations WHERE account_id = $1`, accountID)
		pool.Exec(context.Background(), `DELETE FROM contacts WHERE account_id = $1`, accountID)
		pool.Exec(context.Background(), `DELETE FROM channels WHERE account_id = $1`, accountID)
		pool.Exec(context.Background(), `DELETE FROM users WHERE account_id = $1`, accountID)
		pool.Exec(context.Background(), `DELETE FROM accounts WHERE id = $1`, accountID)
	})

	return accountID, userID
}

// providerMessage builds a message.created event for one provider message;
// configure adjusts the message (sender, content) before it is sent.
func providerMessage(channelID uuid.UUID, thread, providerMessageID string, configure func(*messaging.Message)) messaging.Event {
	event := providerCreatedEvent(channelID, "evt-"+uuid.NewString(), providerMessageID, thread, time.Now())
	if configure != nil {
		configure(event.Message)
	}
	return event
}

func TestIngestProviderEvent_InboundMessages(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	svc, pool, _ := testService(t)
	ctx := context.Background()

	accountID, _ := setupTestTenant(t, pool, "ingest-test")
	channelID := newTelegramChannel(t, pool, accountID, "ingest bot")
	cleanupProviderEvents(t, pool, channelID)

	// 1. Ingest a message for a new contact (new contact + new conversation)
	event1 := providerMessage(channelID, "12345", "msg-id-1", func(m *messaging.Message) {
		m.Sender.DisplayName = "Bob"
		m.Sender.AvatarURL = "http://avatar.url/bob"
		m.Text = "Hello business!"
	})
	require.NoError(t, svc.IngestProviderEvent(ctx, event1))

	// Verify DB state
	var contactID uuid.UUID
	var displayName, avatarURL string
	err := pool.QueryRow(ctx, `SELECT id, display_name, avatar_url FROM contacts WHERE channel_id = $1 AND external_identity = $2`, channelID, "12345").
		Scan(&contactID, &displayName, &avatarURL)
	require.NoError(t, err)
	assert.Equal(t, "Bob", displayName)
	assert.Equal(t, "http://avatar.url/bob", avatarURL)

	var convoID uuid.UUID
	var convoStatus string
	err = pool.QueryRow(ctx, `SELECT id, status FROM conversations WHERE contact_id = $1`, contactID).
		Scan(&convoID, &convoStatus)
	require.NoError(t, err)
	assert.Equal(t, "open", convoStatus)

	var msgCount int
	err = pool.QueryRow(ctx, `SELECT count(*) FROM messages WHERE conversation_id = $1`, convoID).Scan(&msgCount)
	require.NoError(t, err)
	assert.Equal(t, 1, msgCount)

	// 2. Redelivered event (idempotency check)
	require.NoError(t, svc.IngestProviderEvent(ctx, event1)) // Should skip without failing

	err = pool.QueryRow(ctx, `SELECT count(*) FROM messages WHERE conversation_id = $1`, convoID).Scan(&msgCount)
	require.NoError(t, err)
	assert.Equal(t, 1, msgCount, "message count should still be 1 (deduplicated)")

	// 3. Second message from same contact (reuses conversation)
	event2 := providerMessage(channelID, "12345", "msg-id-2", func(m *messaging.Message) {
		m.Sender.DisplayName = "Bob Modified"
		m.Sender.AvatarURL = "" // empty should not overwrite
		m.Text = "Look at this!"
	})
	require.NoError(t, svc.IngestProviderEvent(ctx, event2))

	err = pool.QueryRow(ctx, `SELECT count(*) FROM messages WHERE conversation_id = $1`, convoID).Scan(&msgCount)
	require.NoError(t, err)
	assert.Equal(t, 2, msgCount)

	// Display name updated, avatar retained
	err = pool.QueryRow(ctx, `SELECT display_name, avatar_url FROM contacts WHERE id = $1`, contactID).
		Scan(&displayName, &avatarURL)
	require.NoError(t, err)
	assert.Equal(t, "Bob Modified", displayName)
	assert.Equal(t, "http://avatar.url/bob", avatarURL)

	// 4. Every content type is stored
	contentTypes := []messaging.ContentType{
		messaging.ContentText, messaging.ContentImage, messaging.ContentVideo,
		messaging.ContentAudio, messaging.ContentDocument, messaging.ContentNotice,
	}
	for _, ct := range contentTypes {
		event := providerMessage(channelID, "12345", "msg-ct-"+string(ct), func(m *messaging.Message) {
			m.ContentType = ct
			m.Text = "test " + string(ct)
			switch ct {
			case messaging.ContentNotice:
				m.NoticeCode = "unsupported"
			case messaging.ContentImage, messaging.ContentVideo, messaging.ContentAudio, messaging.ContentDocument:
				m.Media = &messaging.Media{ProviderRef: "ref-" + string(ct), MIMEType: "application/octet-stream", SizeBytes: 10}
			}
		})
		require.NoError(t, svc.IngestProviderEvent(ctx, event), ct)
	}

	// Verify count is 2 + 6 = 8.
	err = pool.QueryRow(ctx, `SELECT count(*) FROM messages WHERE conversation_id = $1`, convoID).Scan(&msgCount)
	require.NoError(t, err)
	assert.Equal(t, 8, msgCount)
}

func TestEnsureSimulatorChannel(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	svc, pool, _ := testService(t)
	accountID, _ := setupTestTenant(t, pool, "simulator-channel-test")

	first, err := svc.EnsureSimulatorChannel(t.Context(), accountID, "whatsapp")
	require.NoError(t, err)
	second, err := svc.EnsureSimulatorChannel(t.Context(), accountID, "whatsapp")
	require.NoError(t, err)
	assert.Equal(t, first.ID, second.ID)
	assert.Equal(t, "whatsapp", first.Type)
	assert.Equal(t, "connected", first.Status)

	var connectionCount int
	require.NoError(t, pool.QueryRow(t.Context(), `
		SELECT COUNT(*) FROM provider_connections WHERE channel_id = $1
	`, first.ID).Scan(&connectionCount))
	assert.Zero(t, connectionCount)

	_, err = svc.EnsureSimulatorChannel(t.Context(), accountID, "instagram")
	assert.ErrorIs(t, err, service.ErrUnsupportedSimulatorProvider)
}

func TestLeadManagement(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	svc, pool, _ := testService(t)
	ctx := context.Background()

	accountID, adminID := setupTestTenant(t, pool, "lead-mgmt-test")

	// Create agent user who is NOT assigned
	var memberID uuid.UUID
	err := pool.QueryRow(ctx, `INSERT INTO users (account_id, email, password_hash, role) VALUES ($1, $2, 'hash', 'agent') RETURNING id`, accountID, fmt.Sprintf("member_%s@example.com", uuid.New())).Scan(&memberID)
	require.NoError(t, err)

	channelID := newTelegramChannel(t, pool, accountID, "lead bot")
	cleanupProviderEvents(t, pool, channelID)

	// 1. Ingest a message with lead_tracking_enabled = true (default)
	require.NoError(t, svc.IngestProviderEvent(ctx, providerMessage(channelID, "leadcontact1", "lead-msg-1", func(m *messaging.Message) {
		m.Sender.DisplayName = "Lead Contact 1"
		m.Text = "Hi, I want to buy something"
	})))

	// Verify conversation & auto-created lead
	var convoID uuid.UUID
	err = pool.QueryRow(ctx, `SELECT id FROM conversations WHERE account_id = $1 AND last_message_at IS NOT NULL LIMIT 1`, accountID).Scan(&convoID)
	require.NoError(t, err)

	var leadID uuid.UUID
	var currentState string
	err = pool.QueryRow(ctx, `SELECT id, current_state_key FROM leads WHERE conversation_id = $1`, convoID).Scan(&leadID, &currentState)
	require.NoError(t, err)
	assert.Equal(t, "new", currentState, "Auto-created lead must be in the first state ('new')")

	// 2. Ingest message with lead_tracking_enabled = false
	// Update settings
	settingsRaw, _ := json.Marshal(types.AccountSettings{
		LeadTrackingEnabled: newBool(false),
	})
	_, err = pool.Exec(ctx, `UPDATE accounts SET settings = $1 WHERE id = $2`, settingsRaw, accountID)
	require.NoError(t, err)

	require.NoError(t, svc.IngestProviderEvent(ctx, providerMessage(channelID, "leadcontact2", "lead-msg-2", func(m *messaging.Message) {
		m.Sender.DisplayName = "Lead Contact 2"
		m.Text = "Hi, is anyone there?"
	})))

	var convo2ID uuid.UUID
	err = pool.QueryRow(ctx, `
		SELECT c.id FROM conversations c
		JOIN contacts co ON c.contact_id = co.id
		WHERE c.account_id = $1 AND co.external_identity = 'leadcontact2'
	`, accountID).Scan(&convo2ID)
	require.NoError(t, err)

	// Verify NO lead was created
	var count int
	err = pool.QueryRow(ctx, `SELECT COUNT(*) FROM leads WHERE conversation_id = $1`, convo2ID).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count, "No lead should be created when lead_tracking_enabled is false")

	// 3. Manual lead creation (POST /conversations/{id}/lead)
	// Create lead manually for convo2
	lead2, err := svc.CreateLead(ctx, accountID, adminID, convo2ID, "manager")
	require.NoError(t, err)
	require.NotNil(t, lead2)
	assert.Equal(t, "new", lead2.CurrentStateKey)

	// Idempotency: call it again
	lead2Dup, err := svc.CreateLead(ctx, accountID, adminID, convo2ID, "manager")
	require.NoError(t, err)
	assert.Equal(t, lead2.ID, lead2Dup.ID, "Manual lead creation must be idempotent")

	// 4. State Transitions (PATCH /leads/{id}/state)
	// Update state to 'won'
	lead2, err = svc.UpdateLeadState(ctx, accountID, adminID, lead2.ID, "manager", "won")
	require.NoError(t, err)
	assert.Equal(t, "won", lead2.CurrentStateKey)

	// Reject invalid state
	_, err = svc.UpdateLeadState(ctx, accountID, adminID, lead2.ID, "manager", "invalid-state-key")
	assert.Error(t, err, "Invalid state key must be rejected")

	// 5. Visibility check
	// Configure settings to unassigned visible = false
	settingsRaw, _ = json.Marshal(types.AccountSettings{
		UnassignedConversationsVisibleToMembers: newBool(false),
	})
	_, err = pool.Exec(ctx, `UPDATE accounts SET settings = $1 WHERE id = $2`, settingsRaw, accountID)
	require.NoError(t, err)

	// Agent tries to access lead2 — should fail with "lead not found"
	_, err = svc.UpdateLeadState(ctx, accountID, memberID, lead2.ID, "agent", "lost")
	assert.Error(t, err, "Agent should be denied lead access due to visibility")
	assert.Contains(t, err.Error(), "lead not found")

	// Assign agent to convo2
	_, err = pool.Exec(ctx, `UPDATE conversations SET assigned_user_ids = $1 WHERE id = $2`, []uuid.UUID{memberID}, convo2ID)
	require.NoError(t, err)

	// Agent tries again — should succeed
	lead2, err = svc.UpdateLeadState(ctx, accountID, memberID, lead2.ID, "agent", "lost")
	require.NoError(t, err)
	assert.Equal(t, "lost", lead2.CurrentStateKey)

	// 6. Tags and Notes (PATCH /leads/{id}/tags, POST /leads/{id}/notes)
	// Update tags
	lead2, err = svc.UpdateLeadTags(ctx, accountID, memberID, lead2.ID, "agent", []string{"tag1", "tag2"})
	require.NoError(t, err)
	assert.Equal(t, []string{"tag1", "tag2"}, lead2.Tags)

	// Create notes
	note1, err := svc.CreateLeadNote(ctx, accountID, memberID, lead2.ID, "agent", "This is first note")
	require.NoError(t, err)
	assert.Equal(t, "This is first note", note1.Body)

	// List notes
	notes, err := svc.ListLeadNotes(ctx, accountID, memberID, lead2.ID, "agent")
	require.NoError(t, err)
	require.Len(t, notes, 1)
	assert.Equal(t, "This is first note", notes[0].Body)

	// List history
	history, err := svc.ListLeadHistory(ctx, accountID, memberID, lead2.ID, "agent")
	require.NoError(t, err)
	// Historied transitions: null -> new (creation), new -> won, won -> lost
	require.Len(t, history, 3)
	assert.Nil(t, history[0].FromState)
	assert.Equal(t, "new", history[0].ToState)
	assert.Equal(t, "new", *history[1].FromState)
	assert.Equal(t, "won", history[1].ToState)
}

func newBool(v bool) *bool {
	return &v
}

func TestIngestProviderEvent_ExternalOutbound(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	svc, pool, _ := testService(t)
	ctx := context.Background()

	accountID, _ := setupTestTenant(t, pool, "external-outbound-test")

	channelID := newTelegramChannel(t, pool, accountID, "outbound bot")
	cleanupProviderEvents(t, pool, channelID)

	// Create contact and conversation first (so that the conversation exists)
	var contactID uuid.UUID
	err := pool.QueryRow(ctx, `
		INSERT INTO contacts (account_id, channel_id, external_identity, display_name)
		VALUES ($1, $2, 'bob-whatsapp', 'Bob') RETURNING id
	`, accountID, channelID).Scan(&contactID)
	require.NoError(t, err)

	var convoID uuid.UUID
	err = pool.QueryRow(ctx, `
		INSERT INTO conversations (account_id, contact_id, channel_id, last_message_at)
		VALUES ($1, $2, $3, NOW()) RETURNING id
	`, accountID, contactID, channelID).Scan(&convoID)
	require.NoError(t, err)

	// A message the business sent from its own phone/app (no local correlation id).
	event := providerMessage(channelID, "bob-whatsapp", "msg-ext-1", func(m *messaging.Message) { // thread matches co.external_identity
		m.Direction = messaging.DirectionOutbound
		m.Text = "Reply from business phone"
	})

	// A soft review flag set by the AI must be cleared once a human takes over.
	_, err = pool.Exec(ctx, `
		UPDATE conversation_ai_state
		SET review_flag_reason = 'prompt_injection', review_flag_priority = 'normal', review_flagged_at = NOW()
		WHERE conversation_id = $1`, convoID)
	require.NoError(t, err)

	require.NoError(t, svc.IngestProviderEvent(ctx, event))

	var flag *string
	require.NoError(t, pool.QueryRow(ctx, `SELECT review_flag_reason FROM conversation_ai_state WHERE conversation_id = $1`, convoID).Scan(&flag))
	assert.Nil(t, flag, "a human message clears the AI review flag")

	// Verify message in DB
	var direction, senderType string
	var externalMsgID *string
	var contentRaw []byte
	err = pool.QueryRow(ctx, `
		SELECT direction, sender_type, external_message_id, content
		FROM messages
		WHERE conversation_id = $1 AND external_message_id = 'msg-ext-1'
	`, convoID).Scan(&direction, &senderType, &externalMsgID, &contentRaw)
	require.NoError(t, err)

	assert.Equal(t, "outbound", direction)
	assert.Equal(t, "human", senderType)
	require.NotNil(t, externalMsgID)
	assert.Equal(t, "msg-ext-1", *externalMsgID)

	var content map[string]any
	err = json.Unmarshal(contentRaw, &content)
	require.NoError(t, err)
	assert.Equal(t, "Reply from business phone", content["text"])

	// Verify durable AI control moved to human ownership.
	var aiState string
	err = pool.QueryRow(ctx, `SELECT state FROM conversation_ai_state WHERE conversation_id = $1`, convoID).Scan(&aiState)
	require.NoError(t, err)
	assert.Equal(t, "paused_human", aiState)
}

func TestCloseConversation_Workflow(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	svc, pool, _ := testService(t)
	ctx := context.Background()

	accountID, userID := setupTestTenant(t, pool, "Close Convo Account")

	// Create channel and contact
	var channelID uuid.UUID
	err := pool.QueryRow(ctx, `
		INSERT INTO channels (account_id, type, status)
		VALUES ($1, 'whatsapp', 'connected') RETURNING id
	`, accountID).Scan(&channelID)
	require.NoError(t, err)

	var contactID uuid.UUID
	err = pool.QueryRow(ctx, `
		INSERT INTO contacts (account_id, channel_id, external_identity, display_name)
		VALUES ($1, $2, 'alice-close-test', 'Alice') RETURNING id
	`, accountID, channelID).Scan(&contactID)
	require.NoError(t, err)

	// Create a conversation and put it under human control.
	var convoID uuid.UUID
	err = pool.QueryRow(ctx, `
		INSERT INTO conversations (account_id, contact_id, channel_id, status, last_message_at)
		VALUES ($1, $2, $3, 'open', NOW()) RETURNING id
	`, accountID, contactID, channelID).Scan(&convoID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `UPDATE conversation_ai_state SET state = 'paused_human' WHERE conversation_id = $1`, convoID)
	require.NoError(t, err)

	// Close the conversation
	err = svc.CloseConversation(ctx, accountID, userID, convoID, "manager")
	require.NoError(t, err)

	// Verify closing resets durable AI control.
	var status, aiState string
	err = pool.QueryRow(ctx, `
		SELECT c.status, state.state
		FROM conversations c JOIN conversation_ai_state state ON state.conversation_id = c.id
		WHERE c.id = $1
	`, convoID).Scan(&status, &aiState)
	require.NoError(t, err)
	assert.Equal(t, "closed", status)
	assert.Equal(t, "active", aiState)

	// Verify audit log
	var auditCount int
	err = pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM audit_logs
		WHERE account_id = $1 AND action = 'conversation.closed' AND target_id = $2
	`, accountID, convoID).Scan(&auditCount)
	require.NoError(t, err)
	assert.Equal(t, 1, auditCount)
}

func TestConcurrentIngestProviderEvent_NoUniqueViolation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	svc, pool, _ := testService(t)
	ctx := context.Background()

	accountID, _ := setupTestTenant(t, pool, "concurrent-ingest")

	channelID := newTelegramChannel(t, pool, accountID, "concurrent bot")
	cleanupProviderEvents(t, pool, channelID)

	const concurrency = 10
	errCh := make(chan error, concurrency)
	var wg sync.WaitGroup

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		msgID := fmt.Sprintf("concurrent-msg-%d", i)
		go func(mid string) {
			defer wg.Done()
			errCh <- svc.IngestProviderEvent(ctx, providerMessage(channelID, "concurrent-contact", mid, func(m *messaging.Message) {
				m.Sender.DisplayName = "Concurrent User"
				m.Text = "Concurrent message: " + mid
			}))
		}(msgID)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		require.NoError(t, err)
	}

	var convoCount int
	err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM conversations c
		JOIN contacts co ON c.contact_id = co.id
		WHERE co.channel_id = $1 AND co.external_identity = 'concurrent-contact'
	`, channelID).Scan(&convoCount)
	require.NoError(t, err)
	assert.Equal(t, 1, convoCount)
}

func TestConcurrentUpdateLeadState_SequentialHistory(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	svc, pool, _ := testService(t)
	ctx := context.Background()

	accountID, adminID := setupTestTenant(t, pool, "concurrent-lead-state")

	var channelID uuid.UUID
	err := pool.QueryRow(ctx, `
		INSERT INTO channels (account_id, type, status)
		VALUES ($1, 'whatsapp', 'connected') RETURNING id
	`, accountID).Scan(&channelID)
	require.NoError(t, err)

	var contactID uuid.UUID
	err = pool.QueryRow(ctx, `
		INSERT INTO contacts (account_id, channel_id, external_identity, display_name)
		VALUES ($1, $2, 'concurrent-lead-user', 'Lead User') RETURNING id
	`, accountID, channelID).Scan(&contactID)
	require.NoError(t, err)

	var convoID uuid.UUID
	err = pool.QueryRow(ctx, `
		INSERT INTO conversations (account_id, contact_id, channel_id, status)
		VALUES ($1, $2, $3, 'open') RETURNING id
	`, accountID, contactID, channelID).Scan(&convoID)
	require.NoError(t, err)

	lead, err := svc.CreateLead(ctx, accountID, adminID, convoID, "manager")
	require.NoError(t, err)

	// Run two concurrent transitions: one to "won", one to "lost"
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = svc.UpdateLeadState(ctx, accountID, adminID, lead.ID, "manager", "won")
	}()
	go func() {
		defer wg.Done()
		_, _ = svc.UpdateLeadState(ctx, accountID, adminID, lead.ID, "manager", "lost")
	}()
	wg.Wait()

	// Check history: should have 3 history rows (initial NULL->new, then state1, then state2).
	rows, err := pool.Query(ctx, `
		SELECT from_state, to_state
		FROM lead_state_history
		WHERE lead_id = $1
		ORDER BY changed_at ASC, id ASC
	`, lead.ID)
	require.NoError(t, err)
	defer rows.Close()

	type transition struct {
		from *string
		to   string
	}
	var history []transition
	for rows.Next() {
		var tr transition
		err := rows.Scan(&tr.from, &tr.to)
		require.NoError(t, err)
		history = append(history, tr)
	}

	require.Equal(t, 3, len(history), "Expected 3 state history entries (initial + 2 updates)")
	// Second entry must be from "new" to (won or lost)
	require.NotNil(t, history[1].from)
	assert.Equal(t, "new", *history[1].from)
	// Third entry must be from history[1].to to history[2].to, NOT from "new"!
	require.NotNil(t, history[2].from)
	assert.Equal(t, history[1].to, *history[2].from, "Third history entry's from_state must match second entry's to_state")
}

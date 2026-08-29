package service_test

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	fakeadapter "github.com/whatfunnel/whatfunnel/adapters/fake"
	"github.com/whatfunnel/whatfunnel/packages/go-common/crypto"
	"github.com/whatfunnel/whatfunnel/packages/go-common/db"
	"github.com/whatfunnel/whatfunnel/packages/go-common/pubsub"
	"github.com/whatfunnel/whatfunnel/packages/go-common/types"
	"github.com/whatfunnel/whatfunnel/services/conversation-svc/internal/service"
)

const testEncryptionKey = "test-key-exactly-32-bytes-padded"

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

	cipher, err := crypto.NewCipherFromHex(testEncryptionKey)
	require.NoError(t, err)

	svc := service.New(pool, cipher, ps)
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

func TestIngestInbound(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	svc, pool, ps := testService(t)
	ctx := context.Background()

	accountID, _ := setupTestTenant(t, pool, "ingest-test")

	// Create a channel
	var channelID uuid.UUID
	err := pool.QueryRow(ctx, `
		INSERT INTO channels (account_id, type, status)
		VALUES ($1, 'matrix_whatsapp', 'connected') RETURNING id
	`, accountID).Scan(&channelID)
	require.NoError(t, err)

	// Clean up Redis stream
	ps.RawClient().Del(ctx, "conversation.updated")

	// 1. Ingest a message for a new contact (new contact + new conversation)
	event1 := types.InboundEvent{
		ChannelID:        channelID.String(),
		ExternalThreadID: "whatsapp-thread-1",
		Contact: types.ContactRef{
			ExternalIdentity: "12345@s.whatsapp.net",
			DisplayName:      "Bob",
			AvatarURL:        "http://avatar.url/bob",
		},
		Message: types.NormalizedMessage{
			ContentType:       "text",
			Text:              "Hello business!",
			ExternalMessageID: "msg-id-1",
		},
		Timestamp: time.Now(),
	}

	err = svc.IngestInbound(ctx, event1)
	require.NoError(t, err)

	// Verify DB state
	var contactID uuid.UUID
	var displayName, avatarURL string
	err = pool.QueryRow(ctx, `SELECT id, display_name, avatar_url FROM contacts WHERE channel_id = $1 AND external_identity = $2`, channelID, "12345@s.whatsapp.net").
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

	// Check Redis event published
	streams, err := ps.RawClient().XRead(ctx, &redis.XReadArgs{
		Streams: []string{"conversation.updated", "0"},
		Count:   1,
	}).Result()
	require.NoError(t, err)
	require.Len(t, streams, 1)
	require.Len(t, streams[0].Messages, 1)

	// 2. Ingest duplicate message (idempotency check)
	err = svc.IngestInbound(ctx, event1)
	require.NoError(t, err) // Should skip without failing

	err = pool.QueryRow(ctx, `SELECT count(*) FROM messages WHERE conversation_id = $1`, convoID).Scan(&msgCount)
	require.NoError(t, err)
	assert.Equal(t, 1, msgCount, "message count should still be 1 (deduplicated)")

	// 3. Second message from same contact (reuses conversation)
	event2 := types.InboundEvent{
		ChannelID:        channelID.String(),
		ExternalThreadID: "whatsapp-thread-1",
		Contact: types.ContactRef{
			ExternalIdentity: "12345@s.whatsapp.net",
			DisplayName:      "Bob Modified",
			AvatarURL:        "", // empty should not overwrite
		},
		Message: types.NormalizedMessage{
			ContentType:       "image",
			Text:              "Look at this!",
			MediaURL:          "http://media.url/image.jpg",
			ExternalMessageID: "msg-id-2",
		},
		Timestamp: time.Now(),
	}

	err = svc.IngestInbound(ctx, event2)
	require.NoError(t, err)

	err = pool.QueryRow(ctx, `SELECT count(*) FROM messages WHERE conversation_id = $1`, convoID).Scan(&msgCount)
	require.NoError(t, err)
	assert.Equal(t, 2, msgCount)

	// Display name updated, avatar retained
	err = pool.QueryRow(ctx, `SELECT display_name, avatar_url FROM contacts WHERE id = $1`, contactID).
		Scan(&displayName, &avatarURL)
	require.NoError(t, err)
	assert.Equal(t, "Bob Modified", displayName)
	assert.Equal(t, "http://avatar.url/bob", avatarURL)

	// 4. Test all content types normalize correctly
	contentTypes := []string{"text", "image", "video", "audio", "document", "reaction", "location", "contact"}
	for i, ct := range contentTypes {
		event := types.InboundEvent{
			ChannelID:        channelID.String(),
			ExternalThreadID: "whatsapp-thread-1",
			Contact: types.ContactRef{
				ExternalIdentity: "12345@s.whatsapp.net",
			},
			Message: types.NormalizedMessage{
				ContentType:       ct,
				Text:              "test " + ct,
				ExternalMessageID: "msg-ct-" + ct,
			},
			Timestamp: time.Now().Add(time.Duration(i) * time.Second),
		}
		err = svc.IngestInbound(ctx, event)
		require.NoError(t, err)
	}

	// Verify count is 2 + 8 = 10
	err = pool.QueryRow(ctx, `SELECT count(*) FROM messages WHERE conversation_id = $1`, convoID).Scan(&msgCount)
	require.NoError(t, err)
	assert.Equal(t, 10, msgCount)
}

func TestSendMessage(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	svc, pool, _ := testService(t)
	ctx := context.Background()

	accountID, userID := setupTestTenant(t, pool, "send-test")

	// Create a channel
	var channelID uuid.UUID
	err := pool.QueryRow(ctx, `
		INSERT INTO channels (account_id, type, status)
		VALUES ($1, 'matrix_whatsapp', 'connected') RETURNING id
	`, accountID).Scan(&channelID)
	require.NoError(t, err)

	// Register fake adapter for WhatsApp
	fakeAdapter := fakeadapter.New()
	svc.RegisterAdapter("matrix_whatsapp", fakeAdapter)

	// Create a contact
	var contactID uuid.UUID
	err = pool.QueryRow(ctx, `
		INSERT INTO contacts (account_id, channel_id, external_identity, display_name)
		VALUES ($1, $2, '98765@s.whatsapp.net', 'Alice') RETURNING id
	`, accountID, channelID).Scan(&contactID)
	require.NoError(t, err)

	// Create a conversation
	var convoID uuid.UUID
	err = pool.QueryRow(ctx, `
		INSERT INTO conversations (account_id, contact_id, channel_id)
		VALUES ($1, $2, $3) RETURNING id
	`, accountID, contactID, channelID).Scan(&convoID)
	require.NoError(t, err)

	var sourceMessageID uuid.UUID
	err = pool.QueryRow(ctx, `
		INSERT INTO messages (account_id, conversation_id, direction, sender_type, content_type, content)
		VALUES ($1, $2, 'inbound', 'contact', 'text', '{"text":"Hello"}')
		RETURNING id
	`, accountID, convoID).Scan(&sourceMessageID)
	require.NoError(t, err)

	var draftID uuid.UUID
	err = pool.QueryRow(ctx, `
		INSERT INTO ai_reply_drafts (
			account_id, conversation_id, source_message_id, draft_text, stage_matched, confidence
		) VALUES ($1, $2, $3, 'Hello Alice from agent', 'pattern', 1.0)
		RETURNING id
	`, accountID, convoID, sourceMessageID).Scan(&draftID)
	require.NoError(t, err)

	// Send outbound message
	msg, err := svc.SendMessage(ctx, accountID, convoID, "human", &userID, "text", "Hello Alice from agent", "", &draftID)
	require.NoError(t, err)
	require.NotNil(t, msg)

	// Assert message stored in DB
	var dbDirection, dbSenderType, dbContentType string
	err = pool.QueryRow(ctx, `SELECT direction, sender_type, content_type FROM messages WHERE id = $1`, msg.ID).
		Scan(&dbDirection, &dbSenderType, &dbContentType)
	require.NoError(t, err)
	assert.Equal(t, "outbound", dbDirection)
	assert.Equal(t, "human", dbSenderType)
	assert.Equal(t, "text", dbContentType)

	var draftStatus string
	var usedMessageID uuid.UUID
	err = pool.QueryRow(ctx, `SELECT status, used_message_id FROM ai_reply_drafts WHERE id = $1`, draftID).
		Scan(&draftStatus, &usedMessageID)
	require.NoError(t, err)
	assert.Equal(t, "used", draftStatus)
	assert.Equal(t, msg.ID, usedMessageID)

	// Assert adapter SendMessage was called
	sent := fakeAdapter.GetSentMessages()
	require.Len(t, sent, 1)
	assert.Equal(t, channelID.String(), sent[0].ChannelID)
	assert.Equal(t, "98765@s.whatsapp.net", sent[0].ExternalThreadID)
	assert.Equal(t, "Hello Alice from agent", sent[0].Message.Text)
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
	err := pool.QueryRow(ctx, `INSERT INTO users (account_id, email, password_hash, role) VALUES ($1, 'member@example.com', 'hash', 'agent') RETURNING id`, accountID).Scan(&memberID)
	require.NoError(t, err)

	// Create a channel
	var channelID uuid.UUID
	err = pool.QueryRow(ctx, `INSERT INTO channels (account_id, type, status) VALUES ($1, 'matrix_whatsapp', 'connected') RETURNING id`, accountID).Scan(&channelID)
	require.NoError(t, err)

	// 1. Ingest a message with lead_tracking_enabled = true (default)
	event1 := types.InboundEvent{
		ChannelID:        channelID.String(),
		ExternalThreadID: "thread-lead-1",
		Contact: types.ContactRef{
			ExternalIdentity: "leadcontact1@s.whatsapp.net",
			DisplayName:      "Lead Contact 1",
		},
		Message: types.NormalizedMessage{
			ContentType:       "text",
			Text:              "Hi, I want to buy something",
			ExternalMessageID: "lead-msg-1",
		},
		Timestamp: time.Now(),
	}

	err = svc.IngestInbound(ctx, event1)
	require.NoError(t, err)

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

	event2 := types.InboundEvent{
		ChannelID:        channelID.String(),
		ExternalThreadID: "thread-lead-2",
		Contact: types.ContactRef{
			ExternalIdentity: "leadcontact2@s.whatsapp.net",
			DisplayName:      "Lead Contact 2",
		},
		Message: types.NormalizedMessage{
			ContentType:       "text",
			Text:              "Hi, is anyone there?",
			ExternalMessageID: "lead-msg-2",
		},
		Timestamp: time.Now(),
	}

	err = svc.IngestInbound(ctx, event2)
	require.NoError(t, err)

	var convo2ID uuid.UUID
	err = pool.QueryRow(ctx, `
		SELECT c.id FROM conversations c
		JOIN contacts co ON c.contact_id = co.id
		WHERE c.account_id = $1 AND co.external_identity = 'leadcontact2@s.whatsapp.net'
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

func TestIngestExternalOutbound(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	svc, pool, _ := testService(t)
	ctx := context.Background()

	accountID, _ := setupTestTenant(t, pool, "external-outbound-test")

	// Create channel
	var channelID uuid.UUID
	err := pool.QueryRow(ctx, `
		INSERT INTO channels (account_id, type, status)
		VALUES ($1, 'matrix_whatsapp', 'connected') RETURNING id
	`, accountID).Scan(&channelID)
	require.NoError(t, err)

	// Create contact and conversation first (so that the conversation exists)
	var contactID uuid.UUID
	err = pool.QueryRow(ctx, `
		INSERT INTO contacts (account_id, channel_id, external_identity, display_name)
		VALUES ($1, $2, 'bob-whatsapp', 'Bob') RETURNING id
	`, accountID, channelID).Scan(&contactID)
	require.NoError(t, err)

	var convoID uuid.UUID
	err = pool.QueryRow(ctx, `
		INSERT INTO conversations (account_id, contact_id, channel_id, last_message_at, ai_mode_active)
		VALUES ($1, $2, $3, NOW(), true) RETURNING id
	`, accountID, contactID, channelID).Scan(&convoID)
	require.NoError(t, err)

	// Ingest external outbound message
	event := types.ExternalOutboundEvent{
		ChannelID:        channelID.String(),
		ExternalThreadID: "bob-whatsapp", // matches co.external_identity
		Message: types.NormalizedMessage{
			ContentType: "text",
			Text:        "Reply from business phone",
		},
		ExternalMessageID: "msg-ext-1",
		Timestamp:         time.Now(),
	}

	err = svc.IngestExternalOutbound(ctx, event)
	require.NoError(t, err)

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
	assert.Equal(t, true, content["external_origin"])

	// Verify conversation updated: ai_mode_active = false
	var aiModeActive bool
	err = pool.QueryRow(ctx, `SELECT ai_mode_active FROM conversations WHERE id = $1`, convoID).Scan(&aiModeActive)
	require.NoError(t, err)
	assert.False(t, aiModeActive)
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
		VALUES ($1, 'matrix_whatsapp', 'connected') RETURNING id
	`, accountID).Scan(&channelID)
	require.NoError(t, err)

	var contactID uuid.UUID
	err = pool.QueryRow(ctx, `
		INSERT INTO contacts (account_id, channel_id, external_identity, display_name)
		VALUES ($1, $2, 'alice-close-test', 'Alice') RETURNING id
	`, accountID, channelID).Scan(&contactID)
	require.NoError(t, err)

	// Create conversation that was taken over by human (ai_mode_active = false, status = 'open')
	var convoID uuid.UUID
	err = pool.QueryRow(ctx, `
		INSERT INTO conversations (account_id, contact_id, channel_id, status, last_message_at, ai_mode_active)
		VALUES ($1, $2, $3, 'open', NOW(), false) RETURNING id
	`, accountID, contactID, channelID).Scan(&convoID)
	require.NoError(t, err)

	// Close the conversation
	err = svc.CloseConversation(ctx, accountID, userID, convoID, "manager")
	require.NoError(t, err)

	// Verify conversation updated in DB: status = 'closed', ai_mode_active = true
	var status string
	var aiModeActive bool
	err = pool.QueryRow(ctx, `SELECT status, ai_mode_active FROM conversations WHERE id = $1`, convoID).Scan(&status, &aiModeActive)
	require.NoError(t, err)
	assert.Equal(t, "closed", status)
	assert.True(t, aiModeActive, "ai_mode_active must be reset to true upon closing conversation")

	// Verify audit log
	var auditCount int
	err = pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM audit_logs
		WHERE account_id = $1 AND action = 'conversation.closed' AND target_id = $2
	`, accountID, convoID).Scan(&auditCount)
	require.NoError(t, err)
	assert.Equal(t, 1, auditCount)
}

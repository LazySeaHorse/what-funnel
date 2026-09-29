package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/whatfunnel/whatfunnel/packages/go-common/messaging"
	"github.com/whatfunnel/whatfunnel/packages/go-common/types"
	"github.com/whatfunnel/whatfunnel/services/conversation-svc/internal/service"
)

// newTelegramChannel creates a connected Telegram channel with full
// capabilities for the tenant.
func newTelegramChannel(t *testing.T, pool *pgxpool.Pool, accountID uuid.UUID, label string) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	capabilities, err := json.Marshal(messaging.Capabilities{Replies: true, Reactions: true, Edits: true, Deletes: true, Media: true})
	require.NoError(t, err)
	var channelID uuid.UUID
	require.NoError(t, pool.QueryRow(ctx, `
		INSERT INTO channels (account_id, type, provider, label, status, capabilities)
		VALUES ($1, 'telegram', 'telegram', $2, 'connected', $3) RETURNING id
	`, accountID, label, capabilities).Scan(&channelID))
	_, err = pool.Exec(ctx, `INSERT INTO provider_connections (channel_id, account_id, provider, state) VALUES ($1, $2, 'telegram', 'connected')`, channelID, accountID)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM message_outbox WHERE account_id = $1`, accountID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM media_objects WHERE account_id = $1`, accountID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM provider_connections WHERE account_id = $1`, accountID)
	})
	return channelID
}

// newConversation creates a contact and conversation on the channel.
func newConversation(t *testing.T, pool *pgxpool.Pool, accountID, channelID uuid.UUID, thread string, assigned ...uuid.UUID) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	if assigned == nil {
		assigned = []uuid.UUID{}
	}
	var contactID, conversationID uuid.UUID
	require.NoError(t, pool.QueryRow(ctx, `
		INSERT INTO contacts (account_id, channel_id, external_identity, display_name)
		VALUES ($1, $2, $3, $3) RETURNING id
	`, accountID, channelID, thread).Scan(&contactID))
	require.NoError(t, pool.QueryRow(ctx, `
		INSERT INTO conversations (account_id, contact_id, channel_id, external_thread_id, assigned_user_ids)
		VALUES ($1, $2, $3, $4, $5) RETURNING id
	`, accountID, contactID, channelID, thread, assigned).Scan(&conversationID))
	return conversationID
}

func TestSendMessage_IdempotencyKeyIsScopedToConversation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	svc, pool, _ := testService(t)
	ctx := context.Background()
	accountID, userID := setupTestTenant(t, pool, "idem-scope")
	channelID := newTelegramChannel(t, pool, accountID, "idem bot")
	convoA := newConversation(t, pool, accountID, channelID, "a")
	convoB := newConversation(t, pool, accountID, channelID, "b")

	send := func(conversationID uuid.UUID) (*types.Message, error) {
		return svc.SendMessage(ctx, service.SendMessageParams{
			AccountID: accountID, ConversationID: conversationID,
			Sender: types.MessageSenderHuman, SenderUserID: &userID,
			ContentType: "text", Text: "hello", IdempotencyKey: "same-key",
		})
	}

	first, err := send(convoA)
	require.NoError(t, err)

	// Same key, same conversation: the original message is returned.
	again, err := send(convoA)
	require.NoError(t, err)
	require.Equal(t, first.ID, again.ID)

	// Same key, different conversation: must not leak convoA's message.
	leaked, err := send(convoB)
	require.Error(t, err)
	require.Nil(t, leaked)
	require.True(t, errors.Is(err, service.ErrConflict), "got %v", err)
}

func providerCreatedEvent(channelID uuid.UUID, eventID, providerMessageID, thread string, at time.Time) messaging.Event {
	return messaging.Event{
		SchemaVersion: messaging.SchemaVersion,
		ID:            eventID,
		Kind:          messaging.EventMessageCreated,
		Provider:      messaging.ProviderTelegram,
		ChannelID:     channelID.String(),
		OccurredAt:    at,
		Message: &messaging.Message{
			ProviderMessageID: providerMessageID,
			ExternalThreadID:  thread,
			Direction:         messaging.DirectionInbound,
			Sender:            messaging.Sender{ExternalID: thread, DisplayName: "Customer"},
			ContentType:       messaging.ContentText,
			Text:              "hi",
			ProviderTimestamp: at,
		},
	}
}

func cleanupProviderEvents(t *testing.T, pool *pgxpool.Pool, channelID uuid.UUID) {
	t.Helper()
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM processed_adapter_events WHERE channel_id = $1`, channelID)
	})
}

func TestIngestProviderEvent_DuplicateProviderMessageIDWithNewEventIDIsSkipped(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	svc, pool, _ := testService(t)
	ctx := context.Background()
	accountID, _ := setupTestTenant(t, pool, "dup-provider-msg")
	channelID := newTelegramChannel(t, pool, accountID, "dup bot")
	cleanupProviderEvents(t, pool, channelID)

	now := time.Now().UTC().Truncate(time.Microsecond)
	require.NoError(t, svc.IngestProviderEvent(ctx, providerCreatedEvent(channelID, "evt-"+uuid.NewString(), "900", "cust-1", now)))
	// Same provider message, brand new event ID: previously a unique
	// violation that rolled back and was redelivered until the DLQ.
	require.NoError(t, svc.IngestProviderEvent(ctx, providerCreatedEvent(channelID, "evt-"+uuid.NewString(), "900", "cust-1", now)))

	var count int
	require.NoError(t, pool.QueryRow(ctx, `SELECT COUNT(*) FROM messages WHERE account_id = $1 AND provider_message_id = '900'`, accountID).Scan(&count))
	require.Equal(t, 1, count)
}

func TestIngestProviderEvent_InboundMessageReopensClosedConversation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	svc, pool, _ := testService(t)
	ctx := context.Background()
	accountID, userID := setupTestTenant(t, pool, "reopen-convo")
	channelID := newTelegramChannel(t, pool, accountID, "reopen bot")
	cleanupProviderEvents(t, pool, channelID)

	now := time.Now().UTC().Truncate(time.Microsecond)
	require.NoError(t, svc.IngestProviderEvent(ctx, providerCreatedEvent(channelID, "evt-"+uuid.NewString(), "1", "cust-2", now)))
	var conversationID uuid.UUID
	require.NoError(t, pool.QueryRow(ctx, `SELECT id FROM conversations WHERE account_id = $1`, accountID).Scan(&conversationID))
	require.NoError(t, svc.CloseConversation(ctx, accountID, userID, conversationID, types.RoleManager))

	var status string
	require.NoError(t, pool.QueryRow(ctx, `SELECT status FROM conversations WHERE id = $1`, conversationID).Scan(&status))
	require.Equal(t, "closed", status)

	require.NoError(t, svc.IngestProviderEvent(ctx, providerCreatedEvent(channelID, "evt-"+uuid.NewString(), "2", "cust-2", now.Add(time.Second))))
	require.NoError(t, pool.QueryRow(ctx, `SELECT status FROM conversations WHERE id = $1`, conversationID).Scan(&status))
	require.Equal(t, "open", status)
}

func TestIngestProviderEvent_MissingTargetIsRetryable(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	svc, pool, _ := testService(t)
	ctx := context.Background()
	accountID, _ := setupTestTenant(t, pool, "missing-target")
	channelID := newTelegramChannel(t, pool, accountID, "missing bot")
	cleanupProviderEvents(t, pool, channelID)

	now := time.Now().UTC().Truncate(time.Microsecond)
	base := func(kind messaging.EventKind) messaging.Event {
		return messaging.Event{
			SchemaVersion: messaging.SchemaVersion, ID: "evt-" + uuid.NewString(), Kind: kind,
			Provider: messaging.ProviderTelegram, ChannelID: channelID.String(), OccurredAt: now,
		}
	}
	edit := base(messaging.EventMessageEdited)
	edit.Message = &messaging.Message{ProviderMessageID: "404", ExternalThreadID: "x", Direction: messaging.DirectionInbound, ContentType: messaging.ContentText, Text: "edited", ProviderTimestamp: now, Sender: messaging.Sender{ExternalID: "x"}}
	del := base(messaging.EventMessageDeleted)
	del.Message = &messaging.Message{ProviderMessageID: "404", ExternalThreadID: "x", ProviderTimestamp: now}
	reaction := base(messaging.EventReactionChanged)
	reaction.Reaction = &messaging.Reaction{ProviderMessageID: "404", SenderExternalID: "x", Emoji: "👍", ProviderTimestamp: now}
	receipt := base(messaging.EventReceiptChanged)
	receipt.Receipt = &messaging.Receipt{ProviderMessageID: "404", Status: messaging.ReceiptDelivered, ProviderTimestamp: now}

	for name, event := range map[string]messaging.Event{"edit": edit, "delete": del, "reaction": reaction, "receipt": receipt} {
		require.NoError(t, event.Validate(), name)
		err := svc.IngestProviderEvent(ctx, event)
		require.Error(t, err, name)
		require.True(t, errors.Is(err, service.ErrProviderTargetNotFound), "%s: %v", name, err)
		require.False(t, service.IsTerminalIngestError(err), name)

		// The marker rolled back with the transaction, so a redelivery
		// after the target arrives can still be applied.
		var processed int
		require.NoError(t, pool.QueryRow(ctx, `SELECT COUNT(*) FROM processed_adapter_events WHERE event_id = $1`, event.ID).Scan(&processed))
		require.Equal(t, 0, processed, name)
	}

	// Once the target exists, the redelivered event applies.
	require.NoError(t, svc.IngestProviderEvent(ctx, providerCreatedEvent(channelID, "evt-"+uuid.NewString(), "404", "x", now)))
	require.NoError(t, svc.IngestProviderEvent(ctx, receipt))
}

func TestBlockedAIStateSurvivesCloseAndHumanReply(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	svc, pool, _ := testService(t)
	ctx := context.Background()
	accountID, managerID := setupTestTenant(t, pool, "blocked-ai-state")
	var agentID uuid.UUID
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO users (account_id, email, password_hash, role) VALUES ($1, 'blocked-agent@example.com', 'hash', 'agent') RETURNING id`, accountID).Scan(&agentID))
	channelID := newTelegramChannel(t, pool, accountID, "blocked bot")

	aiState := func(conversationID uuid.UUID) (string, bool) {
		var state string
		var blockedAt *time.Time
		require.NoError(t, pool.QueryRow(ctx, `SELECT state, blocked_at FROM conversation_ai_state WHERE conversation_id = $1`, conversationID).Scan(&state, &blockedAt))
		return state, blockedAt != nil
	}

	for _, blocked := range []string{"blocked_spam", "blocked_manual"} {
		t.Run(blocked+" closed by agent", func(t *testing.T) {
			convo := newConversation(t, pool, accountID, channelID, "close-"+blocked, agentID)
			_, err := pool.Exec(ctx, `UPDATE conversation_ai_state SET state = $2, blocked_at = NOW() WHERE conversation_id = $1`, convo, blocked)
			require.NoError(t, err)

			require.NoError(t, svc.CloseConversation(ctx, accountID, agentID, convo, types.RoleAgent))
			state, hasBlockedAt := aiState(convo)
			require.Equal(t, blocked, state)
			require.True(t, hasBlockedAt)
		})

		t.Run(blocked+" human reply", func(t *testing.T) {
			convo := newConversation(t, pool, accountID, channelID, "reply-"+blocked, agentID)
			_, err := pool.Exec(ctx, `UPDATE conversation_ai_state SET state = $2, blocked_at = NOW() WHERE conversation_id = $1`, convo, blocked)
			require.NoError(t, err)

			_, err = svc.SendMessage(ctx, service.SendMessageParams{
				AccountID: accountID, ConversationID: convo, Sender: types.MessageSenderHuman,
				SenderUserID: &agentID, ContentType: "text", Text: "hello",
			})
			require.NoError(t, err)
			state, hasBlockedAt := aiState(convo)
			require.Equal(t, blocked, state)
			require.True(t, hasBlockedAt)
		})
	}

	t.Run("non-blocked state is still reset on close", func(t *testing.T) {
		convo := newConversation(t, pool, accountID, channelID, "close-paused")
		_, err := pool.Exec(ctx, `UPDATE conversation_ai_state SET state = 'paused_human' WHERE conversation_id = $1`, convo)
		require.NoError(t, err)
		require.NoError(t, svc.CloseConversation(ctx, accountID, managerID, convo, types.RoleManager))
		state, _ := aiState(convo)
		require.Equal(t, "active", state)
	})
}

func TestMediaVisibilityFollowsConversation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	svc, pool, _ := testService(t)
	ctx := context.Background()
	require.NoError(t, svc.ConfigureMediaCache(t.TempDir()))
	accountID, managerID := setupTestTenant(t, pool, "media-visibility")
	var agent1, agent2 uuid.UUID
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO users (account_id, email, password_hash, role) VALUES ($1, 'media-a1@example.com', 'hash', 'agent') RETURNING id`, accountID).Scan(&agent1))
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO users (account_id, email, password_hash, role) VALUES ($1, 'media-a2@example.com', 'hash', 'agent') RETURNING id`, accountID).Scan(&agent2))
	channelID := newTelegramChannel(t, pool, accountID, "media bot")
	convo := newConversation(t, pool, accountID, channelID, "media-thread", agent1)

	viewer := func(user uuid.UUID, role string) service.MediaViewer {
		return service.MediaViewer{AccountID: accountID, UserID: user, Role: role}
	}

	// A user who cannot see the conversation cannot upload into it.
	_, err := svc.SaveOutboundMedia(ctx, viewer(agent2, types.RoleAgent), convo, "a.png", "image/png", strings.NewReader("png-bytes"))
	require.True(t, errors.Is(err, service.ErrNotFound), "got %v", err)

	media, err := svc.SaveOutboundMedia(ctx, viewer(agent1, types.RoleAgent), convo, "a.png", "image/png", strings.NewReader("png-bytes"))
	require.NoError(t, err)

	open := func(v service.MediaViewer, id uuid.UUID) error {
		content, err := svc.OpenMedia(ctx, &v, id)
		if err == nil {
			_ = content.Reader.Close()
		}
		return err
	}
	require.NoError(t, open(viewer(agent1, types.RoleAgent), media.ID))
	require.NoError(t, open(viewer(managerID, types.RoleManager), media.ID))
	err = open(viewer(agent2, types.RoleAgent), media.ID)
	require.True(t, errors.Is(err, service.ErrNotFound), "got %v", err)

	// Another account never sees it either.
	otherAccount, otherManager := setupTestTenant(t, pool, "media-visibility-other")
	err = open(service.MediaViewer{AccountID: otherAccount, UserID: otherManager, Role: types.RoleManager}, media.ID)
	require.True(t, errors.Is(err, service.ErrNotFound), "got %v", err)

	// Media not tied to any conversation: uploader or manager only.
	stored, err := svc.SaveOutboundMedia(ctx, viewer(managerID, types.RoleManager), convo, "b.png", "image/png", strings.NewReader("xyz"))
	require.NoError(t, err)
	// Reuse the stored blob for an upload that never got tied to a conversation.
	orphanID := uuid.New()
	_, err = pool.Exec(ctx, `
		INSERT INTO media_objects (id, account_id, channel_id, uploaded_by_user_id, mime_type, size_bytes, storage_key, expires_at)
		VALUES ($1, $2, $3, $4, 'image/png', 3, $5, NOW() + INTERVAL '1 day')
	`, orphanID, accountID, channelID, agent1, stored.ID.String())
	require.NoError(t, err)
	require.NoError(t, open(viewer(agent1, types.RoleAgent), orphanID))
	require.NoError(t, open(viewer(managerID, types.RoleManager), orphanID))
	err = open(viewer(agent2, types.RoleAgent), orphanID)
	require.True(t, errors.Is(err, service.ErrNotFound), "got %v", err)
}

func TestAssignConversation_ValidatesConversationAndAssignees(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	svc, pool, _ := testService(t)
	ctx := context.Background()
	accountID, managerID := setupTestTenant(t, pool, "assign-validate")
	otherAccountID, otherUserID := setupTestTenant(t, pool, "assign-validate-other")
	channelID := newTelegramChannel(t, pool, accountID, "assign bot")
	convo := newConversation(t, pool, accountID, channelID, "assign-thread")

	// Unknown conversation: 404, and no audit trail for a no-op.
	missing := uuid.New()
	err := svc.AssignConversation(ctx, accountID, missing, []uuid.UUID{managerID}, managerID)
	require.True(t, errors.Is(err, service.ErrNotFound), "got %v", err)
	var audits int
	require.NoError(t, pool.QueryRow(ctx, `SELECT COUNT(*) FROM audit_logs WHERE account_id = $1 AND action = 'conversation.assigned'`, accountID).Scan(&audits))
	require.Equal(t, 0, audits)

	// A conversation from another account is equally not found.
	err = svc.AssignConversation(ctx, otherAccountID, convo, []uuid.UUID{otherUserID}, otherUserID)
	require.True(t, errors.Is(err, service.ErrNotFound), "got %v", err)

	// Assignees must belong to the account.
	err = svc.AssignConversation(ctx, accountID, convo, []uuid.UUID{otherUserID}, managerID)
	require.True(t, errors.Is(err, service.ErrValidation), "got %v", err)
	err = svc.AssignConversation(ctx, accountID, convo, []uuid.UUID{uuid.New()}, managerID)
	require.True(t, errors.Is(err, service.ErrValidation), "got %v", err)

	// Valid assignment (duplicates collapse) succeeds.
	require.NoError(t, svc.AssignConversation(ctx, accountID, convo, []uuid.UUID{managerID, managerID}, managerID))
	var assigned []uuid.UUID
	require.NoError(t, pool.QueryRow(ctx, `SELECT assigned_user_ids FROM conversations WHERE id = $1`, convo).Scan(&assigned))
	require.Equal(t, []uuid.UUID{managerID}, assigned)
}

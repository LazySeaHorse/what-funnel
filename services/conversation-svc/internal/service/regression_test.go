package service_test

import (
	"context"
	"encoding/json"
	"errors"
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

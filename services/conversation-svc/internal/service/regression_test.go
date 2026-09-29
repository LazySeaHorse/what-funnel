package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

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

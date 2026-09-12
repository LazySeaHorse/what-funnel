package service_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/whatfunnel/whatfunnel/packages/go-common/messaging"
)

func TestProviderMessageMutationsShareTheTransactionalOutbox(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	svc, pool, _ := testService(t)
	ctx := context.Background()
	accountID, userID := setupTestTenant(t, pool, "provider-message-commands")
	capabilities, err := json.Marshal(messaging.Capabilities{Replies: true, Reactions: true, Edits: true, Deletes: true})
	require.NoError(t, err)
	var channelID uuid.UUID
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO channels (account_id, type, provider, label, status, capabilities) VALUES ($1, 'telegram', 'telegram', 'Test bot', 'connected', $2) RETURNING id`, accountID, capabilities).Scan(&channelID))
	_, err = pool.Exec(ctx, `INSERT INTO provider_connections (channel_id, account_id, provider, state) VALUES ($1, $2, 'telegram', 'connected')`, channelID, accountID)
	require.NoError(t, err)
	var contactID uuid.UUID
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO contacts (account_id, channel_id, external_identity, display_name) VALUES ($1, $2, '42', 'Telegram user') RETURNING id`, accountID, channelID).Scan(&contactID))
	var conversationID uuid.UUID
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO conversations (account_id, contact_id, channel_id, external_thread_id) VALUES ($1, $2, $3, '42') RETURNING id`, accountID, contactID, channelID).Scan(&conversationID))
	var messageID uuid.UUID
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO messages (account_id, conversation_id, direction, sender_type, content_type, content, provider_message_id, delivery_status) VALUES ($1, $2, 'outbound', 'human', 'text', '{"text":"before"}', '71', 'sent') RETURNING id`, accountID, conversationID).Scan(&messageID))

	require.NoError(t, svc.EditProviderMessage(ctx, accountID, userID, conversationID, messageID, "after"))
	require.NoError(t, svc.DeleteProviderMessage(ctx, accountID, userID, conversationID, messageID))
	require.NoError(t, svc.ChangeProviderReaction(ctx, accountID, userID, conversationID, messageID, "👍", false))
	reply, err := svc.SendMessage(ctx, accountID, conversationID, "human", &userID, "text", "reply", "", &messageID, nil, nil, "", "reply-idempotency-key")
	require.NoError(t, err)
	var replyPayload []byte
	require.NoError(t, pool.QueryRow(ctx, `SELECT command FROM message_outbox WHERE message_id = $1`, reply.ID).Scan(&replyPayload))
	var replyCommand messaging.Command
	require.NoError(t, json.Unmarshal(replyPayload, &replyCommand))
	require.Equal(t, "71", replyCommand.Message.ReplyToProviderID)
	require.Equal(t, messageID, *reply.ReplyToMessageID)

	rows, err := pool.Query(ctx, `SELECT command FROM message_outbox WHERE message_id = $1`, messageID)
	require.NoError(t, err)
	defer rows.Close()
	kinds := map[messaging.CommandKind]bool{}
	for rows.Next() {
		var payload []byte
		require.NoError(t, rows.Scan(&payload))
		var command messaging.Command
		require.NoError(t, json.Unmarshal(payload, &command))
		require.Equal(t, messaging.ProviderTelegram, command.Provider)
		require.Equal(t, channelID.String(), command.ChannelID)
		require.NoError(t, command.Validate())
		kinds[command.Kind] = true
	}
	require.NoError(t, rows.Err())
	require.Equal(t, map[messaging.CommandKind]bool{
		messaging.CommandEditMessage: true, messaging.CommandDeleteMessage: true, messaging.CommandChangeReaction: true,
	}, kinds)
}

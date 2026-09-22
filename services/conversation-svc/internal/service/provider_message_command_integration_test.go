package service_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/whatfunnel/whatfunnel/packages/go-common/messaging"
	"github.com/whatfunnel/whatfunnel/packages/go-common/types"
	"github.com/whatfunnel/whatfunnel/services/conversation-svc/internal/service"
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

	require.NoError(t, svc.EditProviderMessage(ctx, accountID, userID, types.RoleAdmin, conversationID, messageID, "after"))
	require.NoError(t, svc.DeleteProviderMessage(ctx, accountID, userID, types.RoleAdmin, conversationID, messageID))
	require.NoError(t, svc.ChangeProviderReaction(ctx, accountID, userID, types.RoleAdmin, conversationID, messageID, "👍", false))
	reply, err := svc.SendMessage(ctx, service.SendMessageParams{
		AccountID:        accountID,
		ConversationID:   conversationID,
		Sender:           types.MessageSenderHuman,
		SenderUserID:     &userID,
		ContentType:      "text",
		Text:             "reply",
		ReplyToMessageID: &messageID,
		IdempotencyKey:   "reply-idempotency-key",
	})
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

func TestProviderMessageMutationOwnershipAndPermissions(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	svc, pool, _ := testService(t)
	ctx := context.Background()
	accountID, managerID := setupTestTenant(t, pool, "provider-mutation-auth")

	var agent1ID, agent2ID uuid.UUID
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO users (account_id, email, password_hash, role) VALUES ($1, 'agent1@example.com', 'hash', 'agent') RETURNING id`, accountID).Scan(&agent1ID))
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO users (account_id, email, password_hash, role) VALUES ($1, 'agent2@example.com', 'hash', 'agent') RETURNING id`, accountID).Scan(&agent2ID))

	capabilities, err := json.Marshal(messaging.Capabilities{Replies: true, Reactions: true, Edits: true, Deletes: true})
	require.NoError(t, err)

	var channelID uuid.UUID
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO channels (account_id, type, provider, label, status, capabilities) VALUES ($1, 'telegram', 'telegram', 'Test bot', 'connected', $2) RETURNING id`, accountID, capabilities).Scan(&channelID))
	_, err = pool.Exec(ctx, `INSERT INTO provider_connections (channel_id, account_id, provider, state) VALUES ($1, $2, 'telegram', 'connected')`, channelID, accountID)
	require.NoError(t, err)

	var contactID uuid.UUID
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO contacts (account_id, channel_id, external_identity, display_name) VALUES ($1, $2, '43', 'Telegram user 2') RETURNING id`, accountID, channelID).Scan(&contactID))

	// Conversation assigned to agent1
	var conversationID uuid.UUID
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO conversations (account_id, contact_id, channel_id, external_thread_id, assigned_user_ids) VALUES ($1, $2, $3, '43', $4) RETURNING id`, accountID, contactID, channelID, []uuid.UUID{agent1ID}).Scan(&conversationID))

	// Message authored by agent1
	var messageID uuid.UUID
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO messages (account_id, conversation_id, direction, sender_type, sender_user_id, content_type, content, provider_message_id, delivery_status) VALUES ($1, $2, 'outbound', 'human', $3, 'text', '{"text":"agent1 text"}', '80', 'sent') RETURNING id`, accountID, conversationID, agent1ID).Scan(&messageID))

	// 1. Agent2 (unassigned) trying to edit message -> rejected by visibility (conversation not found)
	err = svc.EditProviderMessage(ctx, accountID, agent2ID, types.RoleAgent, conversationID, messageID, "tampered by agent2")
	require.Error(t, err)
	require.Contains(t, err.Error(), "conversation not found")

	// Now assign agent2 as well so agent2 can view conversation, but did not author the message
	_, err = pool.Exec(ctx, `UPDATE conversations SET assigned_user_ids = $1 WHERE id = $2`, []uuid.UUID{agent1ID, agent2ID}, conversationID)
	require.NoError(t, err)

	// 2. Agent2 can see conversation now, but attempts to edit Agent1's message -> forbidden
	err = svc.EditProviderMessage(ctx, accountID, agent2ID, types.RoleAgent, conversationID, messageID, "tampered by agent2")
	require.Error(t, err)
	require.Contains(t, err.Error(), "forbidden: cannot modify messages sent by other users")

	// 3. Agent2 attempts to delete Agent1's message -> forbidden
	err = svc.DeleteProviderMessage(ctx, accountID, agent2ID, types.RoleAgent, conversationID, messageID)
	require.Error(t, err)
	require.Contains(t, err.Error(), "forbidden: cannot modify messages sent by other users")

	// 4. Agent1 (original author) edits own message -> succeeds
	err = svc.EditProviderMessage(ctx, accountID, agent1ID, types.RoleAgent, conversationID, messageID, "agent1 edited")
	require.NoError(t, err)

	// 5. Manager (different user, but manager role) edits message -> succeeds
	err = svc.EditProviderMessage(ctx, accountID, managerID, types.RoleManager, conversationID, messageID, "manager override")
	require.NoError(t, err)

	// 6. Manager deletes message -> succeeds
	err = svc.DeleteProviderMessage(ctx, accountID, managerID, types.RoleManager, conversationID, messageID)
	require.NoError(t, err)
}


package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/whatfunnel/whatfunnel/packages/go-common/types"
)

func TestService_InboxVisibility(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	svc, pool, _ := testService(t)
	ctx := context.Background()

	accountID, adminID := setupTestTenant(t, pool, "vis-test")

	// Create a member user in the same account
	var memberID uuid.UUID
	err := pool.QueryRow(ctx, `
		INSERT INTO users (account_id, email, password_hash, role)
		VALUES ($1, 'member@example.com', 'hash', 'member') RETURNING id
	`, accountID).Scan(&memberID)
	require.NoError(t, err)

	// Create a channel
	var channelID uuid.UUID
	err = pool.QueryRow(ctx, `
		INSERT INTO channels (account_id, type, status)
		VALUES ($1, 'matrix_whatsapp', 'connected') RETURNING id
	`, accountID).Scan(&channelID)
	require.NoError(t, err)

	// Create three contacts
	var contact1, contact2, contact3 uuid.UUID
	err = pool.QueryRow(ctx, `INSERT INTO contacts (account_id, channel_id, external_identity) VALUES ($1, $2, 'c1') RETURNING id`, accountID, channelID).Scan(&contact1)
	require.NoError(t, err)
	err = pool.QueryRow(ctx, `INSERT INTO contacts (account_id, channel_id, external_identity) VALUES ($1, $2, 'c2') RETURNING id`, accountID, channelID).Scan(&contact2)
	require.NoError(t, err)
	err = pool.QueryRow(ctx, `INSERT INTO contacts (account_id, channel_id, external_identity) VALUES ($1, $2, 'c3') RETURNING id`, accountID, channelID).Scan(&contact3)
	require.NoError(t, err)

	// Create three conversations:
	// convo1: unassigned
	// convo2: assigned to member
	// convo3: assigned to admin only
	var convo1, convo2, convo3 uuid.UUID
	err = pool.QueryRow(ctx, `INSERT INTO conversations (account_id, contact_id, channel_id, assigned_user_ids) VALUES ($1, $2, $3, '{}') RETURNING id`, accountID, contact1, channelID).Scan(&convo1)
	require.NoError(t, err)
	err = pool.QueryRow(ctx, `INSERT INTO conversations (account_id, contact_id, channel_id, assigned_user_ids) VALUES ($1, $2, $3, $4) RETURNING id`, accountID, contact2, channelID, []uuid.UUID{memberID}).Scan(&convo2)
	require.NoError(t, err)
	err = pool.QueryRow(ctx, `INSERT INTO conversations (account_id, contact_id, channel_id, assigned_user_ids) VALUES ($1, $2, $3, $4) RETURNING id`, accountID, contact3, channelID, []uuid.UUID{adminID}).Scan(&convo3)
	require.NoError(t, err)

	// Test case 1: Member when settings.unassigned_conversations_visible_to_members is true (default)
	// Member should see: convo1 (unassigned) and convo2 (assigned to member). Not convo3.
	list, err := svc.ListConversations(ctx, accountID, memberID, types.RoleMember, "all", "")
	require.NoError(t, err)
	assert.Len(t, list, 2)
	ids := map[uuid.UUID]bool{list[0].Conversation.ID: true, list[1].Conversation.ID: true}
	assert.True(t, ids[convo1])
	assert.True(t, ids[convo2])
	assert.False(t, ids[convo3])

	// Test case 2: Admin should see all 3 conversations
	listAdmin, err := svc.ListConversations(ctx, accountID, adminID, types.RoleAdmin, "all", "")
	require.NoError(t, err)
	assert.Len(t, listAdmin, 3)

	// Test case 3: Member when setting is explicitly false
	_, err = pool.Exec(ctx, `UPDATE accounts SET settings = '{"unassigned_conversations_visible_to_members": false}' WHERE id = $1`, accountID)
	require.NoError(t, err)

	// Member should now only see convo2 (assigned to member)
	list2, err := svc.ListConversations(ctx, accountID, memberID, types.RoleMember, "all", "")
	require.NoError(t, err)
	require.Len(t, list2, 1)
	assert.Equal(t, convo2, list2[0].Conversation.ID)

	// Member tries to get convo1 directly -> should return not found (404 logic)
	_, err = svc.GetConversation(ctx, accountID, memberID, convo1, types.RoleMember)
	assert.EqualError(t, err, "conversation not found")

	// Member tries to get convo3 directly -> should return not found
	_, err = svc.GetConversation(ctx, accountID, memberID, convo3, types.RoleMember)
	assert.EqualError(t, err, "conversation not found")

	// Member gets convo2 directly -> should succeed
	convoOut, err := svc.GetConversation(ctx, accountID, memberID, convo2, types.RoleMember)
	require.NoError(t, err)
	assert.Equal(t, convo2, convoOut.Conversation.ID)
}

func TestService_InboxPagination(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	svc, pool, _ := testService(t)
	ctx := context.Background()

	accountID, adminID := setupTestTenant(t, pool, "pag-test")

	// Create channel, contact, conversation
	var channelID uuid.UUID
	err := pool.QueryRow(ctx, `INSERT INTO channels (account_id, type, status) VALUES ($1, 'matrix_whatsapp', 'connected') RETURNING id`, accountID).Scan(&channelID)
	require.NoError(t, err)

	var contactID uuid.UUID
	err = pool.QueryRow(ctx, `INSERT INTO contacts (account_id, channel_id, external_identity) VALUES ($1, $2, 'c1') RETURNING id`, accountID, channelID).Scan(&contactID)
	require.NoError(t, err)

	var convoID uuid.UUID
	err = pool.QueryRow(ctx, `INSERT INTO conversations (account_id, contact_id, channel_id) VALUES ($1, $2, $3) RETURNING id`, accountID, contactID, channelID).Scan(&convoID)
	require.NoError(t, err)

	// Insert 5 messages with distinct timestamps
	// msg1 (oldest) to msg5 (newest)
	msgIDs := make([]uuid.UUID, 5)
	now := time.Now().UTC()
	for i := 0; i < 5; i++ {
		ts := now.Add(time.Duration(i) * time.Minute)
		err = pool.QueryRow(ctx, `
			INSERT INTO messages (account_id, conversation_id, direction, sender_type, content_type, content, created_at)
			VALUES ($1, $2, 'inbound', 'contact', 'text', '{"text": "msg"}', $3) RETURNING id
		`, accountID, convoID, ts).Scan(&msgIDs[i])
		require.NoError(t, err)
	}

	// 1. Fetch first page of size 2 (should return msg5, msg4)
	msgsPage1, cursor, err := svc.GetConversationMessages(ctx, accountID, adminID, convoID, types.RoleAdmin, "", 2)
	require.NoError(t, err)
	require.Len(t, msgsPage1, 2)
	assert.Equal(t, msgIDs[4], msgsPage1[0].ID) // msg5
	assert.Equal(t, msgIDs[3], msgsPage1[1].ID) // msg4
	assert.NotEmpty(t, cursor)

	// 2. Simulate concurrent arrival of msg6 (newest message)
	var msg6ID uuid.UUID
	err = pool.QueryRow(ctx, `
		INSERT INTO messages (account_id, conversation_id, direction, sender_type, content_type, content, created_at)
		VALUES ($1, $2, 'inbound', 'contact', 'text', '{"text": "msg6"}', $3) RETURNING id
	`, accountID, convoID, now.Add(10*time.Minute)).Scan(&msg6ID)
	require.NoError(t, err)

	// 3. Fetch second page using the cursor from page 1 (should return msg3, msg2 - no duplicates, and does not return msg6)
	msgsPage2, cursor2, err := svc.GetConversationMessages(ctx, accountID, adminID, convoID, types.RoleAdmin, cursor, 2)
	require.NoError(t, err)
	require.Len(t, msgsPage2, 2)
	assert.Equal(t, msgIDs[2], msgsPage2[0].ID) // msg3
	assert.Equal(t, msgIDs[1], msgsPage2[1].ID) // msg2
	assert.NotEmpty(t, cursor2)

	// 4. Fetch third page using cursor2 (should return msg1)
	msgsPage3, cursor3, err := svc.GetConversationMessages(ctx, accountID, adminID, convoID, types.RoleAdmin, cursor2, 2)
	require.NoError(t, err)
	require.Len(t, msgsPage3, 1)
	assert.Equal(t, msgIDs[0], msgsPage3[0].ID) // msg1
	assert.Empty(t, cursor3)                    // reached end
}

package service_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	fakeadapter "github.com/whatfunnel/whatfunnel/adapters/fake"
	matrixadapter "github.com/whatfunnel/whatfunnel/adapters/matrix-mautrix"
	"github.com/whatfunnel/whatfunnel/packages/go-common/types"
)

// mockConfigurableAdapter implements types.ChannelAdapter & Configure
type mockConfigurableAdapter struct {
	types.ChannelAdapter
	configured map[string]matrixadapter.Credentials
}

func (m *mockConfigurableAdapter) Configure(channelID string, creds matrixadapter.Credentials) {
	if m.configured == nil {
		m.configured = make(map[string]matrixadapter.Credentials)
	}
	m.configured[channelID] = creds
}

func (m *mockConfigurableAdapter) Status(channelID string) types.ChannelStatus {
	return types.ChannelStatus{
		Status: "connected",
		Detail: "Mock adapter connected",
	}
}

func TestInitAdapters_CredentialsFormats(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	svc, pool, _ := testService(t)
	ctx := context.Background()

	accountID, _ := setupTestTenant(t, pool, "init-creds-test")

	mockAdapter := &mockConfigurableAdapter{
		ChannelAdapter: fakeadapter.New(),
		configured:     make(map[string]matrixadapter.Credentials),
	}
	svc.RegisterAdapter("matrix_telegram", mockAdapter)

	// Case 1: Standard flat JSON credentials
	flatCreds := matrixadapter.Credentials{
		HomeserverURL: "https://matrix.org",
		UserID:        "@bot:matrix.org",
		AccessToken:   "flat_token_123",
	}
	flatBytes, _ := json.Marshal(flatCreds)
	flatEnc, err := svc.EncryptCredentials(flatBytes)
	require.NoError(t, err)

	var ch1ID uuid.UUID
	err = pool.QueryRow(ctx, `
		INSERT INTO channels (account_id, type, bridge_credentials, status)
		VALUES ($1, 'matrix_telegram', $2, 'connected') RETURNING id
	`, accountID, flatEnc).Scan(&ch1ID)
	require.NoError(t, err)

	// Case 2: Legacy double-encoded JSON string credentials
	doubleBytes, _ := json.Marshal(string(flatBytes))
	doubleEnc, err := svc.EncryptCredentials(doubleBytes)
	require.NoError(t, err)

	var ch2ID uuid.UUID
	err = pool.QueryRow(ctx, `
		INSERT INTO channels (account_id, type, bridge_credentials, status)
		VALUES ($1, 'matrix_telegram', $2, 'connected') RETURNING id
	`, accountID, doubleEnc).Scan(&ch2ID)
	require.NoError(t, err)

	// Run InitAdapters
	err = svc.InitAdapters(ctx)
	require.NoError(t, err)

	// Verify both channels were properly configured with decrypted credentials
	creds1, ok1 := mockAdapter.configured[ch1ID.String()]
	assert.True(t, ok1, "flat credentials channel must be configured")
	assert.Equal(t, "https://matrix.org", creds1.HomeserverURL)
	assert.Equal(t, "flat_token_123", creds1.AccessToken)

	creds2, ok2 := mockAdapter.configured[ch2ID.String()]
	assert.True(t, ok2, "double-encoded credentials channel must be configured")
	assert.Equal(t, "https://matrix.org", creds2.HomeserverURL)
	assert.Equal(t, "flat_token_123", creds2.AccessToken)
}

func TestScanConversationRow_EdgeCases(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	svc, pool, _ := testService(t)
	ctx := context.Background()

	accountID, adminID := setupTestTenant(t, pool, "scan-row-test")

	// Create channel
	var channelID uuid.UUID
	err := pool.QueryRow(ctx, `
		INSERT INTO channels (account_id, type, status)
		VALUES ($1, 'matrix_whatsapp', 'connected') RETURNING id
	`, accountID).Scan(&channelID)
	require.NoError(t, err)

	// Create contact
	var contactID uuid.UUID
	err = pool.QueryRow(ctx, `
		INSERT INTO contacts (account_id, channel_id, external_identity, display_name, avatar_url)
		VALUES ($1, $2, 'contact-scan-1', 'Scan Contact', 'https://avatar.url/1') RETURNING id
	`, accountID, channelID).Scan(&contactID)
	require.NoError(t, err)

	// 1. Conversation with NO messages and NO lead
	var emptyConvoID uuid.UUID
	err = pool.QueryRow(ctx, `
		INSERT INTO conversations (account_id, contact_id, channel_id, status)
		VALUES ($1, $2, $3, 'open') RETURNING id
	`, accountID, contactID, channelID).Scan(&emptyConvoID)
	require.NoError(t, err)

	emptyItem, err := svc.GetConversation(ctx, accountID, adminID, emptyConvoID, types.RoleAdmin)
	require.NoError(t, err)
	assert.Equal(t, emptyConvoID, emptyItem.Conversation.ID)
	assert.Nil(t, emptyItem.LastMessagePreview, "empty conversation must have nil LastMessagePreview")
	assert.Nil(t, emptyItem.Lead, "empty conversation must have nil Lead")
	assert.False(t, emptyItem.Unread, "empty conversation without last_message_at must be unread=false")
	require.NotNil(t, emptyItem.ContactName)
	assert.Equal(t, "Scan Contact", *emptyItem.ContactName)
	require.NotNil(t, emptyItem.ContactAvatarURL)
	assert.Equal(t, "https://avatar.url/1", *emptyItem.ContactAvatarURL)
	require.NotNil(t, emptyItem.ChannelType)
	assert.Equal(t, "matrix_whatsapp", *emptyItem.ChannelType)

	// 2. Add message and lead
	msgTime := time.Now().UTC()
	var msgID uuid.UUID
	err = pool.QueryRow(ctx, `
		INSERT INTO messages (account_id, conversation_id, direction, sender_type, content_type, content, external_message_id, created_at)
		VALUES ($1, $2, 'inbound', 'contact', 'text', '{"text":"hello"}', 'ext-1', $3) RETURNING id
	`, accountID, emptyConvoID, msgTime).Scan(&msgID)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `UPDATE conversations SET last_message_at = $1 WHERE id = $2`, msgTime, emptyConvoID)
	require.NoError(t, err)

	// Create Lead for emptyConvoID
	lead, err := svc.CreateLead(ctx, accountID, adminID, emptyConvoID, types.RoleAdmin)
	require.NoError(t, err)

	// Update lead tags
	_, err = svc.UpdateLeadTags(ctx, accountID, adminID, lead.ID, types.RoleAdmin, []string{"urgent", "vip"})
	require.NoError(t, err)

	// Fetch again - now has message preview, lead, and Unread=true (no read receipt yet)
	itemWithData, err := svc.GetConversation(ctx, accountID, adminID, emptyConvoID, types.RoleAdmin)
	require.NoError(t, err)
	require.NotNil(t, itemWithData.LastMessagePreview)
	assert.Equal(t, msgID, itemWithData.LastMessagePreview.ID)
	assert.Equal(t, "inbound", itemWithData.LastMessagePreview.Direction)
	assert.Equal(t, "contact", itemWithData.LastMessagePreview.SenderType)
	assert.Equal(t, "text", itemWithData.LastMessagePreview.ContentType)
	assert.Equal(t, "ext-1", *itemWithData.LastMessagePreview.ExternalMessageID)
	assert.True(t, itemWithData.Unread, "unread should be true when no read receipt exists")

	require.NotNil(t, itemWithData.Lead)
	assert.Equal(t, lead.ID, itemWithData.Lead.ID)
	assert.Equal(t, "new", itemWithData.Lead.CurrentStateKey)
	assert.Equal(t, []string{"urgent", "vip"}, itemWithData.Lead.Tags)

	// 3. Mark conversation as read
	err = svc.ReadConversation(ctx, accountID, adminID, emptyConvoID)
	require.NoError(t, err)

	// Fetch again - Unread should now be false
	readItem, err := svc.GetConversation(ctx, accountID, adminID, emptyConvoID, types.RoleAdmin)
	require.NoError(t, err)
	assert.False(t, readItem.Unread, "unread should be false after ReadConversation")

	// 4. Test ListConversations with filters
	// Filter: lead_state = "new"
	filteredList, err := svc.ListConversations(ctx, accountID, adminID, types.RoleAdmin, "all", "new")
	require.NoError(t, err)
	assert.Len(t, filteredList, 1)
	assert.Equal(t, emptyConvoID, filteredList[0].Conversation.ID)

	// Filter: lead_state = "won" (none should match)
	emptyFilteredList, err := svc.ListConversations(ctx, accountID, adminID, types.RoleAdmin, "all", "won")
	require.NoError(t, err)
	assert.Empty(t, emptyFilteredList)
}

func TestCanSeeConversation_RBAC_AllEndpoints(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	svc, pool, _ := testService(t)
	ctx := context.Background()

	accountID, adminID := setupTestTenant(t, pool, "rbac-all-test")

	// Create 2 member users
	var memberA, memberB uuid.UUID
	err := pool.QueryRow(ctx, `INSERT INTO users (account_id, email, password_hash, role) VALUES ($1, $2, 'h', 'agent') RETURNING id`, accountID, fmt.Sprintf("member_a_%s@example.com", uuid.New())).Scan(&memberA)
	require.NoError(t, err)
	err = pool.QueryRow(ctx, `INSERT INTO users (account_id, email, password_hash, role) VALUES ($1, $2, 'h', 'agent') RETURNING id`, accountID, fmt.Sprintf("member_b_%s@example.com", uuid.New())).Scan(&memberB)
	require.NoError(t, err)

	// Create Channel & 2 Contacts
	var channelID, contactID1, contactID2 uuid.UUID
	err = pool.QueryRow(ctx, `INSERT INTO channels (account_id, type, status) VALUES ($1, 'matrix_whatsapp', 'connected') RETURNING id`, accountID).Scan(&channelID)
	require.NoError(t, err)
	err = pool.QueryRow(ctx, `INSERT INTO contacts (account_id, channel_id, external_identity) VALUES ($1, $2, 'c-rbac-1') RETURNING id`, accountID, channelID).Scan(&contactID1)
	require.NoError(t, err)
	err = pool.QueryRow(ctx, `INSERT INTO contacts (account_id, channel_id, external_identity) VALUES ($1, $2, 'c-rbac-2') RETURNING id`, accountID, channelID).Scan(&contactID2)
	require.NoError(t, err)

	// Create conversation assigned to memberA
	var convoAssignedToA uuid.UUID
	err = pool.QueryRow(ctx, `
		INSERT INTO conversations (account_id, contact_id, channel_id, assigned_user_ids)
		VALUES ($1, $2, $3, $4) RETURNING id
	`, accountID, contactID1, channelID, []uuid.UUID{memberA}).Scan(&convoAssignedToA)
	require.NoError(t, err)

	// Insert message in conversation
	_, err = pool.Exec(ctx, `
		INSERT INTO messages (account_id, conversation_id, direction, sender_type, content_type, content)
		VALUES ($1, $2, 'inbound', 'contact', 'text', '{"text":"test"}')
	`, accountID, convoAssignedToA)
	require.NoError(t, err)

	// Create a lead on convoAssignedToA as Member A
	leadA, err := svc.CreateLead(ctx, accountID, memberA, convoAssignedToA, types.RoleMember)
	require.NoError(t, err)
	require.NotNil(t, leadA)

	// Create a note on leadA as Member A
	noteA, err := svc.CreateLeadNote(ctx, accountID, memberA, leadA.ID, types.RoleMember, "Initial note")
	require.NoError(t, err)
	require.NotNil(t, noteA)

	// ==========================================
	// Test Member B (NOT assigned to convoAssignedToA)
	// Member B should be rejected on all 7 methods!
	// ==========================================

	// 1. GetConversationMessages
	_, _, err = svc.GetConversationMessages(ctx, accountID, memberB, convoAssignedToA, types.RoleMember, "", 10)
	assert.EqualError(t, err, "conversation not found")

	// 2. CreateLead
	_, err = svc.CreateLead(ctx, accountID, memberB, convoAssignedToA, types.RoleMember)
	assert.EqualError(t, err, "conversation not found")

	// 3. UpdateLeadState
	_, err = svc.UpdateLeadState(ctx, accountID, memberB, leadA.ID, types.RoleMember, "contacted")
	assert.EqualError(t, err, "lead not found")

	// 4. UpdateLeadTags
	_, err = svc.UpdateLeadTags(ctx, accountID, memberB, leadA.ID, types.RoleMember, []string{"tag1"})
	assert.EqualError(t, err, "lead not found")

	// 5. CreateLeadNote
	_, err = svc.CreateLeadNote(ctx, accountID, memberB, leadA.ID, types.RoleMember, "Unauthorized note")
	assert.EqualError(t, err, "lead not found")

	// 6. ListLeadNotes
	_, err = svc.ListLeadNotes(ctx, accountID, memberB, leadA.ID, types.RoleMember)
	assert.EqualError(t, err, "lead not found")

	// 7. ListLeadHistory
	_, err = svc.ListLeadHistory(ctx, accountID, memberB, leadA.ID, types.RoleMember)
	assert.EqualError(t, err, "lead not found")

	// ==========================================
	// Test Member A (assigned) - all 7 succeed!
	// ==========================================

	msgs, _, err := svc.GetConversationMessages(ctx, accountID, memberA, convoAssignedToA, types.RoleMember, "", 10)
	require.NoError(t, err)
	assert.Len(t, msgs, 1)

	leadA2, err := svc.CreateLead(ctx, accountID, memberA, convoAssignedToA, types.RoleMember)
	require.NoError(t, err)
	assert.Equal(t, leadA.ID, leadA2.ID)

	leadUpdated, err := svc.UpdateLeadState(ctx, accountID, memberA, leadA.ID, types.RoleMember, "contacted")
	require.NoError(t, err)
	assert.Equal(t, "contacted", leadUpdated.CurrentStateKey)

	leadTags, err := svc.UpdateLeadTags(ctx, accountID, memberA, leadA.ID, types.RoleMember, []string{"member-tag"})
	require.NoError(t, err)
	assert.Equal(t, []string{"member-tag"}, leadTags.Tags)

	noteA2, err := svc.CreateLeadNote(ctx, accountID, memberA, leadA.ID, types.RoleMember, "Second note")
	require.NoError(t, err)
	assert.Equal(t, "Second note", noteA2.Body)

	notesList, err := svc.ListLeadNotes(ctx, accountID, memberA, leadA.ID, types.RoleMember)
	require.NoError(t, err)
	assert.Len(t, notesList, 2)

	historyList, err := svc.ListLeadHistory(ctx, accountID, memberA, leadA.ID, types.RoleMember)
	require.NoError(t, err)
	assert.Len(t, historyList, 2) // initial creation + update to contacted

	// ==========================================
	// Test Admin - all 7 succeed!
	// ==========================================

	adminMsgs, _, err := svc.GetConversationMessages(ctx, accountID, adminID, convoAssignedToA, types.RoleAdmin, "", 10)
	require.NoError(t, err)
	assert.Len(t, adminMsgs, 1)

	adminNotes, err := svc.ListLeadNotes(ctx, accountID, adminID, leadA.ID, types.RoleAdmin)
	require.NoError(t, err)
	assert.Len(t, adminNotes, 2)

	// ==========================================
	// Test Unassigned Conversation Visibility Toggle
	// ==========================================
	var unassignedConvoID uuid.UUID
	err = pool.QueryRow(ctx, `
		INSERT INTO conversations (account_id, contact_id, channel_id, assigned_user_ids)
		VALUES ($1, $2, $3, '{}') RETURNING id
	`, accountID, contactID2, channelID).Scan(&unassignedConvoID)
	require.NoError(t, err)

	// When setting is default/true, Member B CAN see unassigned conversation
	_, _, err = svc.GetConversationMessages(ctx, accountID, memberB, unassignedConvoID, types.RoleMember, "", 10)
	require.NoError(t, err)

	// When setting is false, Member B CANNOT see unassigned conversation
	_, err = pool.Exec(ctx, `UPDATE accounts SET settings = '{"unassigned_conversations_visible_to_members": false}' WHERE id = $1`, accountID)
	require.NoError(t, err)

	_, _, err = svc.GetConversationMessages(ctx, accountID, memberB, unassignedConvoID, types.RoleMember, "", 10)
	assert.EqualError(t, err, "conversation not found")
}

func TestSyncChannelStatus_And_ListChannels(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	svc, pool, _ := testService(t)
	ctx := context.Background()

	accountID, _ := setupTestTenant(t, pool, "channel-status-test")

	mockAdapter := &mockConfigurableAdapter{
		ChannelAdapter: fakeadapter.New(),
	}
	svc.RegisterAdapter("matrix_telegram", mockAdapter)

	// 1. Channel in disconnected status
	var disconnectedChID uuid.UUID
	err := pool.QueryRow(ctx, `
		INSERT INTO channels (account_id, type, status, status_detail)
		VALUES ($1, 'matrix_telegram', 'disconnected', 'User logged out') RETURNING id
	`, accountID).Scan(&disconnectedChID)
	require.NoError(t, err)

	// 2. Channel in pending status
	var pendingChID uuid.UUID
	err = pool.QueryRow(ctx, `
		INSERT INTO channels (account_id, type, status, status_detail)
		VALUES ($1, 'matrix_telegram', 'pending', 'Waiting for QR scan') RETURNING id
	`, accountID).Scan(&pendingChID)
	require.NoError(t, err)

	// 3. Channel in connected status
	var connectedChID uuid.UUID
	err = pool.QueryRow(ctx, `
		INSERT INTO channels (account_id, type, status, status_detail)
		VALUES ($1, 'matrix_telegram', 'connected', 'Old detail') RETURNING id
	`, accountID).Scan(&connectedChID)
	require.NoError(t, err)

	// ListChannels
	channels, err := svc.ListChannels(ctx, accountID)
	require.NoError(t, err)
	require.Len(t, channels, 3)

	channelMap := make(map[uuid.UUID]*types.Channel)
	for _, ch := range channels {
		channelMap[ch.ID] = ch
	}

	// Verify disconnected channel was NOT modified to adapter status
	assert.Equal(t, "disconnected", channelMap[disconnectedChID].Status)
	assert.Equal(t, "User logged out", *channelMap[disconnectedChID].StatusDetail)

	// Verify pending channel was NOT modified to adapter status
	assert.Equal(t, "pending", channelMap[pendingChID].Status)
	assert.Equal(t, "Waiting for QR scan", *channelMap[pendingChID].StatusDetail)

	// Verify connected channel WAS synced with adapter
	assert.Equal(t, "connected", channelMap[connectedChID].Status)
	assert.Equal(t, "Mock adapter connected", *channelMap[connectedChID].StatusDetail)

	// 4. GetChannelStatus fallback to DB when adapter is unregistered
	var unregChID uuid.UUID
	err = pool.QueryRow(ctx, `
		INSERT INTO channels (account_id, type, status, status_detail)
		VALUES ($1, 'matrix_instagram', 'connected', 'DB stored status') RETURNING id
	`, accountID).Scan(&unregChID)
	require.NoError(t, err)

	status, err := svc.GetChannelStatus(ctx, accountID, unregChID)
	require.NoError(t, err)
	assert.Equal(t, "connected", status.Status)
	assert.Equal(t, "DB stored status", status.Detail)
}

func TestSimulateInbound_EXISTS(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	svc, pool, _ := testService(t)
	ctx := context.Background()

	accountA, _ := setupTestTenant(t, pool, "SimulateTenantA")
	accountB, _ := setupTestTenant(t, pool, "SimulateTenantB")

	var chAID uuid.UUID
	err := pool.QueryRow(ctx, `
		INSERT INTO channels (account_id, type, status)
		VALUES ($1, 'matrix_whatsapp', 'connected') RETURNING id
	`, accountA).Scan(&chAID)
	require.NoError(t, err)

	// 1. Valid channel in account A
	err = svc.SimulateInbound(ctx, accountA, chAID.String(), "sim-contact", "Sim Contact", "", "text", "Hello", "")
	require.NoError(t, err)

	// 2. Channel in account A queried under account B (cross-tenant reject)
	err = svc.SimulateInbound(ctx, accountB, chAID.String(), "sim-contact", "Sim Contact", "", "text", "Hello", "")
	assert.Error(t, err)
	assert.EqualError(t, err, "channel not found or not owned by account")

	// 3. Non-existent channel
	err = svc.SimulateInbound(ctx, accountA, uuid.New().String(), "sim-contact", "Sim Contact", "", "text", "Hello", "")
	assert.Error(t, err)
	assert.EqualError(t, err, "channel not found or not owned by account")
}

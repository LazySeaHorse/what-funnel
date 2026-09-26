package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/whatfunnel/whatfunnel/packages/go-common/pubsub"
	"github.com/whatfunnel/whatfunnel/packages/go-common/types"
	"github.com/whatfunnel/whatfunnel/services/conversation-svc/internal/handler"
	"github.com/whatfunnel/whatfunnel/services/conversation-svc/internal/service"
)

func TestHandler_InboxEndpoints(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	pool := testPool(t)
	accountID, adminID := setupTestTenant(t, pool, "hand-inbox")

	// Create a member user
	var memberID uuid.UUID
	err := pool.QueryRow(context.Background(), `
		INSERT INTO users (account_id, email, password_hash, role)
		VALUES ($1, 'member-h@example.com', 'hash', 'agent') RETURNING id
	`, accountID).Scan(&memberID)
	require.NoError(t, err)

	// Create a channel
	var channelID uuid.UUID
	err = pool.QueryRow(context.Background(), `
		INSERT INTO channels (account_id, type, status)
		VALUES ($1, 'whatsapp', 'connected') RETURNING id
	`, accountID).Scan(&channelID)
	require.NoError(t, err)

	// Create contact and conversation
	var contactID uuid.UUID
	err = pool.QueryRow(context.Background(), `
		INSERT INTO contacts (account_id, channel_id, external_identity, display_name)
		VALUES ($1, $2, 'alice-jid', 'Alice') RETURNING id
	`, accountID, channelID).Scan(&contactID)
	require.NoError(t, err)

	var convoID uuid.UUID
	err = pool.QueryRow(context.Background(), `
		INSERT INTO conversations (account_id, contact_id, channel_id, assigned_user_ids)
		VALUES ($1, $2, $3, $4) RETURNING id
	`, accountID, contactID, channelID, []uuid.UUID{adminID}).Scan(&convoID)
	require.NoError(t, err)

	// Set up services
	redisAddr := "localhost:6379"
	ps, err := pubsub.NewClient(redisAddr)
	require.NoError(t, err)
	defer ps.Close()

	svc := service.New(pool, ps)

	// Mock sessions
	adminSess := &mockSessionStore{
		userID:    adminID,
		accountID: accountID,
		role:      "manager",
		loggedIn:  true,
	}

	memberSess := &mockSessionStore{
		userID:    memberID,
		accountID: accountID,
		role:      "member",
		loggedIn:  true,
	}

	// 1. GET /conversations as Admin
	{
		h := handler.New(svc, adminSess)
		r := mux.NewRouter()
		h.RegisterRoutes(r)

		req, _ := http.NewRequest(http.MethodGet, "/conversations", nil)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		var list []*types.ConversationListItem
		err = json.Unmarshal(rr.Body.Bytes(), &list)
		require.NoError(t, err)
		assert.Len(t, list, 1)
		assert.Equal(t, convoID, list[0].Conversation.ID)

		// Test pagination with limit and offset query parameters
		reqPag, _ := http.NewRequest(http.MethodGet, "/conversations?limit=1&offset=0", nil)
		rrPag := httptest.NewRecorder()
		r.ServeHTTP(rrPag, reqPag)
		assert.Equal(t, http.StatusOK, rrPag.Code)
		var listPag []*types.ConversationListItem
		err = json.Unmarshal(rrPag.Body.Bytes(), &listPag)
		require.NoError(t, err)
		assert.Len(t, listPag, 1)

		reqOffset, _ := http.NewRequest(http.MethodGet, "/conversations?limit=1&offset=1", nil)
		rrOffset := httptest.NewRecorder()
		r.ServeHTTP(rrOffset, reqOffset)
		assert.Equal(t, http.StatusOK, rrOffset.Code)
		var listOffset []*types.ConversationListItem
		err = json.Unmarshal(rrOffset.Body.Bytes(), &listOffset)
		require.NoError(t, err)
		assert.Len(t, listOffset, 0)
	}

	// 2. GET /conversations as Member (since convoID is assigned to adminID and setting=true by default,
	// member should be able to see it because it's default to true for unassigned but wait, convoID is NOT unassigned!
	// convoID is assigned to adminID, so member should NOT see it!)
	{
		h := handler.New(svc, memberSess)
		r := mux.NewRouter()
		h.RegisterRoutes(r)

		req, _ := http.NewRequest(http.MethodGet, "/conversations", nil)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		var list []*types.ConversationListItem
		err = json.Unmarshal(rr.Body.Bytes(), &list)
		require.NoError(t, err)
		assert.Len(t, list, 0) // Should not see convoID since it is assigned to adminID
	}

	// 3. Member tries to GET /conversations/{id} directly -> should receive 404
	{
		h := handler.New(svc, memberSess)
		r := mux.NewRouter()
		h.RegisterRoutes(r)

		req, _ := http.NewRequest(http.MethodGet, "/conversations/"+convoID.String(), nil)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusNotFound, rr.Code)
	}

	// 4. Admin assigns conversation to member: PATCH /conversations/{id}/assign
	{
		h := handler.New(svc, adminSess)
		r := mux.NewRouter()
		h.RegisterRoutes(r)

		body := map[string]any{"user_ids": []string{memberID.String()}}
		jsonBody, _ := json.Marshal(body)

		req, _ := http.NewRequest(http.MethodPatch, "/conversations/"+convoID.String()+"/assign", bytes.NewBuffer(jsonBody))
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
	}

	// 5. Member tries to PATCH assign -> should receive 403 (Admin only)
	{
		h := handler.New(svc, memberSess)
		r := mux.NewRouter()
		h.RegisterRoutes(r)

		body := map[string]any{"user_ids": []string{memberID.String()}}
		jsonBody, _ := json.Marshal(body)

		req, _ := http.NewRequest(http.MethodPatch, "/conversations/"+convoID.String()+"/assign", bytes.NewBuffer(jsonBody))
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusForbidden, rr.Code)
	}

	// 6. Now GET /conversations as Member -> should see 1 conversation
	{
		h := handler.New(svc, memberSess)
		r := mux.NewRouter()
		h.RegisterRoutes(r)

		req, _ := http.NewRequest(http.MethodGet, "/conversations", nil)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		var list []*types.ConversationListItem
		err = json.Unmarshal(rr.Body.Bytes(), &list)
		require.NoError(t, err)
		assert.Len(t, list, 1)
		assert.Equal(t, convoID, list[0].Conversation.ID)
	}

	// 7. POST /conversations/{id}/read as Member
	{
		h := handler.New(svc, memberSess)
		r := mux.NewRouter()
		h.RegisterRoutes(r)

		req, _ := http.NewRequest(http.MethodPost, "/conversations/"+convoID.String()+"/read", nil)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
	}

	// Per-chat AI can be disabled and reset to the workspace default.
	{
		h := handler.New(svc, memberSess)
		r := mux.NewRouter()
		h.RegisterRoutes(r)

		req, _ := http.NewRequest(http.MethodPatch, "/conversations/"+convoID.String()+"/ai-control", bytes.NewBufferString(`{"action":"block","reply_override":"disabled"}`))
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)

		var state, replyOverride string
		err = pool.QueryRow(context.Background(), `SELECT state, reply_override FROM conversation_ai_state WHERE conversation_id = $1`, convoID).Scan(&state, &replyOverride)
		require.NoError(t, err)
		assert.Equal(t, "blocked_manual", state)
		assert.Equal(t, "disabled", replyOverride)

		req, _ = http.NewRequest(http.MethodPatch, "/conversations/"+convoID.String()+"/ai-control", bytes.NewBufferString(`{"action":"resume","reply_override":"inherit"}`))
		rr = httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
		err = pool.QueryRow(context.Background(), `SELECT state, reply_override FROM conversation_ai_state WHERE conversation_id = $1`, convoID).Scan(&state, &replyOverride)
		require.NoError(t, err)
		assert.Equal(t, "active", state)
		assert.Equal(t, "inherit", replyOverride)

		_, err = pool.Exec(context.Background(), `
			UPDATE conversation_ai_state
			SET state = 'blocked_spam', state_reason = 'judge_likely_spam'
			WHERE conversation_id = $1
		`, convoID)
		require.NoError(t, err)

		req, _ = http.NewRequest(http.MethodPatch, "/conversations/"+convoID.String()+"/ai-control", bytes.NewBufferString(`{"action":"resume"}`))
		rr = httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusForbidden, rr.Code)

		adminHandler := handler.New(svc, adminSess)
		adminRouter := mux.NewRouter()
		adminHandler.RegisterRoutes(adminRouter)
		req, _ = http.NewRequest(http.MethodPatch, "/conversations/"+convoID.String()+"/ai-control", bytes.NewBufferString(`{"action":"resume"}`))
		rr = httptest.NewRecorder()
		adminRouter.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	}

	var sourceMessageID, draftID uuid.UUID
	err = pool.QueryRow(context.Background(), `
		INSERT INTO messages (account_id, conversation_id, direction, sender_type, content_type, content)
		VALUES ($1, $2, 'inbound', 'contact', 'text', '{"text":"Are you open?"}')
		RETURNING id
	`, accountID, convoID).Scan(&sourceMessageID)
	require.NoError(t, err)
	err = pool.QueryRow(context.Background(), `
		INSERT INTO ai_reply_drafts (
			account_id, conversation_id, source_message_id, draft_text, stage_matched, confidence
		) VALUES ($1, $2, $3, 'Yes, we are open.', 'pattern', 1.0)
		RETURNING id
	`, accountID, convoID, sourceMessageID).Scan(&draftID)
	require.NoError(t, err)

	// 8. An assigned member can retrieve and dismiss the pending draft.
	{
		h := handler.New(svc, memberSess)
		r := mux.NewRouter()
		h.RegisterRoutes(r)

		req, _ := http.NewRequest(http.MethodGet, "/conversations/"+convoID.String()+"/reply-draft", nil)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		require.Equal(t, http.StatusOK, rr.Code)

		var response struct {
			Draft *types.AIReplyDraft `json:"draft"`
		}
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &response))
		require.NotNil(t, response.Draft)
		assert.Equal(t, draftID, response.Draft.ID)
		assert.Equal(t, "Yes, we are open.", response.Draft.DraftText)

		req, _ = http.NewRequest(http.MethodPost, "/conversations/"+convoID.String()+"/reply-draft/"+draftID.String()+"/dismiss", nil)
		rr = httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		require.Equal(t, http.StatusOK, rr.Code)
	}

	var draftStatus string
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT status FROM ai_reply_drafts WHERE id = $1`, draftID).Scan(&draftStatus))
	assert.Equal(t, "dismissed", draftStatus)
}

func TestHandler_SendMessage_InternalAIDispatch(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	internalToken := "test-secret-internal-token-32-chars-long"
	t.Setenv("INTERNAL_SERVICE_TOKEN", internalToken)

	pool := testPool(t)
	accountID, _ := setupTestTenant(t, pool, "internal-send")

	var channelID uuid.UUID
	err := pool.QueryRow(context.Background(), `
		INSERT INTO channels (account_id, type, provider, status)
		VALUES ($1, 'telegram', 'telegram', 'connected') RETURNING id
	`, accountID).Scan(&channelID)
	require.NoError(t, err)

	var contactID uuid.UUID
	err = pool.QueryRow(context.Background(), `
		INSERT INTO contacts (account_id, channel_id, external_identity, display_name)
		VALUES ($1, $2, 'bob-id', 'Bob') RETURNING id
	`, accountID, channelID).Scan(&contactID)
	require.NoError(t, err)

	var convoID uuid.UUID
	err = pool.QueryRow(context.Background(), `
		INSERT INTO conversations (account_id, contact_id, channel_id, status)
		VALUES ($1, $2, $3, 'open') RETURNING id
	`, accountID, contactID, channelID).Scan(&convoID)
	require.NoError(t, err)

	_, err = pool.Exec(context.Background(), `
		UPDATE conversation_ai_state
		SET state = 'active', generation_epoch = 1
		WHERE conversation_id = $1
	`, convoID)
	require.NoError(t, err)

	ps, err := pubsub.NewClient("localhost:6379")
	require.NoError(t, err)
	defer ps.Close()

	svc := service.New(pool, ps)
	h := handler.New(svc, &mockSessionStore{})
	r := mux.NewRouter()
	h.RegisterRoutes(r)

	// 1. With X-User-Role: manager -> 200 OK
	{
		body := `{"content_type":"text","text":"auto reply","sender_type":"ai","generation_epoch":1,"message_purpose":"reply","idempotency_key":"k1"}`
		req, _ := http.NewRequest(http.MethodPost, "/internal/conversations/"+convoID.String()+"/send", bytes.NewBufferString(body))
		req.Header.Set("X-Internal-Token", internalToken)
		req.Header.Set("X-Account-ID", accountID.String())
		req.Header.Set("X-User-Role", "manager")
		req.Header.Set("Content-Type", "application/json")

		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	}

	// 2. With omitted X-User-Role (defaults to RoleManager) -> 200 OK
	{
		body := `{"content_type":"text","text":"auto reply 2","sender_type":"ai","generation_epoch":1,"message_purpose":"reply","idempotency_key":"k2"}`
		req, _ := http.NewRequest(http.MethodPost, "/internal/conversations/"+convoID.String()+"/send", bytes.NewBufferString(body))
		req.Header.Set("X-Internal-Token", internalToken)
		req.Header.Set("X-Account-ID", accountID.String())
		req.Header.Set("Content-Type", "application/json")

		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	}

	// 3. With X-User-Role: agent -> 403 Forbidden
	{
		body := `{"content_type":"text","text":"auto reply 3","sender_type":"ai","generation_epoch":1,"message_purpose":"reply","idempotency_key":"k3"}`
		req, _ := http.NewRequest(http.MethodPost, "/internal/conversations/"+convoID.String()+"/send", bytes.NewBufferString(body))
		req.Header.Set("X-Internal-Token", internalToken)
		req.Header.Set("X-Account-ID", accountID.String())
		req.Header.Set("X-User-Role", "agent")
		req.Header.Set("Content-Type", "application/json")

		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusForbidden, rr.Code)
	}

	// 4. With legacy X-User-Role: admin -> 403 Forbidden (no longer recognized)
	{
		body := `{"content_type":"text","text":"auto reply 4","sender_type":"ai","generation_epoch":1,"message_purpose":"reply","idempotency_key":"k4"}`
		req, _ := http.NewRequest(http.MethodPost, "/internal/conversations/"+convoID.String()+"/send", bytes.NewBufferString(body))
		req.Header.Set("X-Internal-Token", internalToken)
		req.Header.Set("X-Account-ID", accountID.String())
		req.Header.Set("X-User-Role", "admin")
		req.Header.Set("Content-Type", "application/json")

		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusForbidden, rr.Code)
	}
}

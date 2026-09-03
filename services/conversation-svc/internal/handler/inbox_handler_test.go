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
	"github.com/whatfunnel/whatfunnel/packages/go-common/crypto"
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
		VALUES ($1, 'matrix_whatsapp', 'connected') RETURNING id
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

	cipher, err := crypto.NewCipherFromHex("test-key-exactly-32-bytes-padded")
	require.NoError(t, err)

	svc := service.New(pool, cipher, ps)

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

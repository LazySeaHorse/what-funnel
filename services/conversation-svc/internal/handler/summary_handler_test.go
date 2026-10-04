package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/whatfunnel/whatfunnel/packages/go-common/pubsub"
	"github.com/whatfunnel/whatfunnel/packages/go-common/types"
	"github.com/whatfunnel/whatfunnel/services/conversation-svc/internal/handler"
	"github.com/whatfunnel/whatfunnel/services/conversation-svc/internal/service"
)

type summaryWorld struct {
	pool      *pgxpool.Pool
	ps        *pubsub.Client
	svc       *service.Service
	accountID uuid.UUID
	managerID uuid.UUID
	memberID  uuid.UUID // assigned to the conversation
	otherID   uuid.UUID // member not assigned
	convoID   uuid.UUID
}

func (w *summaryWorld) call(t *testing.T, sess *mockSessionStore, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	r := mux.NewRouter()
	handler.New(w.svc, sess).RegisterRoutes(r)
	req, _ := http.NewRequest(method, path, nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	return rr
}

func (w *summaryWorld) summaryURL() string {
	return "/conversations/" + w.convoID.String() + "/summary"
}

func (w *summaryWorld) requests(t *testing.T) int {
	t.Helper()
	entries, err := w.ps.RawClient().XRange(context.Background(), types.SummaryStreamRequested, "-", "+").Result()
	require.NoError(t, err)
	n := 0
	for _, e := range entries {
		var ev struct {
			ConversationID uuid.UUID `json:"conversation_id"`
		}
		require.NoError(t, json.Unmarshal([]byte(e.Values["payload"].(string)), &ev))
		if ev.ConversationID == w.convoID {
			n++
		}
	}
	return n
}

func (w *summaryWorld) addMessages(t *testing.T, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		_, err := w.pool.Exec(context.Background(), `
			INSERT INTO messages (account_id, conversation_id, direction, sender_type, content_type, content)
			VALUES ($1, $2, 'inbound', 'contact', 'text', '{"text":"hi"}')`, w.accountID, w.convoID)
		require.NoError(t, err)
	}
}

func newSummaryWorld(t *testing.T, name string) *summaryWorld {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	pool := testPool(t)
	ps, err := pubsub.NewClient("localhost:6379")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ps.Close() })

	w := &summaryWorld{pool: pool, ps: ps, svc: service.New(pool, ps)}
	w.accountID, w.managerID = setupTestTenant(t, pool, name)
	ctx := context.Background()
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO users (account_id, email, password_hash, role) VALUES ($1, $2, 'hash', 'agent') RETURNING id`, w.accountID, name+"-m@example.com").Scan(&w.memberID))
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO users (account_id, email, password_hash, role) VALUES ($1, $2, 'hash', 'agent') RETURNING id`, w.accountID, name+"-o@example.com").Scan(&w.otherID))
	var channelID, contactID uuid.UUID
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO channels (account_id, type, status) VALUES ($1, 'whatsapp', 'connected') RETURNING id`, w.accountID).Scan(&channelID))
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO contacts (account_id, channel_id, external_identity, display_name) VALUES ($1, $2, 'jid', 'Al') RETURNING id`, w.accountID, channelID).Scan(&contactID))
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO conversations (account_id, contact_id, channel_id, assigned_user_ids) VALUES ($1, $2, $3, $4) RETURNING id`, w.accountID, contactID, channelID, []uuid.UUID{w.memberID}).Scan(&w.convoID))
	// Isolate from stream entries left by earlier runs.
	_, _ = ps.RawClient().Del(ctx, types.SummaryStreamRequested).Result()
	return w
}

func sess(accountID, userID uuid.UUID, role string) *mockSessionStore {
	return &mockSessionStore{userID: userID, accountID: accountID, role: role, loggedIn: true}
}

type summaryResponse struct {
	Status  string                     `json:"status"`
	Summary *types.ConversationSummary `json:"summary"`
}

func TestHandler_ConversationSummary_GetAndRequest(t *testing.T) {
	w := newSummaryWorld(t, "sum-flow")
	ctx := context.Background()
	member := sess(w.accountID, w.memberID, "agent")

	// No summary yet -> null.
	rr := w.call(t, member, http.MethodGet, w.summaryURL())
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	assert.JSONEq(t, `{"summary":null}`, rr.Body.String())

	// Request with no summary: accepted and published.
	w.addMessages(t, 2)
	rr = w.call(t, member, http.MethodPost, w.summaryURL())
	require.Equal(t, http.StatusAccepted, rr.Code, rr.Body.String())
	var resp summaryResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Equal(t, "queued", resp.Status)
	assert.Nil(t, resp.Summary)
	assert.Equal(t, 1, w.requests(t))

	// Stored summary, labels from the account's schema, fresh.
	_, err := w.pool.Exec(ctx, `UPDATE accounts SET settings = jsonb_set(COALESCE(settings,'{}'::jsonb), '{summary_schema}', $2::jsonb) WHERE id = $1`,
		w.accountID, `[{"key":"next_action","label":"Next Step"},{"key":"budget","label":"Budget"}]`)
	require.NoError(t, err)
	_, err = w.pool.Exec(ctx, `INSERT INTO conversation_summaries (account_id, conversation_id, summary_fields, message_count_at_generation) VALUES ($1,$2,$3,2)`,
		w.accountID, w.convoID, `{"budget":"5k","next_action":"call","old":"x"}`)
	require.NoError(t, err)

	rr = w.call(t, member, http.MethodGet, w.summaryURL())
	require.Equal(t, http.StatusOK, rr.Code)
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	require.NotNil(t, resp.Summary)
	assert.Equal(t, []types.SummaryField{
		{Key: "next_action", Label: "Next Step", Value: "call"},
		{Key: "budget", Label: "Budget", Value: "5k"},
		{Key: "old", Label: "old", Value: "x"},
	}, resp.Summary.Fields)
	assert.Equal(t, 2, resp.Summary.MessageCountAtGeneration)
	assert.False(t, resp.Summary.Stale)
	assert.False(t, resp.Summary.GeneratedAt.IsZero())

	// Up to date: 200, nothing new published.
	rr = w.call(t, member, http.MethodPost, w.summaryURL())
	require.Equal(t, http.StatusOK, rr.Code)
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Equal(t, "up_to_date", resp.Status)
	assert.NotNil(t, resp.Summary)
	assert.Equal(t, 1, w.requests(t))

	// New message makes it stale; request is queued again and returns the old summary.
	w.addMessages(t, 1)
	rr = w.call(t, member, http.MethodGet, w.summaryURL())
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.True(t, resp.Summary.Stale)
	rr = w.call(t, member, http.MethodPost, w.summaryURL())
	require.Equal(t, http.StatusAccepted, rr.Code)
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Equal(t, "queued", resp.Status)
	assert.True(t, resp.Summary.Stale)
	assert.Equal(t, 2, w.requests(t))
}

func TestHandler_ConversationSummary_AuthAndScoping(t *testing.T) {
	w := newSummaryWorld(t, "sum-auth")
	ctx := context.Background()
	_, err := w.pool.Exec(ctx, `INSERT INTO conversation_summaries (account_id, conversation_id, summary_fields, message_count_at_generation) VALUES ($1,$2,'{"a":"secret"}',0)`, w.accountID, w.convoID)
	require.NoError(t, err)

	otherAccount, otherManager := setupTestTenant(t, w.pool, "sum-auth-other")

	cases := []struct {
		name string
		sess *mockSessionStore
		want int
	}{
		{"unauthenticated", &mockSessionStore{}, http.StatusUnauthorized},
		{"manager sees any conversation", sess(w.accountID, w.managerID, "manager"), http.StatusOK},
		{"assigned member", sess(w.accountID, w.memberID, "agent"), http.StatusOK},
		{"unassigned member hidden as 404", sess(w.accountID, w.otherID, "agent"), http.StatusNotFound},
		{"manager of another account gets 404", sess(otherAccount, otherManager, "manager"), http.StatusNotFound},
	}
	for _, tc := range cases {
		for _, method := range []string{http.MethodGet, http.MethodPost} {
			t.Run(tc.name+" "+method, func(t *testing.T) {
				rr := w.call(t, tc.sess, method, w.summaryURL())
				require.Equal(t, tc.want, rr.Code, rr.Body.String())
				if tc.want != http.StatusOK {
					assert.NotContains(t, rr.Body.String(), "secret")
				}
			})
		}
	}
	// Only the permitted POSTs for stale/none summaries publish; denied ones never do.
	assert.Equal(t, 0, w.requests(t), "summary is up to date and denied callers must not publish")

	rr := w.call(t, sess(w.accountID, w.managerID, "manager"), http.MethodGet, "/conversations/not-a-uuid/summary")
	assert.Equal(t, http.StatusBadRequest, rr.Code)
	rr = w.call(t, sess(w.accountID, w.managerID, "manager"), http.MethodGet, "/conversations/"+uuid.NewString()+"/summary")
	assert.Equal(t, http.StatusNotFound, rr.Code)
}

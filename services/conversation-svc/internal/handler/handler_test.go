package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	fakeadapter "github.com/whatfunnel/whatfunnel/adapters/fake"
	"github.com/whatfunnel/whatfunnel/packages/go-common/crypto"
	"github.com/whatfunnel/whatfunnel/packages/go-common/db"
	"github.com/whatfunnel/whatfunnel/packages/go-common/pubsub"
	"github.com/whatfunnel/whatfunnel/packages/go-common/types"
	"github.com/whatfunnel/whatfunnel/services/conversation-svc/internal/handler"
	"github.com/whatfunnel/whatfunnel/services/conversation-svc/internal/service"
)

type mockSessionStore struct {
	userID    uuid.UUID
	accountID uuid.UUID
	role      string
	loggedIn  bool
}

func (m *mockSessionStore) GetUserID(r *http.Request) (uuid.UUID, bool) {
	return m.userID, m.loggedIn
}

func (m *mockSessionStore) GetAccountID(r *http.Request) (uuid.UUID, bool) {
	return m.accountID, m.loggedIn
}

func (m *mockSessionStore) GetRole(r *http.Request) (string, bool) {
	return m.role, m.loggedIn
}

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://whatfunnel:whatfunnel@localhost:5432/whatfunnel?sslmode=disable"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := db.Connect(ctx, dsn)
	if err != nil {
		t.Skipf("skipping integration test: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func setupTestTenant(t *testing.T, pool *pgxpool.Pool, name string) (uuid.UUID, uuid.UUID) {
	t.Helper()
	ctx := context.Background()

	var accountID uuid.UUID
	err := pool.QueryRow(ctx, `INSERT INTO accounts (name) VALUES ($1) RETURNING id`, name).Scan(&accountID)
	require.NoError(t, err)

	var userID uuid.UUID
	err = pool.QueryRow(ctx, `INSERT INTO users (account_id, email, password_hash, role) VALUES ($1, $2, 'hash', 'admin') RETURNING id`, accountID, name+"@example.com").Scan(&userID)
	require.NoError(t, err)

	t.Cleanup(func() {
		pool.Exec(context.Background(), `DELETE FROM audit_logs WHERE account_id = $1`, accountID)
		pool.Exec(context.Background(), `DELETE FROM messages WHERE account_id = $1`, accountID)
		pool.Exec(context.Background(), `DELETE FROM conversations WHERE account_id = $1`, accountID)
		pool.Exec(context.Background(), `DELETE FROM contacts WHERE account_id = $1`, accountID)
		pool.Exec(context.Background(), `DELETE FROM channels WHERE account_id = $1`, accountID)
		pool.Exec(context.Background(), `DELETE FROM users WHERE account_id = $1`, accountID)
		pool.Exec(context.Background(), `DELETE FROM accounts WHERE id = $1`, accountID)
	})

	return accountID, userID
}

func TestHandler_Endpoints(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	pool := testPool(t)
	accountID, userID := setupTestTenant(t, pool, "handler-test")

	redisAddr := os.Getenv("REDIS_URL")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}
	ps, err := pubsub.NewClient(redisAddr)
	require.NoError(t, err)
	defer ps.Close()

	cipher, err := crypto.NewCipherFromHex("test-key-exactly-32-bytes-padded")
	require.NoError(t, err)

	svc := service.New(pool, cipher, ps)
	fakeAdapter := fakeadapter.New()
	svc.RegisterAdapter("matrix_whatsapp", fakeAdapter)

	sess := &mockSessionStore{
		userID:    userID,
		accountID: accountID,
		role:      "admin",
		loggedIn:  true,
	}

	h := handler.New(svc, sess)
	r := mux.NewRouter()
	h.RegisterRoutes(r)

	// 1. POST /channels (Create Channel)
	createBody := map[string]any{
		"type":              "matrix_whatsapp",
		"bridge_identity":   "@whatsapp:matrix.org",
		"bridge_credentials": map[string]string{"session_data": "xyz"},
	}
	jsonBody, _ := json.Marshal(createBody)

	req, _ := http.NewRequest(http.MethodPost, "/channels", bytes.NewBuffer(jsonBody))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusCreated, rr.Code)
	var channel types.Channel
	err = json.Unmarshal(rr.Body.Bytes(), &channel)
	require.NoError(t, err)
	assert.NotEmpty(t, channel.ID)
	assert.Equal(t, "matrix_whatsapp", channel.Type)

	// 2. GET /channels/{id} (Get Channel Status)
	req, _ = http.NewRequest(http.MethodGet, "/channels/"+channel.ID.String(), nil)
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	var status types.ChannelStatus
	err = json.Unmarshal(rr.Body.Bytes(), &status)
	require.NoError(t, err)
	assert.Equal(t, "connected", status.Status)

	// 3. POST /internal/conversations/{id}/send
	var contactID uuid.UUID
	err = pool.QueryRow(context.Background(), `
		INSERT INTO contacts (account_id, channel_id, external_identity, display_name)
		VALUES ($1, $2, 'alice-jid', 'Alice') RETURNING id
	`, accountID, channel.ID).Scan(&contactID)
	require.NoError(t, err)

	var convoID uuid.UUID
	err = pool.QueryRow(context.Background(), `
		INSERT INTO conversations (account_id, contact_id, channel_id)
		VALUES ($1, $2, $3) RETURNING id
	`, accountID, contactID, channel.ID).Scan(&convoID)
	require.NoError(t, err)

	sendBody := map[string]any{
		"content_type":   "text",
		"text":           "Hello from endpoint",
		"sender_type":    "human",
		"sender_user_id": userID.String(),
	}
	sendJSON, _ := json.Marshal(sendBody)

	req, _ = http.NewRequest(http.MethodPost, "/internal/conversations/"+convoID.String()+"/send", bytes.NewBuffer(sendJSON))
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	var sentMsg types.Message
	err = json.Unmarshal(rr.Body.Bytes(), &sentMsg)
	require.NoError(t, err)

	var contentMap map[string]string
	err = json.Unmarshal(sentMsg.Content, &contentMap)
	require.NoError(t, err)
	assert.Equal(t, "Hello from endpoint", contentMap["text"])

	// 4. POST /channels/{id}/disconnect
	req, _ = http.NewRequest(http.MethodPost, "/channels/"+channel.ID.String()+"/disconnect", nil)
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	var discResp map[string]string
	err = json.Unmarshal(rr.Body.Bytes(), &discResp)
	require.NoError(t, err)
	assert.Equal(t, "disconnected", discResp["status"])

	// 5. POST /webhooks/telegram?channel_id={id}
	tgPayload := `{
		"update_id": 99999,
		"message": {
			"message_id": 101,
			"from": {
				"id": 888777,
				"first_name": "TgCustomer"
			},
			"chat": {
				"id": 888777,
				"type": "private"
			},
			"date": 1723878000,
			"text": "Hello from native Telegram webhook!"
		}
	}`
	req, _ = http.NewRequest(http.MethodPost, "/webhooks/telegram?channel_id="+channel.ID.String(), bytes.NewBufferString(tgPayload))
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	var whResp map[string]any
	err = json.Unmarshal(rr.Body.Bytes(), &whResp)
	require.NoError(t, err)
	assert.Equal(t, "ok", whResp["status"])
}


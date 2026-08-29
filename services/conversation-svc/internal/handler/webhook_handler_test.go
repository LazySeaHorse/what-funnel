package handler_test

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	fakeadapter "github.com/whatfunnel/whatfunnel/adapters/fake"
	"github.com/whatfunnel/whatfunnel/packages/go-common/crypto"
	"github.com/whatfunnel/whatfunnel/packages/go-common/pubsub"
	"github.com/whatfunnel/whatfunnel/services/conversation-svc/internal/handler"
	"github.com/whatfunnel/whatfunnel/services/conversation-svc/internal/service"
)

func TestWebhookHandler_VerificationAndSignatures(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	pool := testPool(t)
	accountID, userID := setupTestTenant(t, pool, "webhook-handler-test")

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
	svc.RegisterAdapter("matrix_telegram", fakeAdapter)

	sess := &mockSessionStore{
		userID:    userID,
		accountID: accountID,
		role:      "admin",
		loggedIn:  true,
	}

	h := handler.New(svc, sess)
	router := mux.NewRouter()
	h.RegisterRoutes(router)

	ctx := context.Background()

	// 1. Create WhatsApp channel with credentials
	waCreds := map[string]string{
		"app_secret":   "meta_app_secret_12345",
		"verify_token": "meta_verify_token_abc",
	}
	waCredsJSON, _ := json.Marshal(waCreds)
	waChan, err := svc.CreateChannel(ctx, accountID, "matrix_whatsapp", nil, waCredsJSON)
	require.NoError(t, err)

	// 2. Create Telegram channel with credentials
	tgCreds := map[string]string{
		"secret_token": "tg_secret_token_999",
	}
	tgCredsJSON, _ := json.Marshal(tgCreds)
	tgChan, err := svc.CreateChannel(ctx, accountID, "matrix_telegram", nil, tgCredsJSON)
	require.NoError(t, err)

	// --- Test Meta GET Verification Handshake ---
	t.Run("Meta GET Challenge Success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/webhooks/whatsapp/"+waChan.ID.String()+"?hub.mode=subscribe&hub.verify_token=meta_verify_token_abc&hub.challenge=test_challenge_123", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "test_challenge_123", rec.Body.String())
	})

	t.Run("Meta GET Challenge Token Mismatch", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/webhooks/whatsapp/"+waChan.ID.String()+"?hub.mode=subscribe&hub.verify_token=wrong_token&hub.challenge=test_challenge_123", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusForbidden, rec.Code)
	})

	// --- Test WhatsApp POST Signature Verification ---
	waPayload := []byte(`{
		"object": "whatsapp_business_account",
		"entry": [{
			"id": "biz_123",
			"changes": [{
				"value": {
					"messaging_product": "whatsapp",
					"contacts": [{"profile": {"name": "Test User"}, "wa_id": "123456"}],
					"messages": [{"from": "123456", "id": "msg_001", "timestamp": "1723878000", "type": "text", "text": {"body": "Hello"}}]
				},
				"field": "messages"
			}]
		}]
	}`)

	t.Run("WhatsApp POST with Valid Signature", func(t *testing.T) {
		mac := hmac.New(sha256.New, []byte("meta_app_secret_12345"))
		mac.Write(waPayload)
		validSig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

		req := httptest.NewRequest(http.MethodPost, "/webhooks/whatsapp/"+waChan.ID.String(), bytes.NewReader(waPayload))
		req.Header.Set("X-Hub-Signature-256", validSig)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("WhatsApp POST with Forged/Invalid Signature", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/webhooks/whatsapp/"+waChan.ID.String(), bytes.NewReader(waPayload))
		req.Header.Set("X-Hub-Signature-256", "sha256=0000000000000000000000000000000000000000000000000000000000000000")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	// --- Test Telegram POST Secret Token Verification ---
	tgPayload := []byte(`{
		"update_id": 123456,
		"message": {
			"message_id": 1,
			"from": {"id": 987, "first_name": "TgUser"},
			"chat": {"id": 987, "type": "private"},
			"date": 1723878000,
			"text": "Hello Telegram"
		}
	}`)

	t.Run("Telegram POST with Valid Secret Token", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/webhooks/telegram/"+tgChan.ID.String(), bytes.NewReader(tgPayload))
		req.Header.Set("X-Telegram-Bot-Api-Secret-Token", "tg_secret_token_999")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("Telegram POST with Invalid Secret Token", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/webhooks/telegram/"+tgChan.ID.String(), bytes.NewReader(tgPayload))
		req.Header.Set("X-Telegram-Bot-Api-Secret-Token", "wrong_secret")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})
}

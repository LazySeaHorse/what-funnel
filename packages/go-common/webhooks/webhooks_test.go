package webhooks

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestParseTelegramUpdate(t *testing.T) {
	raw := `{
		"update_id": 10002891,
		"message": {
			"message_id": 4821,
			"from": {
				"id": 987654321,
				"is_bot": false,
				"first_name": "Alice",
				"last_name": "Smith",
				"username": "alice_smith"
			},
			"chat": {
				"id": 987654321,
				"type": "private",
				"first_name": "Alice",
				"last_name": "Smith"
			},
			"date": 1723878000,
			"text": "Hi from Telegram native app!"
		}
	}`

	events, err := ParseTelegramUpdate("ch-123", []byte(raw))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	ev := events[0]
	if ev.ChannelID != "ch-123" {
		t.Errorf("expected channel ID ch-123, got %s", ev.ChannelID)
	}
	if ev.Contact.ExternalIdentity != "987654321" {
		t.Errorf("expected identity 987654321, got %s", ev.Contact.ExternalIdentity)
	}
	if ev.Contact.DisplayName != "Alice Smith" {
		t.Errorf("expected Alice Smith, got %s", ev.Contact.DisplayName)
	}
	if ev.Message.Text != "Hi from Telegram native app!" {
		t.Errorf("expected text, got %s", ev.Message.Text)
	}
	if ev.Message.ExternalMessageID != "tg-987654321-4821" {
		t.Errorf("expected message ID tg-987654321-4821, got %s", ev.Message.ExternalMessageID)
	}
}

func TestParseWhatsAppWebhook(t *testing.T) {
	raw := `{
		"object": "whatsapp_business_account",
		"entry": [{
			"id": "biz_123",
			"changes": [{
				"value": {
					"messaging_product": "whatsapp",
					"contacts": [{
						"profile": { "name": "Bob Demo" },
						"wa_id": "15551234567"
					}],
					"messages": [{
						"from": "15551234567",
						"id": "wamid.12345",
						"timestamp": "1723878000",
						"text": { "body": "Hello WhatsApp" },
						"type": "text"
					}]
				},
				"field": "messages"
			}]
		}]
	}`

	events, err := ParseWhatsAppWebhook("ch-wa", []byte(raw))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	ev := events[0]
	if ev.Contact.DisplayName != "Bob Demo" {
		t.Errorf("expected Bob Demo, got %s", ev.Contact.DisplayName)
	}
	if ev.Message.Text != "Hello WhatsApp" {
		t.Errorf("expected text, got %s", ev.Message.Text)
	}
}

func TestParseMetaWebhook(t *testing.T) {
	raw := `{
		"object": "instagram",
		"entry": [{
			"id": "page_123",
			"time": 1723878000000,
			"messaging": [{
				"sender": { "id": "ig_user_999" },
				"recipient": { "id": "page_123" },
				"timestamp": 1723878000000,
				"message": {
					"mid": "mid.ig.123",
					"text": "Hello Instagram"
				}
			}]
		}]
	}`

	events, err := ParseMetaWebhook("ch-ig", []byte(raw))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	ev := events[0]
	if ev.Contact.ExternalIdentity != "ig_user_999" {
		t.Errorf("expected ig_user_999, got %s", ev.Contact.ExternalIdentity)
	}
	if ev.Message.Text != "Hello Instagram" {
		t.Errorf("expected text, got %s", ev.Message.Text)
	}
}

func TestVerifyMetaSignature(t *testing.T) {
	appSecret := "meta_test_secret_123"
	payload := []byte(`{"entry":[{"id":"123"}]}`)

	// Valid signature: HMAC-SHA256 of payload with appSecret
	// echo -n '{"entry":[{"id":"123"}]}' | openssl dgst -sha256 -hmac "meta_test_secret_123"
	// -> 8ea71ea68efbbce7e340c497491cf0eb2fe8e9aa5d7d3d1921316b24d77517c2
	mac := hmac.New(sha256.New, []byte(appSecret))
	mac.Write(payload)
	validSig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	if err := VerifyMetaSignature(payload, validSig, appSecret); err != nil {
		t.Fatalf("expected valid signature to pass, got: %v", err)
	}

	// Invalid signature
	if err := VerifyMetaSignature(payload, "sha256=badhexsignature123456", appSecret); err == nil {
		t.Fatal("expected invalid signature to fail")
	}

	// Missing prefix
	if err := VerifyMetaSignature(payload, "badprefixsignature", appSecret); err == nil {
		t.Fatal("expected signature without prefix to fail")
	}

	// Missing header
	if err := VerifyMetaSignature(payload, "", appSecret); err == nil {
		t.Fatal("expected empty header to fail")
	}

	// Empty secret
	if err := VerifyMetaSignature(payload, validSig, ""); err == nil {
		t.Fatal("expected empty secret to fail")
	}
}

func TestVerifyTelegramSecret(t *testing.T) {
	secret := "telegram_secret_token_xyz"

	if err := VerifyTelegramSecret("telegram_secret_token_xyz", secret); err != nil {
		t.Fatalf("expected valid secret to pass, got: %v", err)
	}

	if err := VerifyTelegramSecret("wrong_secret", secret); err == nil {
		t.Fatal("expected wrong secret to fail")
	}

	if err := VerifyTelegramSecret("", secret); err == nil {
		t.Fatal("expected empty header to fail")
	}
}

func TestVerifyMetaChallenge(t *testing.T) {
	expectedToken := "my_verify_token"
	challenge := "challenge_code_123"

	res, err := VerifyMetaChallenge("subscribe", "my_verify_token", expectedToken, challenge)
	if err != nil || res != challenge {
		t.Fatalf("expected challenge match, got res=%s, err=%v", res, err)
	}

	// Wrong token
	if _, err := VerifyMetaChallenge("subscribe", "wrong_token", expectedToken, challenge); err == nil {
		t.Fatal("expected wrong token to fail")
	}

	// Wrong mode
	if _, err := VerifyMetaChallenge("other_mode", expectedToken, expectedToken, challenge); err == nil {
		t.Fatal("expected wrong mode to fail")
	}
}

package webhooks

import (
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

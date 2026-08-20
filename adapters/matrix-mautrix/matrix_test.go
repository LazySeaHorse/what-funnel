package matrix

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRegistrationMACUsesSynapseSharedSecretFormat(t *testing.T) {
	assert.Equal(t,
		"2d26005ec6579eb9293b5aeb2a3eefd4427fa361",
		registrationMAC("registration-secret", "nonce-123", "wf_test", "password-123"),
	)
}

func TestMatrixAdapter_NormalizeEvent(t *testing.T) {
	adapter := New()

	// 1. Test Text Message Event
	content := map[string]any{
		"msgtype": "m.text",
		"body":    "Hello, world!",
	}
	ev := adapter.NormalizeEvent("chan-1", "!room:matrix.org", "$msg-1", "@alice:matrix.org", "m.room.message", 1620000000000, content)

	assert.Equal(t, "chan-1", ev.ChannelID)
	assert.Equal(t, "!room:matrix.org", ev.ExternalThreadID)
	assert.Equal(t, "@alice:matrix.org", ev.Contact.ExternalIdentity)
	assert.Equal(t, "text", ev.Message.ContentType)
	assert.Equal(t, "Hello, world!", ev.Message.Text)
	assert.Equal(t, "$msg-1", ev.Message.ExternalMessageID)
	assert.Equal(t, time.UnixMilli(1620000000000), ev.Timestamp)

	// 2. Test Image Message Event
	content = map[string]any{
		"msgtype": "m.image",
		"body":    "image.png",
		"url":     "mxc://matrix.org/abc",
	}
	ev = adapter.NormalizeEvent("chan-1", "!room:matrix.org", "$msg-2", "@alice:matrix.org", "m.room.message", 1620000000000, content)
	assert.Equal(t, "image", ev.Message.ContentType)
	assert.Equal(t, "mxc://matrix.org/abc", ev.Message.MediaURL)

	// 3. Test Reaction Event
	content = map[string]any{
		"m.relates_to": map[string]any{
			"rel_type": "m.annotation",
			"event_id": "$parent-id",
			"key":      "👍",
		},
	}
	ev = adapter.NormalizeEvent("chan-1", "!room:matrix.org", "$msg-3", "@alice:matrix.org", "m.reaction", 1620000000000, content)
	assert.Equal(t, "reaction", ev.Message.ContentType)
	assert.Equal(t, "👍", ev.Message.Text)
	assert.Equal(t, "$parent-id", ev.Message.ReplyToExternalID)

	// 4. Test WhatsApp Contact Card
	content = map[string]any{
		"vcard": "BEGIN:VCARD\nFN:Alice\nEND:VCARD",
	}
	ev = adapter.NormalizeEvent("chan-1", "!room:matrix.org", "$msg-4", "@alice:matrix.org", "net.maunium.whatsapp.contact", 1620000000000, content)
	assert.Equal(t, "contact", ev.Message.ContentType)
	assert.Equal(t, "BEGIN:VCARD\nFN:Alice\nEND:VCARD", ev.Message.Text)

	// 5. Test Fallback for unrecognized type
	content = map[string]any{
		"msgtype": "m.unsupported",
		"body":    "something",
	}
	ev = adapter.NormalizeEvent("chan-1", "!room:matrix.org", "$msg-5", "@alice:matrix.org", "m.room.message", 1620000000000, content)
	assert.Equal(t, "document", ev.Message.ContentType)
}

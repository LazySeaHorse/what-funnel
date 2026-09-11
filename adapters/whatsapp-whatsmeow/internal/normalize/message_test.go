package normalize

import (
	"testing"
	"time"

	"github.com/whatfunnel/whatfunnel/packages/go-common/messaging"
	waCommon "go.mau.fi/whatsmeow/proto/waCommon"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

func TestMessage(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name            string
		message         *waE2E.Message
		isGroup         bool
		isViewOnce      bool
		expectedKind    messaging.EventKind
		expectedType    messaging.ContentType
		expectedText    string
		expectedReplyID string
		expectedOK      bool
	}{
		{
			name:         "text",
			message:      &waE2E.Message{Conversation: proto.String("hello")},
			expectedKind: messaging.EventMessageCreated,
			expectedType: messaging.ContentText,
			expectedText: "hello",
			expectedOK:   true,
		},
		{
			name: "image with reply",
			message: &waE2E.Message{ImageMessage: &waE2E.ImageMessage{
				Caption:       proto.String("receipt"),
				Mimetype:      proto.String("image/jpeg"),
				FileLength:    proto.Uint64(1024),
				DirectPath:    proto.String("/media/path"),
				MediaKey:      []byte("key"),
				FileSHA256:    []byte("plain"),
				FileEncSHA256: []byte("encrypted"),
				ContextInfo:   &waE2E.ContextInfo{StanzaID: proto.String("quoted-1")},
			}},
			expectedKind:    messaging.EventMessageCreated,
			expectedType:    messaging.ContentImage,
			expectedText:    "receipt",
			expectedReplyID: "quoted-1",
			expectedOK:      true,
		},
		{
			name:         "view once becomes notice",
			message:      &waE2E.Message{ImageMessage: &waE2E.ImageMessage{}},
			isViewOnce:   true,
			expectedKind: messaging.EventMessageCreated,
			expectedType: messaging.ContentNotice,
			expectedText: "Open WhatsApp to view this one-time message.",
			expectedOK:   true,
		},
		{
			name:       "group ignored",
			message:    &waE2E.Message{Conversation: proto.String("hello group")},
			isGroup:    true,
			expectedOK: false,
		},
		{
			name: "reaction",
			message: &waE2E.Message{ReactionMessage: &waE2E.ReactionMessage{
				Key:  &waCommon.MessageKey{ID: proto.String("target-1")},
				Text: proto.String("👍"),
			}},
			expectedKind: messaging.EventReactionChanged,
			expectedOK:   true,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			event := &events.Message{
				Info: types.MessageInfo{
					MessageSource: types.MessageSource{
						Chat:    types.NewJID("15551234567", types.DefaultUserServer),
						Sender:  types.NewJID("15551234567", types.DefaultUserServer),
						IsGroup: test.isGroup,
					},
					ID:        "provider-message-1",
					PushName:  "Customer",
					Timestamp: now,
				},
				Message:    test.message,
				IsViewOnce: test.isViewOnce,
			}

			result, ok := Message("channel-1", event)
			if ok != test.expectedOK {
				t.Fatalf("Message() ok = %v, want %v", ok, test.expectedOK)
			}
			if !ok {
				return
			}
			if result.Event.Kind != test.expectedKind {
				t.Errorf("kind = %q, want %q", result.Event.Kind, test.expectedKind)
			}
			if test.expectedType != "" && result.Event.Message.ContentType != test.expectedType {
				t.Errorf("content type = %q, want %q", result.Event.Message.ContentType, test.expectedType)
			}
			if test.expectedText != "" && result.Event.Message.Text != test.expectedText {
				t.Errorf("text = %q, want %q", result.Event.Message.Text, test.expectedText)
			}
			if test.expectedReplyID != "" && result.Event.Message.ReplyToProviderID != test.expectedReplyID {
				t.Errorf("reply id = %q, want %q", result.Event.Message.ReplyToProviderID, test.expectedReplyID)
			}
			if err := result.Event.Validate(); err != nil {
				t.Errorf("event validation error = %v", err)
			}
		})
	}
}

func TestReceipt(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	events := Receipt("channel-1", &events.Receipt{
		MessageSource: types.MessageSource{Chat: types.NewJID("15551234567", types.DefaultUserServer)},
		MessageIDs:    []types.MessageID{"one", "two"},
		Timestamp:     now,
		Type:          types.ReceiptTypeRead,
	})

	if len(events) != 2 {
		t.Fatalf("Receipt() length = %d, want 2", len(events))
	}
	for _, event := range events {
		if event.Receipt.Status != messaging.ReceiptRead {
			t.Errorf("status = %q, want %q", event.Receipt.Status, messaging.ReceiptRead)
		}
		if err := event.Validate(); err != nil {
			t.Errorf("event validation error = %v", err)
		}
	}
}

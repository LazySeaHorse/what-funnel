package normalize

import (
	"testing"

	"github.com/whatfunnel/whatfunnel/adapters/telegram-botapi/internal/botapi"
	"github.com/whatfunnel/whatfunnel/packages/go-common/messaging"
)

func TestUpdate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		update      botapi.Update
		wantOK      bool
		wantKind    messaging.EventKind
		wantContent messaging.ContentType
		wantText    string
	}{
		{
			name: "private text", wantOK: true, wantKind: messaging.EventMessageCreated,
			wantContent: messaging.ContentText, wantText: "hello",
			update: botapi.Update{UpdateID: 7, Message: &botapi.Message{MessageID: 11, Date: 100, Chat: botapi.Chat{ID: 42, Type: "private"}, From: &botapi.User{ID: 42, FirstName: "Ada"}, Text: "hello"}},
		},
		{
			name: "edited caption", wantOK: true, wantKind: messaging.EventMessageEdited,
			wantContent: messaging.ContentImage, wantText: "new caption",
			update: botapi.Update{UpdateID: 8, EditedMessage: &botapi.Message{MessageID: 12, Date: 100, EditDate: 101, Chat: botapi.Chat{ID: 42, Type: "private"}, Photo: []botapi.PhotoSize{{FileRef: botapi.FileRef{FileID: "small", FileSize: 10}}, {FileRef: botapi.FileRef{FileID: "large", FileSize: 20}}}, Caption: "new caption"}},
		},
		{
			name: "unsupported private message", wantOK: true, wantKind: messaging.EventMessageCreated,
			wantContent: messaging.ContentNotice, wantText: "Open Telegram to view this unsupported message.",
			update: botapi.Update{UpdateID: 9, Message: &botapi.Message{MessageID: 13, Date: 100, Chat: botapi.Chat{ID: 42, Type: "private"}}},
		},
		{
			name: "oversized media is notice", wantOK: true, wantKind: messaging.EventMessageCreated,
			wantContent: messaging.ContentNotice, wantText: "This media is larger than 20 MiB. Open Telegram to view it.",
			update: botapi.Update{UpdateID: 10, Message: &botapi.Message{MessageID: 14, Date: 100, Chat: botapi.Chat{ID: 42, Type: "private"}, Document: &botapi.Document{FileRef: botapi.FileRef{FileID: "large", FileSize: messaging.MaxMediaBytes + 1}}}},
		},
		{
			name: "group ignored", wantOK: false,
			update: botapi.Update{UpdateID: 11, Message: &botapi.Message{MessageID: 15, Date: 100, Chat: botapi.Chat{ID: -100, Type: "supergroup"}, Text: "ignore"}},
		},
		{
			name: "channel ignored", wantOK: false,
			update: botapi.Update{UpdateID: 12, Message: &botapi.Message{MessageID: 16, Date: 100, Chat: botapi.Chat{ID: -200, Type: "channel"}, Text: "ignore"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			event, ok := Update("channel-1", tt.update)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if err := event.Validate(); err != nil {
				t.Fatalf("event validation: %v", err)
			}
			if event.Kind != tt.wantKind || event.Message.ContentType != tt.wantContent || event.Message.Text != tt.wantText {
				t.Fatalf("event = %#v", event)
			}
			if tt.name == "edited caption" && event.Message.Media.ProviderRef != "large" {
				t.Fatalf("provider ref = %q, want largest photo", event.Message.Media.ProviderRef)
			}
		})
	}
}

func TestUpdateReaction(t *testing.T) {
	t.Parallel()
	event, ok := Update("channel-1", botapi.Update{UpdateID: 20, MessageReaction: &botapi.MessageReactionUpdated{
		Chat: botapi.Chat{ID: 42, Type: "private"}, MessageID: 11, Date: 100,
		User: &botapi.User{ID: 42, FirstName: "Ada"}, NewReaction: []botapi.ReactionType{{Type: "emoji", Emoji: "👍"}},
	}})
	if !ok || event.Kind != messaging.EventReactionChanged || event.Reaction.Emoji != "👍" {
		t.Fatalf("event = %#v, ok = %v", event, ok)
	}
	if err := event.Validate(); err != nil {
		t.Fatalf("event validation: %v", err)
	}
}

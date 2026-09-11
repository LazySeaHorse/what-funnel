package normalize

import (
	"fmt"
	"math"
	"time"

	"github.com/whatfunnel/whatfunnel/packages/go-common/messaging"
	"go.mau.fi/whatsmeow"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types/events"
)

type Result struct {
	Event        messaging.Event
	Downloadable whatsmeow.DownloadableMessage
}

func Message(channelID string, event *events.Message) (Result, bool) {
	if event == nil || event.Message == nil || event.Info.IsGroup {
		return Result{}, false
	}

	if event.Message.GetReactionMessage() != nil {
		return reaction(channelID, event), true
	}

	if isRevoke(event.Message) {
		return deleted(channelID, event), true
	}

	message := baseMessage(event)
	kind := messaging.EventMessageCreated
	if event.IsEdit {
		kind = messaging.EventMessageEdited
	}

	result := Result{}
	if event.IsViewOnce {
		message.ContentType = messaging.ContentNotice
		message.Text = "Open WhatsApp to view this one-time message."
		message.NoticeCode = "view_once"
	} else {
		result.Downloadable = content(event.Message, &message)
	}
	if message.Media != nil && message.Media.SizeBytes > messaging.MaxMediaBytes {
		message.ContentType = messaging.ContentNotice
		message.Text = "This media is larger than 20 MiB. Open WhatsApp to view it."
		message.NoticeCode = "media_too_large"
		message.Media = nil
		result.Downloadable = nil
	}

	if message.ContentType == "" {
		message.ContentType = messaging.ContentNotice
		message.Text = "Open WhatsApp to view this unsupported message."
		message.NoticeCode = "unsupported"
	}

	result.Event = envelope(channelID, event.Info.ID, kind, event.Info.Timestamp)
	result.Event.Message = &message
	return result, true
}

func Receipt(channelID string, event *events.Receipt) []messaging.Event {
	if event == nil || event.IsGroup {
		return []messaging.Event{}
	}

	status, ok := receiptStatus(string(event.Type))
	if !ok {
		return []messaging.Event{}
	}

	result := make([]messaging.Event, 0, len(event.MessageIDs))
	for _, messageID := range event.MessageIDs {
		eventID := fmt.Sprintf("receipt:%s:%s:%s", messageID, status, event.Timestamp.UTC().Format(time.RFC3339Nano))
		normalized := envelope(channelID, eventID, messaging.EventReceiptChanged, event.Timestamp)
		normalized.Receipt = &messaging.Receipt{
			ProviderMessageID: string(messageID),
			Status:            status,
			ProviderTimestamp: event.Timestamp,
		}
		result = append(result, normalized)
	}
	return result
}

func baseMessage(event *events.Message) messaging.Message {
	direction := messaging.DirectionInbound
	if event.Info.IsFromMe {
		direction = messaging.DirectionOutbound
	}

	senderID := event.Info.Sender.String()
	if senderID == "" {
		senderID = event.Info.Chat.String()
	}

	return messaging.Message{
		ProviderMessageID: string(event.Info.ID),
		ExternalThreadID:  event.Info.Chat.String(),
		Direction:         direction,
		Sender: messaging.Sender{
			ExternalID:  senderID,
			DisplayName: event.Info.PushName,
		},
		ProviderTimestamp: event.Info.Timestamp,
	}
}

func content(source *waE2E.Message, target *messaging.Message) whatsmeow.DownloadableMessage {
	if text := source.GetConversation(); text != "" {
		target.ContentType = messaging.ContentText
		target.Text = text
		return nil
	}
	if text := source.GetExtendedTextMessage(); text != nil {
		target.ContentType = messaging.ContentText
		target.Text = text.GetText()
		target.ReplyToProviderID = text.GetContextInfo().GetStanzaID()
		return nil
	}
	if image := source.GetImageMessage(); image != nil {
		setMedia(target, messaging.ContentImage, image.GetCaption(), image.GetMimetype(), image.GetFileLength())
		target.ReplyToProviderID = image.GetContextInfo().GetStanzaID()
		return image
	}
	if video := source.GetVideoMessage(); video != nil {
		setMedia(target, messaging.ContentVideo, video.GetCaption(), video.GetMimetype(), video.GetFileLength())
		target.ReplyToProviderID = video.GetContextInfo().GetStanzaID()
		return video
	}
	if audio := source.GetAudioMessage(); audio != nil {
		setMedia(target, messaging.ContentAudio, "", audio.GetMimetype(), audio.GetFileLength())
		target.ReplyToProviderID = audio.GetContextInfo().GetStanzaID()
		return audio
	}
	if document := source.GetDocumentMessage(); document != nil {
		setMedia(target, messaging.ContentDocument, document.GetCaption(), document.GetMimetype(), document.GetFileLength())
		target.Media.Filename = document.GetFileName()
		target.ReplyToProviderID = document.GetContextInfo().GetStanzaID()
		return document
	}
	return nil
}

func setMedia(target *messaging.Message, contentType messaging.ContentType, caption, mimeType string, size uint64) {
	target.ContentType = contentType
	target.Text = caption
	if size > math.MaxInt64 {
		size = math.MaxInt64
	}
	target.Media = &messaging.Media{
		ProviderRef: string(target.ProviderMessageID),
		MIMEType:    mimeType,
		SizeBytes:   int64(size),
	}
}

func reaction(channelID string, event *events.Message) Result {
	reactionMessage := event.Message.GetReactionMessage()
	normalized := envelope(channelID, event.Info.ID, messaging.EventReactionChanged, event.Info.Timestamp)
	normalized.Reaction = &messaging.Reaction{
		ProviderMessageID: reactionMessage.GetKey().GetID(),
		SenderExternalID:  event.Info.Sender.String(),
		Emoji:             reactionMessage.GetText(),
		Removed:           reactionMessage.GetText() == "",
		ProviderTimestamp: event.Info.Timestamp,
	}
	return Result{Event: normalized}
}

func deleted(channelID string, event *events.Message) Result {
	protocol := event.Message.GetProtocolMessage()
	normalized := envelope(channelID, event.Info.ID, messaging.EventMessageDeleted, event.Info.Timestamp)
	normalized.Message = &messaging.Message{
		ProviderMessageID: protocol.GetKey().GetID(),
		ExternalThreadID:  event.Info.Chat.String(),
		ProviderTimestamp: event.Info.Timestamp,
	}
	return Result{Event: normalized}
}

func isRevoke(message *waE2E.Message) bool {
	protocol := message.GetProtocolMessage()
	return protocol != nil && protocol.GetType() == waE2E.ProtocolMessage_REVOKE
}

func envelope(channelID string, providerID any, kind messaging.EventKind, timestamp time.Time) messaging.Event {
	return messaging.Event{
		SchemaVersion: messaging.SchemaVersion,
		ID:            fmt.Sprintf("whatsapp:%s:%s:%v", channelID, kind, providerID),
		Kind:          kind,
		Provider:      messaging.ProviderWhatsApp,
		ChannelID:     channelID,
		OccurredAt:    timestamp,
	}
}

func receiptStatus(receiptType string) (messaging.ReceiptStatus, bool) {
	switch receiptType {
	case "":
		return messaging.ReceiptDelivered, true
	case "read", "read-self", "played":
		return messaging.ReceiptRead, true
	default:
		return "", false
	}
}

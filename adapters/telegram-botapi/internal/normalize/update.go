// Package normalize translates Telegram-native updates into the provider-neutral
// messaging contract.
package normalize

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/whatfunnel/whatfunnel/adapters/telegram-botapi/internal/botapi"
	"github.com/whatfunnel/whatfunnel/packages/go-common/messaging"
)

func Update(channelID string, update botapi.Update) (messaging.Event, bool) {
	if update.Message != nil {
		return message(channelID, update.UpdateID, messaging.EventMessageCreated, update.Message)
	}
	if update.EditedMessage != nil {
		return message(channelID, update.UpdateID, messaging.EventMessageEdited, update.EditedMessage)
	}
	if update.MessageReaction != nil {
		return reaction(channelID, update.UpdateID, update.MessageReaction)
	}
	return messaging.Event{}, false
}

func message(channelID string, updateID int64, kind messaging.EventKind, source *botapi.Message) (messaging.Event, bool) {
	if source == nil || source.Chat.Type != "private" {
		return messaging.Event{}, false
	}
	timestamp := time.Unix(source.Date, 0).UTC()
	if source.EditDate != 0 {
		timestamp = time.Unix(source.EditDate, 0).UTC()
	}
	direction := messaging.DirectionInbound
	if source.From != nil && source.From.IsBot {
		direction = messaging.DirectionOutbound
	}
	normalized := messaging.Message{
		ProviderMessageID: strconv.FormatInt(source.MessageID, 10),
		ExternalThreadID:  strconv.FormatInt(source.Chat.ID, 10),
		Direction:         direction,
		Sender:            sender(source),
		ProviderTimestamp: timestamp,
	}
	if source.ReplyToMessage != nil {
		normalized.ReplyToProviderID = strconv.FormatInt(source.ReplyToMessage.MessageID, 10)
	}
	setContent(&normalized, source)
	if normalized.Media != nil && normalized.Media.SizeBytes > messaging.MaxMediaBytes {
		normalized.ContentType = messaging.ContentNotice
		normalized.Text = "This media is larger than 20 MiB. Open Telegram to view it."
		normalized.NoticeCode = "media_too_large"
		normalized.Media = nil
	}
	if normalized.ContentType == "" {
		normalized.ContentType = messaging.ContentNotice
		normalized.Text = "Open Telegram to view this unsupported message."
		normalized.NoticeCode = "unsupported"
	}
	event := envelope(channelID, updateID, kind, timestamp)
	event.Message = &normalized
	return event, true
}

func sender(source *botapi.Message) messaging.Sender {
	if source.From == nil {
		return messaging.Sender{ExternalID: strconv.FormatInt(source.Chat.ID, 10)}
	}
	name := strings.TrimSpace(source.From.FirstName + " " + source.From.LastName)
	return messaging.Sender{
		ExternalID:  strconv.FormatInt(source.From.ID, 10),
		DisplayName: name,
	}
}

func setContent(target *messaging.Message, source *botapi.Message) {
	if source.Text != "" {
		target.ContentType = messaging.ContentText
		target.Text = source.Text
		return
	}
	if len(source.Photo) > 0 {
		photo := source.Photo[len(source.Photo)-1]
		setMedia(target, messaging.ContentImage, source.Caption, photo.FileRef, "image/jpeg", "")
		return
	}
	if source.Video != nil {
		setMedia(target, messaging.ContentVideo, source.Caption, source.Video.FileRef, source.Video.MIMEType, source.Video.FileName)
		return
	}
	if source.Audio != nil {
		setMedia(target, messaging.ContentAudio, source.Caption, source.Audio.FileRef, source.Audio.MIMEType, source.Audio.FileName)
		return
	}
	if source.Voice != nil {
		setMedia(target, messaging.ContentAudio, source.Caption, source.Voice.FileRef, source.Voice.MIMEType, "voice.ogg")
		return
	}
	if source.Document != nil {
		setMedia(target, messaging.ContentDocument, source.Caption, source.Document.FileRef, source.Document.MIMEType, source.Document.FileName)
	}
}

func setMedia(target *messaging.Message, contentType messaging.ContentType, caption string, file botapi.FileRef, mimeType, filename string) {
	target.ContentType = contentType
	target.Text = caption
	target.Media = &messaging.Media{
		ProviderRef: file.FileID,
		Filename:    filename,
		MIMEType:    mimeType,
		SizeBytes:   file.FileSize,
	}
}

func reaction(channelID string, updateID int64, source *botapi.MessageReactionUpdated) (messaging.Event, bool) {
	if source == nil || source.Chat.Type != "private" || source.User == nil {
		return messaging.Event{}, false
	}
	timestamp := time.Unix(source.Date, 0).UTC()
	if len(source.NewReaction) > 0 && source.NewReaction[0].Type != "emoji" {
		notice := messaging.Message{
			ProviderMessageID: fmt.Sprintf("reaction-notice-%d", updateID),
			ExternalThreadID:  strconv.FormatInt(source.Chat.ID, 10),
			Direction:         messaging.DirectionInbound,
			Sender: messaging.Sender{
				ExternalID:  strconv.FormatInt(source.User.ID, 10),
				DisplayName: strings.TrimSpace(source.User.FirstName + " " + source.User.LastName),
			},
			ContentType:       messaging.ContentNotice,
			Text:              "A reaction changed. Open Telegram to view it.",
			NoticeCode:        "unsupported_reaction",
			ProviderTimestamp: timestamp,
		}
		event := envelope(channelID, updateID, messaging.EventMessageCreated, timestamp)
		event.Message = &notice
		return event, true
	}
	reaction := &messaging.Reaction{
		ProviderMessageID: strconv.FormatInt(source.MessageID, 10),
		SenderExternalID:  strconv.FormatInt(source.User.ID, 10),
		ProviderTimestamp: timestamp,
		Removed:           len(source.NewReaction) == 0,
	}
	if len(source.NewReaction) > 0 {
		reaction.Emoji = source.NewReaction[0].Emoji
	}
	event := envelope(channelID, updateID, messaging.EventReactionChanged, timestamp)
	event.Reaction = reaction
	return event, true
}

func envelope(channelID string, updateID int64, kind messaging.EventKind, timestamp time.Time) messaging.Event {
	return messaging.Event{
		SchemaVersion: messaging.SchemaVersion,
		ID:            fmt.Sprintf("telegram:%s:update:%d:%s", channelID, updateID, kind),
		Kind:          kind,
		Provider:      messaging.ProviderTelegram,
		ChannelID:     channelID,
		OccurredAt:    timestamp,
	}
}

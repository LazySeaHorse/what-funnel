package session

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/whatfunnel/whatfunnel/adapters/telegram-botapi/internal/botapi"
	"github.com/whatfunnel/whatfunnel/packages/go-common/messaging"
)

type MediaFile struct {
	Data     []byte
	MIMEType string
	Filename string
}

type MediaSource interface {
	Fetch(context.Context, string) (MediaFile, error)
}

func (m *Manager) SetMediaSource(source MediaSource) {
	// The source is set once during startup before commands are consumed.
	m.mediaSource = source
}

func (m *Manager) Send(ctx context.Context, command messaging.Command) error {
	if err := command.Validate(); err != nil || command.Provider != messaging.ProviderTelegram {
		return errors.New("telegram session: unsupported command")
	}
	session, err := m.get(command.ChannelID)
	if err != nil {
		return err
	}
	if session.copySnapshot().State != messaging.ConnectionConnected {
		return ErrNotConnected
	}
	session.sendMu.Lock()
	defer session.sendMu.Unlock()
	completed, err := m.commandCompleted(ctx, command.ID)
	if err != nil || completed {
		return err
	}

	event, providerMessageID, err := m.executeCommand(ctx, session, command)
	if err != nil {
		var apiErr *botapi.Error
		if errors.As(err, &apiErr) && apiErr.Permanent() {
			return m.completePermanentFailure(ctx, command, apiErr)
		}
		return err
	}
	return m.completeCommandWithEvent(ctx, command.ID, providerMessageID, event)
}

func (m *Manager) executeCommand(ctx context.Context, session *botSession, command messaging.Command) (messaging.Event, string, error) {
	switch command.Kind {
	case messaging.CommandSendMessage:
		return m.sendMessage(ctx, session, command)
	case messaging.CommandEditMessage:
		return m.editMessage(ctx, session, command)
	case messaging.CommandDeleteMessage:
		return m.deleteMessage(ctx, session, command)
	case messaging.CommandChangeReaction:
		return m.changeReaction(ctx, session, command)
	default:
		return messaging.Event{}, "", errors.New("telegram session: unsupported command")
	}
}

func (m *Manager) sendMessage(ctx context.Context, session *botSession, command messaging.Command) (messaging.Event, string, error) {
	message := command.Message
	chatID, err := privateChatID(message.ExternalThreadID)
	if err != nil {
		return messaging.Event{}, "", err
	}
	params := map[string]any{"chat_id": chatID}
	if message.ReplyToProviderID != "" {
		replyID, err := providerMessageID(message.ReplyToProviderID)
		if err != nil {
			return messaging.Event{}, "", err
		}
		params["reply_parameters"] = map[string]any{"message_id": replyID, "allow_sending_without_reply": true}
	}

	var sent botapi.Message
	if message.ContentType == messaging.ContentText {
		params["text"] = message.Text
		sent, err = m.api.SendJSON(ctx, session.token, "sendMessage", params)
	} else {
		sent, err = m.sendMedia(ctx, session, *message, params)
	}
	if err != nil {
		return messaging.Event{}, "", err
	}
	providerID := strconv.FormatInt(sent.MessageID, 10)
	timestamp := time.Unix(sent.Date, 0).UTC()
	if sent.Date == 0 {
		timestamp = time.Now().UTC()
	}
	normalized := *message
	normalized.ProviderMessageID = providerID
	normalized.ProviderTimestamp = timestamp
	event := messaging.Event{
		SchemaVersion: messaging.SchemaVersion,
		ID:            fmt.Sprintf("telegram:%s:command:%s:message.created", command.ChannelID, command.ID),
		Kind:          messaging.EventMessageCreated,
		Provider:      messaging.ProviderTelegram,
		ChannelID:     command.ChannelID,
		CorrelationID: command.MessageID,
		OccurredAt:    timestamp,
		Message:       &normalized,
	}
	return event, providerID, nil
}

func (m *Manager) sendMedia(ctx context.Context, session *botSession, message messaging.Message, params map[string]any) (botapi.Message, error) {
	if m.mediaSource == nil || message.Media == nil || strings.TrimSpace(message.Media.ID) == "" {
		return botapi.Message{}, errors.New("telegram session: outbound media is unavailable")
	}
	file, err := m.mediaSource.Fetch(ctx, message.Media.ID)
	if err != nil {
		return botapi.Message{}, fmt.Errorf("fetch outbound media: %w", err)
	}
	if len(file.Data) > int(messaging.MaxMediaBytes) {
		return botapi.Message{}, messaging.ErrMediaTooLarge
	}
	method, field := telegramMediaMethod(message.ContentType, file.MIMEType)
	if method == "" {
		return botapi.Message{}, fmt.Errorf("telegram session: unsupported media type %q", message.ContentType)
	}
	filename := file.Filename
	if filename == "" {
		filename = message.Media.Filename
	}
	if filename == "" {
		filename = "attachment"
	}
	mimeType := file.MIMEType
	if mimeType == "" {
		mimeType = http.DetectContentType(file.Data)
	}
	fields := make(map[string]string, len(params)+1)
	for key, value := range params {
		encoded, marshalErr := json.Marshal(value)
		if marshalErr != nil {
			return botapi.Message{}, fmt.Errorf("encode telegram media field %s: %w", key, marshalErr)
		}
		if text, ok := value.(string); ok {
			fields[key] = text
		} else {
			fields[key] = string(encoded)
		}
	}
	if message.Text != "" {
		fields["caption"] = message.Text
	}
	return m.api.SendMedia(ctx, session.token, method, field, filename, mimeType, file.Data, fields)
}

func telegramMediaMethod(contentType messaging.ContentType, mimeType string) (string, string) {
	switch contentType {
	case messaging.ContentImage:
		return "sendPhoto", "photo"
	case messaging.ContentVideo:
		return "sendVideo", "video"
	case messaging.ContentAudio:
		if mimeType == "audio/ogg" || mimeType == "audio/opus" {
			return "sendVoice", "voice"
		}
		return "sendAudio", "audio"
	case messaging.ContentDocument:
		return "sendDocument", "document"
	default:
		return "", ""
	}
}

func (m *Manager) editMessage(ctx context.Context, session *botSession, command messaging.Command) (messaging.Event, string, error) {
	message := command.Message
	chatID, err := privateChatID(message.ExternalThreadID)
	if err != nil {
		return messaging.Event{}, "", err
	}
	messageID, err := providerMessageID(message.ProviderMessageID)
	if err != nil {
		return messaging.Event{}, "", err
	}
	params := map[string]any{"chat_id": chatID, "message_id": messageID}
	method := "editMessageText"
	if message.ContentType == messaging.ContentText {
		params["text"] = message.Text
	} else {
		method = "editMessageCaption"
		params["caption"] = message.Text
	}
	if _, err := m.api.SendJSON(ctx, session.token, method, params); err != nil {
		return messaging.Event{}, "", err
	}
	timestamp := time.Now().UTC()
	message.ProviderTimestamp = timestamp
	event := messaging.Event{SchemaVersion: messaging.SchemaVersion,
		ID:   fmt.Sprintf("telegram:%s:command:%s:message.edited", command.ChannelID, command.ID),
		Kind: messaging.EventMessageEdited, Provider: messaging.ProviderTelegram, ChannelID: command.ChannelID,
		CorrelationID: command.MessageID, OccurredAt: timestamp, Message: message,
	}
	return event, message.ProviderMessageID, nil
}

func (m *Manager) deleteMessage(ctx context.Context, session *botSession, command messaging.Command) (messaging.Event, string, error) {
	message := command.Message
	chatID, err := privateChatID(message.ExternalThreadID)
	if err != nil {
		return messaging.Event{}, "", err
	}
	messageID, err := providerMessageID(message.ProviderMessageID)
	if err != nil {
		return messaging.Event{}, "", err
	}
	if err := m.api.Call(ctx, session.token, "deleteMessage", map[string]any{"chat_id": chatID, "message_id": messageID}); err != nil {
		return messaging.Event{}, "", err
	}
	timestamp := time.Now().UTC()
	message.ProviderTimestamp = timestamp
	event := messaging.Event{SchemaVersion: messaging.SchemaVersion,
		ID:   fmt.Sprintf("telegram:%s:command:%s:message.deleted", command.ChannelID, command.ID),
		Kind: messaging.EventMessageDeleted, Provider: messaging.ProviderTelegram, ChannelID: command.ChannelID,
		CorrelationID: command.MessageID, OccurredAt: timestamp, Message: message,
	}
	return event, message.ProviderMessageID, nil
}

func (m *Manager) changeReaction(ctx context.Context, session *botSession, command messaging.Command) (messaging.Event, string, error) {
	reaction := command.Reaction
	messageID, err := providerMessageID(reaction.ProviderMessageID)
	if err != nil {
		return messaging.Event{}, "", err
	}
	// Reaction commands use SenderExternalID as the direct chat identity.
	chatID, err := privateChatID(reaction.SenderExternalID)
	if err != nil {
		return messaging.Event{}, "", err
	}
	params := map[string]any{"chat_id": chatID, "message_id": messageID, "reaction": []any{}}
	if !reaction.Removed {
		params["reaction"] = []map[string]string{{"type": "emoji", "emoji": reaction.Emoji}}
	}
	if err := m.api.Call(ctx, session.token, "setMessageReaction", params); err != nil {
		return messaging.Event{}, "", err
	}
	timestamp := time.Now().UTC()
	normalized := *reaction
	normalized.ProviderTimestamp = timestamp
	normalized.SenderExternalID = "business"
	event := messaging.Event{SchemaVersion: messaging.SchemaVersion,
		ID:   fmt.Sprintf("telegram:%s:command:%s:reaction.changed", command.ChannelID, command.ID),
		Kind: messaging.EventReactionChanged, Provider: messaging.ProviderTelegram, ChannelID: command.ChannelID,
		CorrelationID: command.MessageID, OccurredAt: timestamp, Reaction: &normalized,
	}
	return event, reaction.ProviderMessageID, nil
}

func privateChatID(value string) (int64, error) {
	id, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.New("telegram session: invalid private chat id")
	}
	return id, nil
}

func providerMessageID(value string) (int64, error) {
	id, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.New("telegram session: invalid provider message id")
	}
	return id, nil
}

func (m *Manager) commandCompleted(ctx context.Context, commandID string) (bool, error) {
	var exists int
	err := m.db.QueryRowContext(ctx, `SELECT 1 FROM adapter_commands WHERE command_id = ?`, commandID).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check completed telegram command: %w", err)
	}
	return true, nil
}

func (m *Manager) completePermanentFailure(ctx context.Context, command messaging.Command, apiErr *botapi.Error) error {
	detail := "Telegram rejected this message."
	if apiErr.ErrorCode == http.StatusForbidden {
		detail = "Telegram rejected this message. The user may have blocked the bot."
	}
	timestamp := time.Now().UTC()
	event := messaging.Event{
		SchemaVersion: messaging.SchemaVersion,
		ID:            fmt.Sprintf("telegram:%s:command:%s:receipt.failed", command.ChannelID, command.ID),
		Kind:          messaging.EventReceiptChanged,
		Provider:      messaging.ProviderTelegram,
		ChannelID:     command.ChannelID,
		CorrelationID: command.MessageID,
		OccurredAt:    timestamp,
		Receipt: &messaging.Receipt{
			ProviderMessageID: "unsent:" + command.ID,
			Status:            messaging.ReceiptFailed, Detail: detail, ProviderTimestamp: timestamp,
		},
	}
	return m.completeCommandWithEvent(ctx, command.ID, "unsent", event)
}

func (m *Manager) completeCommandWithEvent(ctx context.Context, commandID, providerID string, event messaging.Event) error {
	if err := event.Validate(); err != nil {
		return fmt.Errorf("validate completed telegram event: %w", err)
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal completed telegram event: %w", err)
	}
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin completed telegram command: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO adapter_commands (command_id, provider_message_id)
		VALUES (?, ?) ON CONFLICT(command_id) DO NOTHING
	`, commandID, providerID); err != nil {
		return fmt.Errorf("record completed telegram command: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO adapter_event_outbox (event_id, payload, available_at)
		VALUES (?, ?, ?) ON CONFLICT(event_id) DO NOTHING
	`, event.ID, payload, time.Now().UnixMilli()); err != nil {
		return fmt.Errorf("record completed telegram event: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit completed telegram command: %w", err)
	}
	m.wakePublisher()
	return nil
}

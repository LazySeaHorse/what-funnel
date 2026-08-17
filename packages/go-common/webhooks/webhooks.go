package webhooks

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/whatfunnel/whatfunnel/packages/go-common/types"
)

// Telegram Webhook structures
type TelegramUpdate struct {
	UpdateID      int64            `json:"update_id"`
	Message       *TelegramMessage `json:"message,omitempty"`
	EditedMessage *TelegramMessage `json:"edited_message,omitempty"`
	ChannelPost   *TelegramMessage `json:"channel_post,omitempty"`
}

type TelegramUser struct {
	ID        int64  `json:"id"`
	IsBot     bool   `json:"is_bot"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name,omitempty"`
	Username  string `json:"username,omitempty"`
}

type TelegramChat struct {
	ID        int64  `json:"id"`
	Type      string `json:"type"`
	Title     string `json:"title,omitempty"`
	Username  string `json:"username,omitempty"`
	FirstName string `json:"first_name,omitempty"`
	LastName  string `json:"last_name,omitempty"`
}

type TelegramPhotoSize struct {
	FileID   string `json:"file_id"`
	FileSize int    `json:"file_size,omitempty"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
}

type TelegramMedia struct {
	FileID   string `json:"file_id"`
	FileName string `json:"file_name,omitempty"`
	MimeType string `json:"mime_type,omitempty"`
}

type TelegramMessage struct {
	MessageID int64               `json:"message_id"`
	From      *TelegramUser       `json:"from,omitempty"`
	Chat      TelegramChat        `json:"chat"`
	Date      int64               `json:"date"`
	Text      string              `json:"text,omitempty"`
	Caption   string              `json:"caption,omitempty"`
	Photo     []TelegramPhotoSize `json:"photo,omitempty"`
	Document  *TelegramMedia      `json:"document,omitempty"`
	Video     *TelegramMedia      `json:"video,omitempty"`
	Voice     *TelegramMedia      `json:"voice,omitempty"`
	Audio     *TelegramMedia      `json:"audio,omitempty"`
}

// ParseTelegramUpdate converts a native Telegram Bot API Update payload into InboundEvent(s).
func ParseTelegramUpdate(channelID string, rawPayload []byte) ([]types.InboundEvent, error) {
	var update TelegramUpdate
	if err := json.Unmarshal(rawPayload, &update); err != nil {
		return nil, fmt.Errorf("unmarshal telegram update: %w", err)
	}

	msg := update.Message
	if msg == nil {
		msg = update.EditedMessage
	}
	if msg == nil {
		msg = update.ChannelPost
	}
	if msg == nil {
		return nil, fmt.Errorf("telegram update has no message")
	}

	senderID := ""
	senderName := ""
	if msg.From != nil {
		senderID = strconv.FormatInt(msg.From.ID, 10)
		nameParts := []string{}
		if msg.From.FirstName != "" {
			nameParts = append(nameParts, msg.From.FirstName)
		}
		if msg.From.LastName != "" {
			nameParts = append(nameParts, msg.From.LastName)
		}
		senderName = strings.TrimSpace(strings.Join(nameParts, " "))
		if senderName == "" && msg.From.Username != "" {
			senderName = msg.From.Username
		}
	} else {
		senderID = strconv.FormatInt(msg.Chat.ID, 10)
		senderName = msg.Chat.Title
	}
	if senderName == "" {
		senderName = fmt.Sprintf("Telegram User %s", senderID)
	}

	threadID := strconv.FormatInt(msg.Chat.ID, 10)
	extMsgID := fmt.Sprintf("tg-%d-%d", msg.Chat.ID, msg.MessageID)

	contentType := "text"
	text := msg.Text
	mediaURL := ""

	if len(msg.Photo) > 0 {
		contentType = "image"
		mediaURL = msg.Photo[len(msg.Photo)-1].FileID
		text = msg.Caption
	} else if msg.Video != nil {
		contentType = "video"
		mediaURL = msg.Video.FileID
		text = msg.Caption
	} else if msg.Voice != nil || msg.Audio != nil {
		contentType = "audio"
		if msg.Voice != nil {
			mediaURL = msg.Voice.FileID
		} else {
			mediaURL = msg.Audio.FileID
		}
		text = msg.Caption
	} else if msg.Document != nil {
		contentType = "document"
		mediaURL = msg.Document.FileID
		text = msg.Caption
	}

	ts := time.Now()
	if msg.Date > 0 {
		ts = time.Unix(msg.Date, 0)
	}

	event := types.InboundEvent{
		ChannelID:        channelID,
		ExternalThreadID: threadID,
		Contact: types.ContactRef{
			ExternalIdentity: senderID,
			DisplayName:      senderName,
		},
		Message: types.NormalizedMessage{
			ContentType:       contentType,
			Text:              text,
			MediaURL:          mediaURL,
			ExternalMessageID: extMsgID,
		},
		Timestamp: ts,
	}

	return []types.InboundEvent{event}, nil
}

// WhatsApp Webhook structures (Meta WhatsApp Cloud API)
type WhatsAppWebhookPayload struct {
	Object string          `json:"object"`
	Entry  []WhatsAppEntry `json:"entry"`
}

type WhatsAppEntry struct {
	ID      string           `json:"id"`
	Changes []WhatsAppChange `json:"changes"`
}

type WhatsAppChange struct {
	Value WhatsAppValue `json:"value"`
	Field string          `json:"field"`
}

type WhatsAppValue struct {
	MessagingProduct string            `json:"messaging_product"`
	Metadata         WhatsAppMetadata  `json:"metadata"`
	Contacts         []WhatsAppContact `json:"contacts"`
	Messages         []WhatsAppMessage `json:"messages"`
}

type WhatsAppMetadata struct {
	DisplayPhoneNumber string `json:"display_phone_number"`
	PhoneNumberID      string `json:"phone_number_id"`
}

type WhatsAppContact struct {
	Profile WhatsAppProfile `json:"profile"`
	WaID    string          `json:"wa_id"`
}

type WhatsAppProfile struct {
	Name string `json:"name"`
}

type WhatsAppMedia struct {
	ID       string `json:"id"`
	MimeType string `json:"mime_type,omitempty"`
	Caption  string `json:"caption,omitempty"`
}

type WhatsAppMessage struct {
	From      string                 `json:"from"`
	ID        string                 `json:"id"`
	Timestamp string                 `json:"timestamp"`
	Type      string                 `json:"type"`
	Text      *WhatsAppText          `json:"text,omitempty"`
	Image     *WhatsAppMedia         `json:"image,omitempty"`
	Video     *WhatsAppMedia         `json:"video,omitempty"`
	Audio     *WhatsAppMedia         `json:"audio,omitempty"`
	Document  *WhatsAppMedia         `json:"document,omitempty"`
}

type WhatsAppText struct {
	Body string `json:"body"`
}

// ParseWhatsAppWebhook converts a native Meta WhatsApp Cloud API payload into InboundEvent(s).
func ParseWhatsAppWebhook(channelID string, rawPayload []byte) ([]types.InboundEvent, error) {
	var payload WhatsAppWebhookPayload
	if err := json.Unmarshal(rawPayload, &payload); err != nil {
		return nil, fmt.Errorf("unmarshal whatsapp payload: %w", err)
	}

	var events []types.InboundEvent
	for _, entry := range payload.Entry {
		for _, change := range entry.Changes {
			contactsMap := make(map[string]string)
			for _, contact := range change.Value.Contacts {
				if contact.Profile.Name != "" {
					contactsMap[contact.WaID] = contact.Profile.Name
				}
			}

			for _, msg := range change.Value.Messages {
				senderID := msg.From
				senderName := contactsMap[senderID]
				if senderName == "" {
					senderName = senderID
				}

				contentType := "text"
				text := ""
				mediaURL := ""

				switch msg.Type {
				case "image":
					contentType = "image"
					if msg.Image != nil {
						mediaURL = msg.Image.ID
						text = msg.Image.Caption
					}
				case "video":
					contentType = "video"
					if msg.Video != nil {
						mediaURL = msg.Video.ID
						text = msg.Video.Caption
					}
				case "audio":
					contentType = "audio"
					if msg.Audio != nil {
						mediaURL = msg.Audio.ID
					}
				case "document":
					contentType = "document"
					if msg.Document != nil {
						mediaURL = msg.Document.ID
						text = msg.Document.Caption
					}
				default:
					contentType = "text"
					if msg.Text != nil {
						text = msg.Text.Body
					}
				}

				ts := time.Now()
				if msg.Timestamp != "" {
					if sec, err := strconv.ParseInt(msg.Timestamp, 10, 64); err == nil {
						ts = time.Unix(sec, 0)
					}
				}

				events = append(events, types.InboundEvent{
					ChannelID:        channelID,
					ExternalThreadID: senderID,
					Contact: types.ContactRef{
						ExternalIdentity: senderID,
						DisplayName:      senderName,
					},
					Message: types.NormalizedMessage{
						ContentType:       contentType,
						Text:              text,
						MediaURL:          mediaURL,
						ExternalMessageID: msg.ID,
					},
					Timestamp: ts,
				})
			}
		}
	}

	return events, nil
}

// Meta (Instagram / Messenger) Graph Webhook structures
type MetaWebhookPayload struct {
	Object string      `json:"object"` // "instagram" or "page"
	Entry  []MetaEntry `json:"entry"`
}

type MetaEntry struct {
	ID        string          `json:"id"`
	Time      int64           `json:"time"`
	Messaging []MetaMessaging `json:"messaging"`
}

type MetaParticipant struct {
	ID string `json:"id"`
}

type MetaMessaging struct {
	Sender    MetaParticipant `json:"sender"`
	Recipient MetaParticipant `json:"recipient"`
	Timestamp int64           `json:"timestamp"`
	Message   *MetaMessage    `json:"message,omitempty"`
}

type MetaAttachmentPayload struct {
	URL string `json:"url"`
}

type MetaAttachment struct {
	Type    string                `json:"type"`
	Payload MetaAttachmentPayload `json:"payload"`
}

type MetaMessage struct {
	MID         string           `json:"mid"`
	Text        string           `json:"text,omitempty"`
	Attachments []MetaAttachment `json:"attachments,omitempty"`
}

// ParseMetaWebhook converts a native Meta Instagram or Messenger Webhook payload into InboundEvent(s).
func ParseMetaWebhook(channelID string, rawPayload []byte) ([]types.InboundEvent, error) {
	var payload MetaWebhookPayload
	if err := json.Unmarshal(rawPayload, &payload); err != nil {
		return nil, fmt.Errorf("unmarshal meta webhook payload: %w", err)
	}

	var events []types.InboundEvent
	for _, entry := range payload.Entry {
		for _, m := range entry.Messaging {
			if m.Message == nil {
				continue
			}

			senderID := m.Sender.ID
			senderName := senderID
			if payload.Object == "instagram" {
				senderName = fmt.Sprintf("ig_%s", senderID)
			}

			contentType := "text"
			text := m.Message.Text
			mediaURL := ""

			if len(m.Message.Attachments) > 0 {
				att := m.Message.Attachments[0]
				mediaURL = att.Payload.URL
				switch att.Type {
				case "image":
					contentType = "image"
				case "video":
					contentType = "video"
				case "audio":
					contentType = "audio"
				case "file":
					contentType = "document"
				default:
					contentType = "image"
				}
			}

			ts := time.Now()
			if m.Timestamp > 0 {
				ts = time.UnixMilli(m.Timestamp)
			}

			events = append(events, types.InboundEvent{
				ChannelID:        channelID,
				ExternalThreadID: senderID,
				Contact: types.ContactRef{
					ExternalIdentity: senderID,
					DisplayName:      senderName,
				},
				Message: types.NormalizedMessage{
					ContentType:       contentType,
					Text:              text,
					MediaURL:          mediaURL,
					ExternalMessageID: m.Message.MID,
				},
				Timestamp: ts,
			})
		}
	}

	return events, nil
}

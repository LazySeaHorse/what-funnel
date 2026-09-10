package webhooks

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/whatfunnel/whatfunnel/packages/go-common/types"
)

var (
	ErrMissingSignature = errors.New("missing webhook signature or secret header")
	ErrInvalidSignature = errors.New("invalid webhook signature or secret token")
	ErrInvalidChallenge = errors.New("invalid webhook verification challenge")
)

// VerifyMetaSignature validates the X-Hub-Signature-256 header from Meta (WhatsApp / Instagram / Messenger).
func VerifyMetaSignature(rawBody []byte, headerSignature, appSecret string) error {
	if headerSignature == "" || appSecret == "" {
		return ErrMissingSignature
	}

	const prefix = "sha256="
	if !strings.HasPrefix(headerSignature, prefix) {
		return ErrInvalidSignature
	}
	receivedHex := strings.TrimPrefix(headerSignature, prefix)

	mac := hmac.New(sha256.New, []byte(appSecret))
	mac.Write(rawBody)
	expectedHex := hex.EncodeToString(mac.Sum(nil))

	if subtle.ConstantTimeCompare([]byte(receivedHex), []byte(expectedHex)) != 1 {
		return ErrInvalidSignature
	}
	return nil
}

// VerifyTelegramSecret validates the X-Telegram-Bot-Api-Secret-Token header.
func VerifyTelegramSecret(headerSecret, expectedSecret string) error {
	if headerSecret == "" || expectedSecret == "" {
		return ErrMissingSignature
	}
	if subtle.ConstantTimeCompare([]byte(headerSecret), []byte(expectedSecret)) != 1 {
		return ErrInvalidSignature
	}
	return nil
}

// VerifyMetaChallenge validates a GET verification handshake request from Meta Webhooks.
func VerifyMetaChallenge(mode, verifyToken, expectedToken, challenge string) (string, error) {
	if mode != "subscribe" || expectedToken == "" || subtle.ConstantTimeCompare([]byte(verifyToken), []byte(expectedToken)) != 1 {
		return "", ErrInvalidChallenge
	}
	return challenge, nil
}

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
	Field string        `json:"field"`
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
	From      string         `json:"from"`
	ID        string         `json:"id"`
	Timestamp string         `json:"timestamp"`
	Type      string         `json:"type"`
	Text      *WhatsAppText  `json:"text,omitempty"`
	Image     *WhatsAppMedia `json:"image,omitempty"`
	Video     *WhatsAppMedia `json:"video,omitempty"`
	Audio     *WhatsAppMedia `json:"audio,omitempty"`
	Document  *WhatsAppMedia `json:"document,omitempty"`
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
			contactNames := whatsAppContactNames(change.Value.Contacts)
			for _, msg := range change.Value.Messages {
				events = append(events, whatsAppInboundEvent(channelID, contactNames, msg, time.Now()))
			}
		}
	}

	return events, nil
}

func whatsAppContactNames(contacts []WhatsAppContact) map[string]string {
	names := make(map[string]string, len(contacts))
	for _, contact := range contacts {
		if contact.Profile.Name != "" {
			names[contact.WaID] = contact.Profile.Name
		}
	}
	return names
}

func whatsAppInboundEvent(channelID string, contactNames map[string]string, msg WhatsAppMessage, fallbackTime time.Time) types.InboundEvent {
	senderName := contactNames[msg.From]
	if senderName == "" {
		senderName = msg.From
	}

	return types.InboundEvent{
		ChannelID:        channelID,
		ExternalThreadID: msg.From,
		Contact: types.ContactRef{
			ExternalIdentity: msg.From,
			DisplayName:      senderName,
		},
		Message:   normalizeWhatsAppMessage(msg),
		Timestamp: parseWhatsAppTimestamp(msg.Timestamp, fallbackTime),
	}
}

func normalizeWhatsAppMessage(msg WhatsAppMessage) types.NormalizedMessage {
	normalized := types.NormalizedMessage{
		ContentType:       "text",
		ExternalMessageID: msg.ID,
	}

	switch msg.Type {
	case "image":
		normalized.ContentType = "image"
		setWhatsAppMedia(&normalized, msg.Image, true)
	case "video":
		normalized.ContentType = "video"
		setWhatsAppMedia(&normalized, msg.Video, true)
	case "audio":
		normalized.ContentType = "audio"
		setWhatsAppMedia(&normalized, msg.Audio, false)
	case "document":
		normalized.ContentType = "document"
		setWhatsAppMedia(&normalized, msg.Document, true)
	default:
		if msg.Text != nil {
			normalized.Text = msg.Text.Body
		}
	}

	return normalized
}

func setWhatsAppMedia(msg *types.NormalizedMessage, media *WhatsAppMedia, includeCaption bool) {
	if media == nil {
		return
	}
	msg.MediaURL = media.ID
	if includeCaption {
		msg.Text = media.Caption
	}
}

func parseWhatsAppTimestamp(raw string, fallback time.Time) time.Time {
	seconds, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return fallback
	}
	return time.Unix(seconds, 0)
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

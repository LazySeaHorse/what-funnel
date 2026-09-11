// Package messaging defines the provider-neutral wire contract shared by the
// conversation service and messaging adapters.
package messaging

import "time"

const SchemaVersion = 1

type Provider string

const (
	ProviderWhatsApp Provider = "whatsapp"
	ProviderTelegram Provider = "telegram"
)

func (p Provider) Valid() bool {
	switch p {
	case ProviderWhatsApp, ProviderTelegram:
		return true
	default:
		return false
	}
}

type Direction string

const (
	DirectionInbound  Direction = "inbound"
	DirectionOutbound Direction = "outbound"
)

func (d Direction) Valid() bool {
	return d == DirectionInbound || d == DirectionOutbound
}

type ContentType string

const (
	ContentText     ContentType = "text"
	ContentImage    ContentType = "image"
	ContentVideo    ContentType = "video"
	ContentAudio    ContentType = "audio"
	ContentDocument ContentType = "document"
	ContentNotice   ContentType = "notice"
)

func (t ContentType) Valid() bool {
	switch t {
	case ContentText, ContentImage, ContentVideo, ContentAudio, ContentDocument, ContentNotice:
		return true
	default:
		return false
	}
}

type Sender struct {
	ExternalID  string `json:"external_id"`
	DisplayName string `json:"display_name,omitempty"`
	AvatarURL   string `json:"avatar_url,omitempty"`
}

type Media struct {
	ID          string `json:"id,omitempty"`
	ProviderRef string `json:"provider_ref,omitempty"`
	Filename    string `json:"filename,omitempty"`
	MIMEType    string `json:"mime_type,omitempty"`
	SizeBytes   int64  `json:"size_bytes,omitempty"`
}

type Message struct {
	ProviderMessageID string      `json:"provider_message_id,omitempty"`
	ExternalThreadID  string      `json:"external_thread_id"`
	Direction         Direction   `json:"direction"`
	Sender            Sender      `json:"sender"`
	ContentType       ContentType `json:"content_type"`
	Text              string      `json:"text,omitempty"`
	Media             *Media      `json:"media,omitempty"`
	ReplyToProviderID string      `json:"reply_to_provider_id,omitempty"`
	ProviderTimestamp time.Time   `json:"provider_timestamp"`
	NoticeCode        string      `json:"notice_code,omitempty"`
}

type Reaction struct {
	ProviderMessageID string    `json:"provider_message_id"`
	SenderExternalID  string    `json:"sender_external_id"`
	Emoji             string    `json:"emoji,omitempty"`
	Removed           bool      `json:"removed"`
	ProviderTimestamp time.Time `json:"provider_timestamp"`
}

type ReceiptStatus string

const (
	ReceiptSent      ReceiptStatus = "sent"
	ReceiptDelivered ReceiptStatus = "delivered"
	ReceiptRead      ReceiptStatus = "read"
	ReceiptFailed    ReceiptStatus = "failed"
)

func (s ReceiptStatus) Valid() bool {
	switch s {
	case ReceiptSent, ReceiptDelivered, ReceiptRead, ReceiptFailed:
		return true
	default:
		return false
	}
}

type Receipt struct {
	ProviderMessageID string        `json:"provider_message_id"`
	Status            ReceiptStatus `json:"status"`
	Detail            string        `json:"detail,omitempty"`
	ProviderTimestamp time.Time     `json:"provider_timestamp"`
}

type ConnectionStatus string

const (
	ConnectionPending      ConnectionStatus = "pending"
	ConnectionAwaitingScan ConnectionStatus = "awaiting_scan"
	ConnectionConnecting   ConnectionStatus = "connecting"
	ConnectionConnected    ConnectionStatus = "connected"
	ConnectionDisconnected ConnectionStatus = "disconnected"
	ConnectionError        ConnectionStatus = "error"
)

func (s ConnectionStatus) Valid() bool {
	switch s {
	case ConnectionPending, ConnectionAwaitingScan, ConnectionConnecting,
		ConnectionConnected, ConnectionDisconnected, ConnectionError:
		return true
	default:
		return false
	}
}

type Status struct {
	State  ConnectionStatus `json:"state"`
	Detail string           `json:"detail,omitempty"`
}

type Capabilities struct {
	Media     bool `json:"media"`
	Replies   bool `json:"replies"`
	Reactions bool `json:"reactions"`
	Edits     bool `json:"edits"`
	Deletes   bool `json:"deletes"`
	Receipts  bool `json:"receipts"`
}

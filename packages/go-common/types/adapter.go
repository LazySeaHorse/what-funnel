package types

import (
	"context"
	"time"
)

// NormalizedMessage represents a content-normalized message payload.
type NormalizedMessage struct {
	ContentType       string // text|image|video|audio|document|reaction|location|contact
	Text              string
	MediaURL          string
	ReplyToExternalID string
	ExternalMessageID string
}

// ContactRef represents information about the contact.
type ContactRef struct {
	ExternalIdentity string
	DisplayName      string
	AvatarURL        string
}

// InboundEvent is published by adapters when a message is received.
type InboundEvent struct {
	ChannelID        string
	ExternalThreadID string
	Contact          ContactRef
	Message          NormalizedMessage
	Timestamp        time.Time
}

// ChannelStatus represents the live status of a channel connection.
type ChannelStatus struct {
	Status string // connected|disconnected|error
	Detail string
}

// ChannelAdapter defines the contract for messaging adapters.
type ChannelAdapter interface {
	Start(ctx context.Context, publish func(InboundEvent)) error
	SendMessage(ctx context.Context, channelID, externalThreadID string, msg NormalizedMessage) error
	Status(channelID string) ChannelStatus
}

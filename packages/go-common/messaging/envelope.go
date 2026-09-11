package messaging

import "time"

type EventKind string

const (
	EventMessageCreated  EventKind = "message.created"
	EventMessageEdited   EventKind = "message.edited"
	EventMessageDeleted  EventKind = "message.deleted"
	EventReactionChanged EventKind = "reaction.changed"
	EventReceiptChanged  EventKind = "receipt.changed"
	EventChannelStatus   EventKind = "channel.status.changed"
)

func (k EventKind) Valid() bool {
	switch k {
	case EventMessageCreated, EventMessageEdited, EventMessageDeleted,
		EventReactionChanged, EventReceiptChanged, EventChannelStatus:
		return true
	default:
		return false
	}
}

type Event struct {
	SchemaVersion int       `json:"schema_version"`
	ID            string    `json:"id"`
	Kind          EventKind `json:"kind"`
	Provider      Provider  `json:"provider"`
	ChannelID     string    `json:"channel_id"`
	CorrelationID string    `json:"correlation_id,omitempty"`
	OccurredAt    time.Time `json:"occurred_at"`
	Message       *Message  `json:"message,omitempty"`
	Reaction      *Reaction `json:"reaction,omitempty"`
	Receipt       *Receipt  `json:"receipt,omitempty"`
	Status        *Status   `json:"status,omitempty"`
}

type CommandKind string

const (
	CommandSendMessage    CommandKind = "message.send"
	CommandEditMessage    CommandKind = "message.edit"
	CommandDeleteMessage  CommandKind = "message.delete"
	CommandChangeReaction CommandKind = "reaction.change"
)

func (k CommandKind) Valid() bool {
	switch k {
	case CommandSendMessage, CommandEditMessage, CommandDeleteMessage, CommandChangeReaction:
		return true
	default:
		return false
	}
}

type Command struct {
	SchemaVersion int         `json:"schema_version"`
	ID            string      `json:"id"`
	Kind          CommandKind `json:"kind"`
	Provider      Provider    `json:"provider"`
	ChannelID     string      `json:"channel_id"`
	CreatedAt     time.Time   `json:"created_at"`
	MessageID     string      `json:"message_id"`
	Message       *Message    `json:"message,omitempty"`
	Reaction      *Reaction   `json:"reaction,omitempty"`
}

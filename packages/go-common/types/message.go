package types

// MessageSender identifies who authored a conversation message.
type MessageSender string

const (
	MessageSenderContact MessageSender = "contact"
	MessageSenderHuman   MessageSender = "human"
	MessageSenderAI      MessageSender = "ai"
)

func (s MessageSender) Valid() bool {
	switch s {
	case MessageSenderContact, MessageSenderHuman, MessageSenderAI:
		return true
	default:
		return false
	}
}

// MessagePurpose describes why an AI-authored message is being sent.
type MessagePurpose string

const (
	MessagePurposeReply          MessagePurpose = "reply"
	MessagePurposeHumanReviewAck MessagePurpose = "human_review_ack"
)

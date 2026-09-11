package messaging

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

const MaxMediaBytes int64 = 20 * 1024 * 1024

var (
	ErrInvalidEnvelope = errors.New("messaging: invalid envelope")
	ErrMediaTooLarge   = errors.New("messaging: media exceeds 20 mib limit")
)

func RetentionForSize(sizeBytes int64) (time.Duration, error) {
	switch {
	case sizeBytes < 0:
		return 0, ErrInvalidEnvelope
	case sizeBytes <= 1024*1024:
		return 7 * 24 * time.Hour, nil
	case sizeBytes <= 10*1024*1024:
		return 24 * time.Hour, nil
	case sizeBytes <= MaxMediaBytes:
		return time.Hour, nil
	default:
		return 0, ErrMediaTooLarge
	}
}

func (e Event) Validate() error {
	if err := validateEnvelope(e.SchemaVersion, e.ID, e.Provider, e.ChannelID); err != nil {
		return err
	}
	if !e.Kind.Valid() || e.OccurredAt.IsZero() {
		return ErrInvalidEnvelope
	}

	switch e.Kind {
	case EventMessageCreated, EventMessageEdited:
		if !onlyPayload(e.Message != nil, e.Reaction != nil, e.Receipt != nil, e.Status != nil) {
			return ErrInvalidEnvelope
		}
		return e.Message.Validate()
	case EventMessageDeleted:
		if !onlyPayload(e.Message != nil, e.Reaction != nil, e.Receipt != nil, e.Status != nil) {
			return ErrInvalidEnvelope
		}
		return e.Message.ValidateReference()
	case EventReactionChanged:
		if !onlyPayload(e.Reaction != nil, e.Message != nil, e.Receipt != nil, e.Status != nil) {
			return ErrInvalidEnvelope
		}
		return e.Reaction.Validate()
	case EventReceiptChanged:
		if !onlyPayload(e.Receipt != nil, e.Message != nil, e.Reaction != nil, e.Status != nil) {
			return ErrInvalidEnvelope
		}
		return e.Receipt.Validate()
	case EventChannelStatus:
		if !onlyPayload(e.Status != nil, e.Message != nil, e.Reaction != nil, e.Receipt != nil) {
			return ErrInvalidEnvelope
		}
		return e.Status.Validate()
	default:
		return ErrInvalidEnvelope
	}
}

func (c Command) Validate() error {
	if err := validateEnvelope(c.SchemaVersion, c.ID, c.Provider, c.ChannelID); err != nil {
		return err
	}
	if !c.Kind.Valid() || c.CreatedAt.IsZero() || strings.TrimSpace(c.MessageID) == "" {
		return ErrInvalidEnvelope
	}

	switch c.Kind {
	case CommandSendMessage, CommandEditMessage:
		if !onlyPayload(c.Message != nil, c.Reaction != nil, false, false) {
			return ErrInvalidEnvelope
		}
		return c.Message.Validate()
	case CommandDeleteMessage:
		if !onlyPayload(c.Message != nil, c.Reaction != nil, false, false) {
			return ErrInvalidEnvelope
		}
		return c.Message.ValidateReference()
	case CommandChangeReaction:
		if !onlyPayload(c.Reaction != nil, c.Message != nil, false, false) {
			return ErrInvalidEnvelope
		}
		return c.Reaction.Validate()
	default:
		return ErrInvalidEnvelope
	}
}

func (m Message) Validate() error {
	if strings.TrimSpace(m.ExternalThreadID) == "" || !m.Direction.Valid() || !m.ContentType.Valid() || m.ProviderTimestamp.IsZero() {
		return ErrInvalidEnvelope
	}
	if m.Media != nil {
		if err := m.Media.Validate(); err != nil {
			return err
		}
	}

	hasText := strings.TrimSpace(m.Text) != ""
	hasMedia := m.Media != nil
	switch m.ContentType {
	case ContentText:
		if !hasText || hasMedia {
			return ErrInvalidEnvelope
		}
	case ContentNotice:
		if !hasText || strings.TrimSpace(m.NoticeCode) == "" || hasMedia {
			return ErrInvalidEnvelope
		}
	case ContentImage, ContentVideo, ContentAudio, ContentDocument:
		if !hasMedia {
			return ErrInvalidEnvelope
		}
	default:
		return ErrInvalidEnvelope
	}
	return nil
}

func (m Message) ValidateReference() error {
	if strings.TrimSpace(m.ProviderMessageID) == "" || strings.TrimSpace(m.ExternalThreadID) == "" || m.ProviderTimestamp.IsZero() {
		return ErrInvalidEnvelope
	}
	return nil
}

func (m Media) Validate() error {
	if m.SizeBytes < 0 {
		return ErrInvalidEnvelope
	}
	if m.SizeBytes > MaxMediaBytes {
		return ErrMediaTooLarge
	}
	if strings.TrimSpace(m.ID) == "" && strings.TrimSpace(m.ProviderRef) == "" {
		return ErrInvalidEnvelope
	}
	return nil
}

func (r Reaction) Validate() error {
	if strings.TrimSpace(r.ProviderMessageID) == "" || strings.TrimSpace(r.SenderExternalID) == "" || r.ProviderTimestamp.IsZero() {
		return ErrInvalidEnvelope
	}
	if !r.Removed && strings.TrimSpace(r.Emoji) == "" {
		return ErrInvalidEnvelope
	}
	return nil
}

func (r Receipt) Validate() error {
	if strings.TrimSpace(r.ProviderMessageID) == "" || !r.Status.Valid() || r.ProviderTimestamp.IsZero() {
		return ErrInvalidEnvelope
	}
	return nil
}

func (s Status) Validate() error {
	if !s.State.Valid() {
		return ErrInvalidEnvelope
	}
	return nil
}

func validateEnvelope(schemaVersion int, id string, provider Provider, channelID string) error {
	if schemaVersion != SchemaVersion {
		return fmt.Errorf("%w: unsupported schema version %d", ErrInvalidEnvelope, schemaVersion)
	}
	if strings.TrimSpace(id) == "" || !provider.Valid() || strings.TrimSpace(channelID) == "" {
		return ErrInvalidEnvelope
	}
	return nil
}

func onlyPayload(isExpectedSet, isFirstUnexpectedSet, isSecondUnexpectedSet, isThirdUnexpectedSet bool) bool {
	return isExpectedSet && !isFirstUnexpectedSet && !isSecondUnexpectedSet && !isThirdUnexpectedSet
}

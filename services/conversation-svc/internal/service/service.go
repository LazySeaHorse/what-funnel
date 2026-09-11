package service

import (
	"context"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/whatfunnel/whatfunnel/packages/go-common/messaging"
	"github.com/whatfunnel/whatfunnel/packages/go-common/pubsub"

	"github.com/google/uuid"
)

// Service is the central business-logic object for the conversation service.
// Domain operations live in domain-specific files:
//   - adapter.go     – adapter registry, credential encryption
//   - channel.go     – channel CRUD, status sync
//   - conversation.go – conversation queries, RBAC, message pagination
//   - ingest.go      – inbound / outbound message ingestion
//   - lead.go        – lead lifecycle, notes, history
//   - provider_connection.go – provider adapter setup lifecycle
type Service struct {
	pool          *pgxpool.Pool
	pubsub        *pubsub.Client
	controls      map[messaging.Provider]AdapterControl
	controlsMu    sync.RWMutex
	mediaRoot     string
	mediaFetchers map[messaging.Provider]ProviderMediaFetcher
	mediaMu       sync.RWMutex
}

func New(pool *pgxpool.Pool, pubsub *pubsub.Client) *Service {
	return &Service{
		pool:          pool,
		pubsub:        pubsub,
		controls:      make(map[messaging.Provider]AdapterControl),
		mediaFetchers: make(map[messaging.Provider]ProviderMediaFetcher),
	}
}

type ProviderMedia struct {
	Data     []byte
	MIMEType string
	Filename string
}

type ProviderMediaFetcher interface {
	Download(context.Context, string, string) (ProviderMedia, error)
}

func (s *Service) PubSub() *pubsub.Client {
	return s.pubsub
}

func (s *Service) Pool() *pgxpool.Pool {
	return s.pool
}

// ---------------------------------------------------------------------------
// Shared pubsub event types
// ---------------------------------------------------------------------------

// ConversationUpdatedEvent is published to the conversation.updated stream.
type ConversationUpdatedEvent struct {
	AccountID      uuid.UUID `json:"account_id"`
	ConversationID uuid.UUID `json:"conversation_id"`
	MessageID      uuid.UUID `json:"message_id"`
}

// ConversationAssignedEvent is published to the conversation.assigned stream.
type ConversationAssignedEvent struct {
	AccountID       uuid.UUID   `json:"account_id"`
	ConversationID  uuid.UUID   `json:"conversation_id"`
	AssignedUserIDs []uuid.UUID `json:"assigned_user_ids"`
}

// AIReplyDraftUpdatedEvent clears or updates pending-draft state in connected
// inbox clients after a draft is used, dismissed, or superseded.
type AIReplyDraftUpdatedEvent struct {
	AccountID      uuid.UUID  `json:"account_id"`
	ConversationID uuid.UUID  `json:"conversation_id"`
	DraftID        *uuid.UUID `json:"draft_id,omitempty"`
	Action         string     `json:"action"`
}

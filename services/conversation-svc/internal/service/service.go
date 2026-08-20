package service

import (
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/whatfunnel/whatfunnel/packages/go-common/crypto"
	"github.com/whatfunnel/whatfunnel/packages/go-common/pubsub"
	"github.com/whatfunnel/whatfunnel/packages/go-common/types"

	"github.com/google/uuid"
)

// Service is the central business-logic object for the conversation service.
// Domain operations live in domain-specific files:
//   - adapter.go     – adapter registry, credential encryption
//   - channel.go     – channel CRUD, status sync
//   - conversation.go – conversation queries, RBAC, message pagination
//   - ingest.go      – inbound / outbound message ingestion
//   - lead.go        – lead lifecycle, notes, history
//   - bridge_connection.go – Matrix bridge setup lifecycle
type Service struct {
	pool         *pgxpool.Pool
	cipher       *crypto.Cipher
	pubsub       *pubsub.Client
	adapters     map[string]types.ChannelAdapter
	adaptersMu   sync.RWMutex
	bridgeConfig BridgeConnectionConfig
}

func New(pool *pgxpool.Pool, cipher *crypto.Cipher, pubsub *pubsub.Client) *Service {
	return &Service{
		pool:     pool,
		cipher:   cipher,
		pubsub:   pubsub,
		adapters: make(map[string]types.ChannelAdapter),
	}
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

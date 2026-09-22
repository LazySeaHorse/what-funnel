package service

import (
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/whatfunnel/whatfunnel/packages/go-common/pubsub"
)

// Service is the central business-logic façade composing focused domain services:
//   - ConnectionService   – adapter registry, channel CRUD, provider connection lifecycle
//   - MediaService        – disk caching, media fetching and downloads
//   - LeadService         – lead lifecycle, notes, history
//   - OutboxService       – transactional outbox command dispatch
//   - AIDraftService      – AI controls and reply draft lifecycle
//   - ConversationService – conversation queries, RBAC, outbound messaging, edits, deletes
//   - IngestionService    – inbound and external outbound message ingestion
type Service struct {
	pool   *pgxpool.Pool
	pubsub *pubsub.Client

	*ConnectionService
	*MediaService
	*LeadService
	*OutboxService
	*AIDraftService
	*ConversationService
	*IngestionService
}

func New(pool *pgxpool.Pool, pubsub *pubsub.Client) *Service {
	pipelineResolver := &DBLeadPipelineResolver{}
	connSvc := NewConnectionService(pool)
	mediaSvc := NewMediaService(pool)
	outboxSvc := NewOutboxService(pool, pubsub)
	leadSvc := NewLeadService(pool, pubsub, pipelineResolver)
	aiDraftSvc := NewAIDraftService(pool, pubsub)
	convoSvc := NewConversationService(pool, pubsub, outboxSvc)
	ingestSvc := NewIngestionService(pool, pubsub, pipelineResolver)

	return &Service{
		pool:                pool,
		pubsub:              pubsub,
		ConnectionService:   connSvc,
		MediaService:        mediaSvc,
		LeadService:         leadSvc,
		OutboxService:       outboxSvc,
		AIDraftService:      aiDraftSvc,
		ConversationService: convoSvc,
		IngestionService:    ingestSvc,
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

// AIReplyDraftUpdatedEvent clears or updates pending-draft state in connected
// inbox clients after a draft is used, dismissed, or superseded.
type AIReplyDraftUpdatedEvent struct {
	AccountID      uuid.UUID  `json:"account_id"`
	ConversationID uuid.UUID  `json:"conversation_id"`
	DraftID        *uuid.UUID `json:"draft_id,omitempty"`
	Action         string     `json:"action"`
}

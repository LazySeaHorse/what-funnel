package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/whatfunnel/whatfunnel/packages/go-common/messaging"
	"github.com/whatfunnel/whatfunnel/packages/go-common/pubsub"
)

type IngestionService struct {
	pool             *pgxpool.Pool
	pubsub           *pubsub.Client
	pipelineResolver PipelineResolver
}

func NewIngestionService(pool *pgxpool.Pool, pubsub *pubsub.Client, resolver PipelineResolver) *IngestionService {
	if resolver == nil {
		resolver = &DBLeadPipelineResolver{}
	}
	return &IngestionService{
		pool:             pool,
		pubsub:           pubsub,
		pipelineResolver: resolver,
	}
}

// SimulateInbound publishes a provider-neutral adapter event. It exercises the
// same validation and ingestion boundary as a real adapter without contacting
// the provider.
func (s *IngestionService) SimulateInbound(
	ctx context.Context,
	accountID uuid.UUID,
	channelID string,
	senderExternalID string,
	senderDisplayName string,
	senderAvatarURL string,
	contentType string,
	text string,
	mediaURL string,
) error {
	var provider messaging.Provider
	if err := s.pool.QueryRow(ctx,
		`SELECT provider FROM channels WHERE id = $1 AND account_id = $2`,
		channelID, accountID,
	).Scan(&provider); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return notFoundf("channel not found or not owned by account")
		}
		return fmt.Errorf("channel lookup failed: %w", err)
	}
	if !provider.Valid() {
		return invalidf("channel does not use a supported provider adapter")
	}

	now := time.Now().UTC()
	eventID := "sim-" + uuid.NewString()
	message := &messaging.Message{
		ProviderMessageID: eventID,
		ExternalThreadID:  senderExternalID,
		Direction:         messaging.DirectionInbound,
		Sender: messaging.Sender{
			ExternalID: senderExternalID, DisplayName: senderDisplayName, AvatarURL: senderAvatarURL,
		},
		ContentType:       messaging.ContentType(contentType),
		Text:              text,
		ProviderTimestamp: now,
	}
	if mediaURL != "" {
		message.Media = &messaging.Media{ProviderRef: mediaURL}
	}
	event := messaging.Event{
		SchemaVersion: messaging.SchemaVersion,
		ID:            eventID,
		Kind:          messaging.EventMessageCreated,
		Provider:      provider,
		ChannelID:     channelID,
		OccurredAt:    now,
		Message:       message,
	}
	if err := event.Validate(); err != nil {
		return invalidf("invalid simulated event: %v", err)
	}

	if _, err := s.pubsub.Publish(ctx, "adapter.events", event); err != nil {
		return fmt.Errorf("publish simulated inbound event: %w", err)
	}
	return nil
}

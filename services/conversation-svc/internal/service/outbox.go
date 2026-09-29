package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/whatfunnel/whatfunnel/packages/go-common/messaging"
	"github.com/whatfunnel/whatfunnel/packages/go-common/pubsub"
)

const (
	whatsAppCommandsStream = "adapter.commands.whatsapp"
	telegramCommandsStream = "adapter.commands.telegram"
)

type OutboxService struct {
	pool   *pgxpool.Pool
	pubsub *pubsub.Client
}

func NewOutboxService(pool *pgxpool.Pool, pubsub *pubsub.Client) *OutboxService {
	return &OutboxService{
		pool:   pool,
		pubsub: pubsub,
	}
}

// maxOutboxAttempts bounds delivery retries for one command. With the capped
// exponential backoff below this is roughly half an hour of trying, after
// which the command is abandoned and its message is marked failed instead of
// staying "queued" forever.
const maxOutboxAttempts = 10

const outboxFailureDetail = "Could not deliver this message to the messaging provider."

type claimedCommand struct {
	id       uuid.UUID
	provider messaging.Provider
	command  messaging.Command
}

// DispatchOutbox runs until ctx is cancelled and delivers committed commands
// to the provider streams. Database claims expire so another replica can
// recover work after a crash.
func (s *OutboxService) DispatchOutbox(ctx context.Context) error {
	const (
		initialBackoff = 100 * time.Millisecond
		maxBackoff     = 5 * time.Second
	)
	backoff := initialBackoff
	timer := time.NewTimer(backoff)
	if !timer.Stop() {
		<-timer.C
	}
	defer timer.Stop()

	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		dispatched, err := s.dispatchOutboxOnce(ctx, "")
		if err != nil && ctx.Err() == nil {
			// The row is released with backoff by dispatchOutboxOnce. Keep the
			// worker alive so a transient Redis or database failure self-heals.
			fmt.Printf("failed to dispatch provider command: %v\n", err)
		}
		if dispatched {
			backoff = initialBackoff
			continue
		}

		timer.Reset(backoff)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}

		backoff = nextBackoff(backoff, maxBackoff)
	}
}

func nextBackoff(current, max time.Duration) time.Duration {
	next := current * 2
	if next > max {
		return max
	}
	return next
}

// DispatchOutboxCommand attempts to deliver one specific just-committed command
// as a low-latency nudge. If an earlier command for the same conversation has
// not been delivered yet, it is left for the background worker so order is
// preserved. The row stays queued on failure, so callers may log and continue.
func (s *OutboxService) DispatchOutboxCommand(ctx context.Context, commandID string) error {
	_, err := s.dispatchOutboxOnce(ctx, commandID)
	return err
}

// nudgeOutbox dispatches a freshly committed command and logs (never drops)
// failures: the durable row is retried by the background worker.
func (s *OutboxService) nudgeOutbox(ctx context.Context, commandID string) {
	if s == nil {
		return
	}
	if err := s.DispatchOutboxCommand(ctx, commandID); err != nil {
		slog.WarnContext(ctx, "immediate outbox dispatch failed; the worker will retry", "command_id", commandID, "error", err)
	}
}

func (s *OutboxService) dispatchOutboxOnce(ctx context.Context, commandID string) (bool, error) {
	claimID := uuid.NewString()
	claimed, err := s.claimOutboxCommand(ctx, claimID, commandID)
	if err != nil || claimed == nil {
		return false, err
	}

	stream, err := commandStream(claimed.provider)
	if err == nil {
		_, err = s.pubsub.Publish(ctx, stream, claimed.command)
	}
	if err != nil {
		releaseErr := s.releaseOutboxCommand(ctx, claimed, claimID, err)
		return true, errors.Join(err, releaseErr)
	}
	if err := s.markOutboxDispatched(ctx, claimed.id, claimID); err != nil {
		// Publishing succeeded. The expiring claim and adapter-side stable
		// provider message ID make a later redelivery safe.
		return true, err
	}
	return true, nil
}

// claimOutboxCommand claims the oldest deliverable command, or, when commandID
// is set, that specific command if it is deliverable. A command is only
// deliverable when no earlier undelivered command exists for the same
// conversation, so a backing-off command is never overtaken by a later one.
func (s *OutboxService) claimOutboxCommand(ctx context.Context, claimID, commandID string) (*claimedCommand, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin outbox claim: %w", err)
	}
	defer tx.Rollback(ctx)

	var (
		rowID       uuid.UUID
		provider    messaging.Provider
		commandJSON []byte
	)
	err = tx.QueryRow(ctx, `
		WITH candidate AS (
			SELECT o.id
			FROM message_outbox AS o
			JOIN messages AS m ON m.id = o.message_id
			WHERE o.dispatched_at IS NULL
			  AND o.available_at <= NOW()
			  AND (o.claimed_at IS NULL OR o.claimed_at < NOW() - INTERVAL '5 minutes')
			  AND ($2::TEXT = '' OR o.command->>'id' = $2::TEXT)
			  AND NOT EXISTS (
			      SELECT 1
			      FROM message_outbox AS earlier
			      JOIN messages AS earlier_message ON earlier_message.id = earlier.message_id
			      WHERE earlier.dispatched_at IS NULL
			        AND earlier_message.conversation_id = m.conversation_id
			        AND (earlier.created_at, earlier.id) < (o.created_at, o.id)
			  )
			ORDER BY o.created_at, o.id
			FOR UPDATE OF o SKIP LOCKED
			LIMIT 1
		)
		UPDATE message_outbox AS outbox
		SET claimed_at = NOW(), claimed_by = $1
		FROM candidate
		WHERE outbox.id = candidate.id
		RETURNING outbox.id, outbox.provider, outbox.command
	`, claimID, commandID).Scan(&rowID, &provider, &commandJSON)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("claim outbox command: %w", err)
	}

	var command messaging.Command
	if err := json.Unmarshal(commandJSON, &command); err != nil {
		return nil, s.rejectOutboxCommand(ctx, tx, rowID, claimID, fmt.Errorf("decode command: %w", err))
	}
	if err := command.Validate(); err != nil || command.Provider != provider {
		return nil, s.rejectOutboxCommand(ctx, tx, rowID, claimID, errors.Join(err, messaging.ErrInvalidEnvelope))
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit outbox claim: %w", err)
	}
	return &claimedCommand{id: rowID, provider: provider, command: command}, nil
}

// rejectOutboxCommand permanently consumes an internally corrupt row. Retrying
// bytes that can never form a valid command would otherwise poison the queue.
func (s *OutboxService) rejectOutboxCommand(
	ctx context.Context,
	tx pgx.Tx,
	rowID uuid.UUID,
	claimID string,
	reason error,
) error {
	detail := "Invalid provider command."
	if _, err := tx.Exec(ctx, `
		UPDATE messages
		SET delivery_status = 'failed', delivery_detail = $3
		WHERE id = (SELECT message_id FROM message_outbox WHERE id = $1 AND claimed_by = $2)
	`, rowID, claimID, detail); err != nil {
		return fmt.Errorf("fail message for invalid outbox command %s: %w", rowID, err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE message_outbox
		SET dispatched_at = NOW(), claimed_at = NULL, claimed_by = NULL,
		    last_error = $3
		WHERE id = $1 AND claimed_by = $2
	`, rowID, claimID, reason.Error()); err != nil {
		return fmt.Errorf("reject invalid outbox command %s: %w", rowID, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit invalid outbox command %s: %w", rowID, err)
	}
	return fmt.Errorf("outbox command %s was permanently rejected: %w", rowID, reason)
}

func (s *OutboxService) markOutboxDispatched(ctx context.Context, id uuid.UUID, claimID string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE message_outbox
		SET dispatched_at = NOW(), claimed_at = NULL, claimed_by = NULL, last_error = NULL
		WHERE id = $1 AND claimed_by = $2 AND dispatched_at IS NULL
	`, id, claimID)
	if err != nil {
		return fmt.Errorf("mark outbox command dispatched: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return errors.New("mark outbox command dispatched: claim was lost")
	}
	return nil
}

// releaseOutboxCommand records a failed delivery attempt and schedules a
// retry with backoff. After maxOutboxAttempts the command is abandoned: the
// row is closed (so it stops blocking the conversation's later commands) and,
// for a send, the message is marked failed so the agent sees it.
func (s *OutboxService) releaseOutboxCommand(ctx context.Context, claimed *claimedCommand, claimID string, dispatchErr error) error {
	detail := dispatchErr.Error()
	if len(detail) > 1000 {
		detail = detail[:1000]
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin release outbox command: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var attempts int
	var messageID uuid.UUID
	err = tx.QueryRow(ctx, `
		UPDATE message_outbox
		SET attempts = attempts + 1,
		    available_at = NOW() + LEAST(INTERVAL '5 minutes', INTERVAL '1 second' * POWER(2, LEAST(attempts, 8))),
		    claimed_at = NULL,
		    claimed_by = NULL,
		    last_error = $3
		WHERE id = $1 AND claimed_by = $2 AND dispatched_at IS NULL
		RETURNING attempts, message_id
	`, claimed.id, claimID, detail).Scan(&attempts, &messageID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil // The claim expired and another worker owns the row now.
	}
	if err != nil {
		return fmt.Errorf("release outbox command: %w", err)
	}

	abandoned := attempts >= maxOutboxAttempts
	var failed ConversationUpdatedEvent
	if abandoned {
		if _, err := tx.Exec(ctx, `
			UPDATE message_outbox SET dispatched_at = NOW() WHERE id = $1
		`, claimed.id); err != nil {
			return fmt.Errorf("abandon outbox command %s: %w", claimed.id, err)
		}
		if claimed.command.Kind == messaging.CommandSendMessage {
			err := tx.QueryRow(ctx, `
				UPDATE messages
				SET delivery_status = 'failed', delivery_detail = $2
				WHERE id = $1 AND delivery_status = 'queued'
				RETURNING account_id, conversation_id
			`, messageID, outboxFailureDetail).Scan(&failed.AccountID, &failed.ConversationID)
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("fail message for abandoned outbox command %s: %w", claimed.id, err)
			}
			failed.MessageID = messageID
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit release outbox command: %w", err)
	}
	if abandoned {
		slog.ErrorContext(ctx, "abandoned provider command after repeated delivery failures",
			"outbox_id", claimed.id, "command_id", claimed.command.ID, "kind", claimed.command.Kind,
			"attempts", attempts, "error", dispatchErr)
		if failed.ConversationID != uuid.Nil && s.pubsub != nil {
			if _, err := s.pubsub.Publish(ctx, "conversation.updated", failed); err != nil {
				slog.WarnContext(ctx, "failed to publish conversation.updated for failed message", "error", err)
			}
		}
	}
	return nil
}

func commandStream(provider messaging.Provider) (string, error) {
	switch provider {
	case messaging.ProviderWhatsApp:
		return whatsAppCommandsStream, nil
	case messaging.ProviderTelegram:
		return telegramCommandsStream, nil
	default:
		return "", fmt.Errorf("unsupported command provider %q", strings.TrimSpace(string(provider)))
	}
}

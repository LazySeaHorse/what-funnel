package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/whatfunnel/whatfunnel/packages/go-common/messaging"
)

const (
	whatsAppCommandsStream = "adapter.commands.whatsapp"
	telegramCommandsStream = "adapter.commands.telegram"
)

type claimedCommand struct {
	id       uuid.UUID
	provider messaging.Provider
	command  messaging.Command
}

// DispatchOutbox runs until ctx is cancelled and delivers committed commands
// to the provider streams. Database claims expire so another replica can
// recover work after a crash.
func (s *Service) DispatchOutbox(ctx context.Context) error {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		dispatched, err := s.dispatchOutboxOnce(ctx)
		if err != nil && ctx.Err() == nil {
			// The row is released with backoff by dispatchOutboxOnce. Keep the
			// worker alive so a transient Redis or database failure self-heals.
			fmt.Printf("failed to dispatch provider command: %v\n", err)
		}
		if dispatched {
			continue
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// DispatchOutboxOnce attempts one ready command. It is also used as a
// best-effort low-latency nudge after an outbound message commits.
func (s *Service) DispatchOutboxOnce(ctx context.Context) error {
	_, err := s.dispatchOutboxOnce(ctx)
	return err
}

func (s *Service) dispatchOutboxOnce(ctx context.Context) (bool, error) {
	claimID := uuid.NewString()
	claimed, err := s.claimOutboxCommand(ctx, claimID)
	if err != nil || claimed == nil {
		return false, err
	}

	stream, err := commandStream(claimed.provider)
	if err == nil {
		_, err = s.pubsub.Publish(ctx, stream, claimed.command)
	}
	if err != nil {
		releaseErr := s.releaseOutboxCommand(ctx, claimed.id, claimID, err)
		return true, errors.Join(err, releaseErr)
	}
	if err := s.markOutboxDispatched(ctx, claimed.id, claimID); err != nil {
		// Publishing succeeded. The expiring claim and adapter-side stable
		// provider message ID make a later redelivery safe.
		return true, err
	}
	return true, nil
}

func (s *Service) claimOutboxCommand(ctx context.Context, claimID string) (*claimedCommand, error) {
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
			SELECT id
			FROM message_outbox
			WHERE dispatched_at IS NULL
			  AND available_at <= NOW()
			  AND (claimed_at IS NULL OR claimed_at < NOW() - INTERVAL '5 minutes')
			ORDER BY created_at, id
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)
		UPDATE message_outbox AS outbox
		SET claimed_at = NOW(), claimed_by = $1
		FROM candidate
		WHERE outbox.id = candidate.id
		RETURNING outbox.id, outbox.provider, outbox.command
	`, claimID).Scan(&rowID, &provider, &commandJSON)
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
func (s *Service) rejectOutboxCommand(
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

func (s *Service) markOutboxDispatched(ctx context.Context, id uuid.UUID, claimID string) error {
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

func (s *Service) releaseOutboxCommand(ctx context.Context, id uuid.UUID, claimID string, dispatchErr error) error {
	detail := dispatchErr.Error()
	if len(detail) > 1000 {
		detail = detail[:1000]
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE message_outbox
		SET attempts = attempts + 1,
		    available_at = NOW() + LEAST(INTERVAL '5 minutes', INTERVAL '1 second' * POWER(2, LEAST(attempts, 8))),
		    claimed_at = NULL,
		    claimed_by = NULL,
		    last_error = $3
		WHERE id = $1 AND claimed_by = $2 AND dispatched_at IS NULL
	`, id, claimID, detail)
	if err != nil {
		return fmt.Errorf("release outbox command: %w", err)
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

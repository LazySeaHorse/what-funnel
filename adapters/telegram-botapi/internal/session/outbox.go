package session

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/whatfunnel/whatfunnel/packages/go-common/messaging"
)

func (m *Manager) enqueueEvent(ctx context.Context, event messaging.Event) error {
	if err := event.Validate(); err != nil {
		return fmt.Errorf("validate telegram event: %w", err)
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal telegram event: %w", err)
	}
	if _, err := m.db.ExecContext(ctx, `
		INSERT INTO adapter_event_outbox (event_id, payload, available_at)
		VALUES (?, ?, ?) ON CONFLICT(event_id) DO NOTHING
	`, event.ID, payload, time.Now().UnixMilli()); err != nil {
		return fmt.Errorf("enqueue telegram event: %w", err)
	}
	m.wakePublisher()
	return nil
}

func (m *Manager) wakePublisher() {
	select {
	case m.eventWake <- struct{}{}:
	default:
	}
}

func (m *Manager) publishEvents() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		if err := m.publishEventOnce(m.ctx); err != nil && m.ctx.Err() == nil {
			m.logger.Error("publish telegram adapter event", "error", err)
		}
		select {
		case <-m.ctx.Done():
			return
		case <-m.eventWake:
		case <-ticker.C:
		}
	}
}

func (m *Manager) publishEventOnce(ctx context.Context) error {
	var eventID string
	var payload []byte
	err := m.db.QueryRowContext(ctx, `
		SELECT event_id, payload FROM adapter_event_outbox
		WHERE available_at <= ? ORDER BY available_at, event_id LIMIT 1
	`, time.Now().UnixMilli()).Scan(&eventID, &payload)
	if err != nil {
		return nil
	}
	var event messaging.Event
	if err := json.Unmarshal(payload, &event); err != nil {
		_, _ = m.db.ExecContext(ctx, `DELETE FROM adapter_event_outbox WHERE event_id = ?`, eventID)
		return fmt.Errorf("decode queued telegram event: %w", err)
	}
	if err := m.publisher.Publish(ctx, event); err != nil {
		_, _ = m.db.ExecContext(ctx, `
			UPDATE adapter_event_outbox SET attempts = attempts + 1, available_at = ? WHERE event_id = ?
		`, time.Now().Add(2*time.Second).UnixMilli(), eventID)
		return err
	}
	if _, err := m.db.ExecContext(ctx, `DELETE FROM adapter_event_outbox WHERE event_id = ?`, eventID); err != nil {
		return fmt.Errorf("delete published telegram event: %w", err)
	}
	m.wakePublisher()
	return nil
}

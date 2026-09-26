package integration

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/whatfunnel/whatfunnel/packages/go-common/messaging"
)

// mockRedisPublisher simulates Redis with pause/resume capability.
type mockRedisPublisher struct {
	mu        sync.Mutex
	paused    atomic.Bool
	published []messaging.Event
}

func (p *mockRedisPublisher) Pause() {
	p.paused.Store(true)
}

func (p *mockRedisPublisher) Resume() {
	p.paused.Store(false)
}

func (p *mockRedisPublisher) Publish(_ context.Context, event messaging.Event) error {
	if p.paused.Load() {
		return errors.New("redis: connection refused (paused)")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.published = append(p.published, event)
	return nil
}

func (p *mockRedisPublisher) Count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.published)
}

// TestOutboxStress_RedisPauseResume tests that when Redis is paused under high throughput,
// SQLite buffers all events locally and replays them cleanly without data loss once Redis resumes.
func TestOutboxStress_RedisPauseResume(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "outbox_stress.db")

	db, err := sql.Open("sqlite3", fmt.Sprintf("file:%s?_busy_timeout=5000&_journal_mode=WAL", dbPath))
	require.NoError(t, err)
	defer db.Close()

	// Initialize outbox table matching adapter_event_outbox schema
	_, err = db.Exec(`
		CREATE TABLE adapter_event_outbox (
			event_id TEXT PRIMARY KEY,
			channel_id TEXT NOT NULL,
			payload BLOB NOT NULL,
			available_at INTEGER NOT NULL,
			attempts INTEGER NOT NULL DEFAULT 0
		);
	`)
	require.NoError(t, err)

	redisPub := &mockRedisPublisher{}
	wakeCh := make(chan struct{}, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	wakePublisher := func() {
		select {
		case wakeCh <- struct{}{}:
		default:
		}
	}

	// Outbox worker loop
	var workerWg sync.WaitGroup
	workerWg.Add(1)
	go func() {
		defer workerWg.Done()
		ticker := time.NewTicker(20 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-wakeCh:
			case <-ticker.C:
			}

			// Process available outbox events
			for {
				var eventID string
				var payload []byte
				rowErr := db.QueryRowContext(ctx, `
					SELECT event_id, payload FROM adapter_event_outbox
					WHERE available_at <= ? ORDER BY available_at, event_id LIMIT 1
				`, time.Now().UnixMilli()).Scan(&eventID, &payload)
				if rowErr != nil {
					break
				}

				var event messaging.Event
				if err := json.Unmarshal(payload, &event); err != nil {
					_, _ = db.ExecContext(ctx, `DELETE FROM adapter_event_outbox WHERE event_id = ?`, eventID)
					continue
				}

				if err := redisPub.Publish(ctx, event); err != nil {
					// Redis paused / down: backoff and retry later
					_, _ = db.ExecContext(ctx, `
						UPDATE adapter_event_outbox SET attempts = attempts + 1, available_at = ? WHERE event_id = ?
					`, time.Now().Add(50*time.Millisecond).UnixMilli(), eventID)
					break
				}

				// Successfully delivered: remove from SQLite buffer
				_, _ = db.ExecContext(ctx, `DELETE FROM adapter_event_outbox WHERE event_id = ?`, eventID)
			}
		}
	}()

	// 1. Pause Redis to simulate mid-traffic outage
	t.Log("Step 1: Pausing Redis...")
	redisPub.Pause()

	// 2. High throughput burst: 50 concurrent messages produced
	const messageBurst = 50
	t.Logf("Step 2: Producing burst of %d messages during Redis outage...", messageBurst)

	enqueueEvent := func(ev messaging.Event) error {
		payload, err := json.Marshal(ev)
		if err != nil {
			return err
		}
		_, err = db.ExecContext(ctx, `
			INSERT INTO adapter_event_outbox (event_id, channel_id, payload, available_at)
			VALUES (?, ?, ?, ?) ON CONFLICT(event_id) DO NOTHING
		`, ev.ID, ev.ChannelID, payload, time.Now().UnixMilli())
		wakePublisher()
		return err
	}

	var producerWg sync.WaitGroup
	for i := 0; i < messageBurst; i++ {
		producerWg.Add(1)
		go func(idx int) {
			defer producerWg.Done()
			now := time.Now().UTC()
			ev := messaging.Event{
				SchemaVersion: messaging.SchemaVersion,
				ID:            fmt.Sprintf("stress:msg:%d:%s", idx, uuid.NewString()),
				Kind:          messaging.EventMessageCreated,
				Provider:      messaging.ProviderTelegram,
				ChannelID:     "chan-stress-1",
				OccurredAt:    now,
				Message: &messaging.Message{
					ProviderMessageID: fmt.Sprintf("tg_msg_%d", idx),
					ExternalThreadID:  fmt.Sprintf("chat_%d", idx),
					Direction:         messaging.DirectionInbound,
					Sender:            messaging.Sender{ExternalID: fmt.Sprintf("user_%d", idx)},
					ContentType:       messaging.ContentText,
					Text:              fmt.Sprintf("High throughput stress message %d", idx),
					ProviderTimestamp: now,
				},
			}
			_ = enqueueEvent(ev)
		}(i)
	}
	producerWg.Wait()

	// 3. Assert all 50 messages are buffered locally in SQLite and 0 delivered to Redis
	var bufferedCount int
	err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM adapter_event_outbox`).Scan(&bufferedCount)
	require.NoError(t, err)
	assert.Equal(t, messageBurst, bufferedCount, "All messages must be buffered in SQLite during Redis pause")
	assert.Equal(t, 0, redisPub.Count(), "No messages should reach Redis while paused")
	t.Logf("Step 3: Confirmed %d messages safely buffered in SQLite local store", bufferedCount)

	// 4. Resume Redis
	t.Log("Step 4: Resuming Redis connection...")
	redisPub.Resume()
	wakePublisher()

	// 5. Assert outbox replays cleanly and drains SQLite buffer to 0
	t.Log("Step 5: Verifying all buffered messages are replayed without data loss...")
	require.Eventually(t, func() bool {
		return redisPub.Count() == messageBurst
	}, 10*time.Second, 50*time.Millisecond, "All messages must be replayed to Redis")

	require.Eventually(t, func() bool {
		var remaining int
		_ = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM adapter_event_outbox`).Scan(&remaining)
		return remaining == 0
	}, 5*time.Second, 50*time.Millisecond, "SQLite outbox buffer must be completely drained")

	t.Logf("Step 6: Stress test passed! Replayed %d/%d messages cleanly with 0 data loss.",
		redisPub.Count(), messageBurst)
}

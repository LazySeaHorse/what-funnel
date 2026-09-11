// Package session owns encrypted Telegram bot credentials, polling lifecycle,
// update offsets, command idempotency, and the adapter event outbox.
package session

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/whatfunnel/whatfunnel/adapters/telegram-botapi/internal/botapi"
	"github.com/whatfunnel/whatfunnel/adapters/telegram-botapi/internal/normalize"
	wfcrypto "github.com/whatfunnel/whatfunnel/packages/go-common/crypto"
	"github.com/whatfunnel/whatfunnel/packages/go-common/messaging"
)

var (
	ErrAlreadyExists = errors.New("telegram session: bot already connected")
	ErrNotFound      = errors.New("telegram session: channel not found")
	ErrNotConnected  = errors.New("telegram session: channel not connected")
)

type EventPublisher interface {
	Publish(context.Context, messaging.Event) error
}

type Snapshot struct {
	ChannelID       string                     `json:"channel_id"`
	State           messaging.ConnectionStatus `json:"state"`
	Detail          string                     `json:"detail,omitempty"`
	RemoteAccountID string                     `json:"remote_account_id,omitempty"`
}

type botSession struct {
	channelID string
	botID     int64
	username  string
	token     string

	mu       sync.RWMutex
	snapshot Snapshot
	cancel   context.CancelFunc
	sendMu   sync.Mutex
}

func (s *botSession) copySnapshot() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.snapshot
}

type Manager struct {
	db          *sql.DB
	api         *botapi.Client
	cipher      *wfcrypto.Cipher
	publisher   EventPublisher
	mediaSource MediaSource
	logger      *slog.Logger

	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	eventWake chan struct{}

	mu       sync.RWMutex
	sessions map[string]*botSession
}

func NewManager(
	parent context.Context,
	databasePath string,
	cipher *wfcrypto.Cipher,
	api *botapi.Client,
	publisher EventPublisher,
	logger *slog.Logger,
) (*Manager, error) {
	if parent == nil || cipher == nil || api == nil || publisher == nil || strings.TrimSpace(databasePath) == "" {
		return nil, errors.New("telegram session: invalid manager configuration")
	}
	if logger == nil {
		logger = slog.Default()
	}
	if err := prepareDatabasePath(databasePath); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite3", fmt.Sprintf("file:%s?_foreign_keys=on&_busy_timeout=5000&_journal_mode=WAL", databasePath))
	if err != nil {
		return nil, fmt.Errorf("open telegram session database: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if err := createTables(parent, db); err != nil {
		_ = db.Close()
		return nil, err
	}
	ctx, cancel := context.WithCancel(parent)
	manager := &Manager{
		db: db, api: api, cipher: cipher, publisher: publisher, logger: logger,
		ctx: ctx, cancel: cancel, eventWake: make(chan struct{}, 1), sessions: make(map[string]*botSession),
	}
	manager.wg.Go(manager.publishEvents)
	if err := manager.restore(ctx); err != nil {
		_ = manager.Close()
		return nil, err
	}
	return manager, nil
}

func (m *Manager) Create(ctx context.Context, channelID, token string) (Snapshot, error) {
	channelID = strings.TrimSpace(channelID)
	token = strings.TrimSpace(token)
	if channelID == "" || token == "" {
		return Snapshot{}, errors.New("telegram session: channel id and bot token are required")
	}
	m.mu.RLock()
	_, exists := m.sessions[channelID]
	m.mu.RUnlock()
	if exists {
		return Snapshot{}, ErrAlreadyExists
	}
	validateCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	bot, err := m.api.GetMe(validateCtx, token)
	if err != nil || !bot.IsBot {
		return Snapshot{}, errors.New("telegram session: bot token validation failed")
	}
	// Switching from webhooks is required for getUpdates. Dropping queued
	// updates on first connection prevents accidental history import.
	if err := m.api.DeleteWebhook(validateCtx, token, true); err != nil {
		return Snapshot{}, errors.New("telegram session: could not enable long polling")
	}
	encrypted, err := m.cipher.Encrypt([]byte(token))
	if err != nil {
		return Snapshot{}, fmt.Errorf("encrypt telegram bot token: %w", err)
	}
	username := strings.TrimSpace(bot.Username)
	remoteID := fmt.Sprintf("%d", bot.ID)
	if username != "" {
		remoteID = "@" + username
	}
	_, err = m.db.ExecContext(ctx, `
		INSERT INTO telegram_sessions (channel_id, bot_id, username, encrypted_token, update_offset, state, remote_account_id, updated_at)
		VALUES (?, ?, ?, ?, 0, 'connecting', ?, CURRENT_TIMESTAMP)
	`, channelID, bot.ID, username, encrypted, remoteID)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return Snapshot{}, ErrAlreadyExists
		}
		return Snapshot{}, fmt.Errorf("store telegram session: %w", err)
	}
	session := m.newSession(channelID, bot.ID, username, token, messaging.ConnectionConnecting, "Connecting to Telegram.", remoteID)
	m.mu.Lock()
	m.sessions[channelID] = session
	m.mu.Unlock()
	m.startPolling(session)
	return session.copySnapshot(), nil
}

func (m *Manager) Retry(ctx context.Context, channelID, credential string) (Snapshot, error) {
	session, err := m.get(channelID)
	if err != nil {
		if errors.Is(err, ErrNotFound) && strings.TrimSpace(credential) != "" {
			return m.Create(ctx, channelID, credential)
		}
		return Snapshot{}, err
	}
	validateCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if _, err := m.api.GetMe(validateCtx, session.token); err != nil {
		m.setStatus(session, messaging.ConnectionError, "Telegram rejected this bot token.")
		return session.copySnapshot(), errors.New("telegram session: bot token validation failed")
	}
	m.restartPolling(session)
	return session.copySnapshot(), nil
}

func (m *Manager) Snapshot(channelID string) (Snapshot, error) {
	session, err := m.get(channelID)
	if err != nil {
		return Snapshot{}, err
	}
	return session.copySnapshot(), nil
}

func (m *Manager) Logout(ctx context.Context, channelID string) error {
	session, err := m.get(channelID)
	if err != nil {
		return err
	}
	if session.cancel != nil {
		session.cancel()
	}
	if _, err := m.db.ExecContext(ctx, `DELETE FROM telegram_sessions WHERE channel_id = ?`, channelID); err != nil {
		return fmt.Errorf("delete telegram session: %w", err)
	}
	m.mu.Lock()
	delete(m.sessions, channelID)
	m.mu.Unlock()
	return nil
}

func (m *Manager) Close() error {
	m.cancel()
	m.mu.RLock()
	for _, session := range m.sessions {
		if session.cancel != nil {
			session.cancel()
		}
	}
	m.mu.RUnlock()
	m.wg.Wait()
	return m.db.Close()
}

func (m *Manager) get(channelID string) (*botSession, error) {
	m.mu.RLock()
	session := m.sessions[channelID]
	m.mu.RUnlock()
	if session == nil {
		return nil, ErrNotFound
	}
	return session, nil
}

func (m *Manager) newSession(channelID string, botID int64, username, token string, state messaging.ConnectionStatus, detail, remoteID string) *botSession {
	return &botSession{channelID: channelID, botID: botID, username: username, token: token, snapshot: Snapshot{
		ChannelID: channelID, State: state, Detail: detail, RemoteAccountID: remoteID,
	}}
}

func (m *Manager) startPolling(session *botSession) {
	pollCtx, cancel := context.WithCancel(m.ctx)
	session.mu.Lock()
	session.cancel = cancel
	session.mu.Unlock()
	m.wg.Go(func() { m.poll(pollCtx, session) })
}

func (m *Manager) restartPolling(session *botSession) {
	session.mu.Lock()
	if session.cancel != nil {
		session.cancel()
	}
	session.snapshot.State = messaging.ConnectionConnecting
	session.snapshot.Detail = "Reconnecting to Telegram."
	session.mu.Unlock()
	m.startPolling(session)
}

func (m *Manager) poll(ctx context.Context, session *botSession) {
	offset, err := m.loadOffset(ctx, session.channelID)
	if err != nil {
		m.logger.Error("load telegram update offset", "channel_id", session.channelID, "error", err)
		m.setStatus(session, messaging.ConnectionError, "Could not restore Telegram polling state.")
		return
	}
	failures := 0
	for {
		updates, err := m.api.GetUpdates(ctx, session.token, offset)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			failures++
			if failures == 1 {
				m.setStatus(session, messaging.ConnectionDisconnected, "Telegram connection was interrupted. Retrying automatically.")
			}
			if !wait(ctx, retryDelay(err, failures)) {
				return
			}
			continue
		}
		failures = 0
		if session.copySnapshot().State != messaging.ConnectionConnected {
			m.setStatus(session, messaging.ConnectionConnected, "")
		}
		for _, update := range updates {
			if err := m.storeUpdate(ctx, session.channelID, update); err != nil {
				m.logger.Error("store telegram update", "channel_id", session.channelID, "update_id", update.UpdateID, "error", err)
				break
			}
			offset = update.UpdateID + 1
		}
	}
}

func retryDelay(err error, failures int) time.Duration {
	var apiErr *botapi.Error
	if errors.As(err, &apiErr) && apiErr.RetryAfter > 0 {
		return time.Duration(apiErr.RetryAfter) * time.Second
	}
	if failures > 5 {
		failures = 5
	}
	return time.Duration(1<<uint(failures-1)) * time.Second
}

func wait(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func (m *Manager) storeUpdate(ctx context.Context, channelID string, update botapi.Update) error {
	event, publish := normalize.Update(channelID, update)
	var payload []byte
	var err error
	if publish {
		if err := event.Validate(); err != nil {
			return fmt.Errorf("validate telegram event: %w", err)
		}
		payload, err = json.Marshal(event)
		if err != nil {
			return fmt.Errorf("marshal telegram event: %w", err)
		}
	}
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin telegram update: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck
	if publish {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO adapter_event_outbox (event_id, payload, available_at)
			VALUES (?, ?, ?) ON CONFLICT(event_id) DO NOTHING
		`, event.ID, payload, time.Now().UnixMilli()); err != nil {
			return fmt.Errorf("enqueue telegram event: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE telegram_sessions SET update_offset = MAX(update_offset, ?), updated_at = CURRENT_TIMESTAMP
		WHERE channel_id = ?
	`, update.UpdateID+1, channelID); err != nil {
		return fmt.Errorf("advance telegram update offset: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit telegram update: %w", err)
	}
	if publish {
		m.wakePublisher()
	}
	return nil
}

func (m *Manager) setStatus(session *botSession, state messaging.ConnectionStatus, detail string) {
	session.mu.Lock()
	if session.snapshot.State == state && session.snapshot.Detail == detail {
		session.mu.Unlock()
		return
	}
	session.snapshot.State = state
	session.snapshot.Detail = detail
	snapshot := session.snapshot
	session.mu.Unlock()
	_, err := m.db.ExecContext(m.ctx, `
		UPDATE telegram_sessions SET state = ?, detail = NULLIF(?, ''), updated_at = CURRENT_TIMESTAMP
		WHERE channel_id = ?
	`, state, detail, session.channelID)
	if err != nil {
		m.logger.Error("persist telegram status", "channel_id", session.channelID, "error", err)
		return
	}
	event := messaging.Event{
		SchemaVersion: messaging.SchemaVersion,
		ID:            fmt.Sprintf("telegram:%s:status:%s:%d", session.channelID, state, time.Now().UnixNano()),
		Kind:          messaging.EventChannelStatus,
		Provider:      messaging.ProviderTelegram,
		ChannelID:     session.channelID,
		OccurredAt:    time.Now().UTC(),
		Status:        &messaging.Status{State: state, Detail: detail},
	}
	if err := m.enqueueEvent(m.ctx, event); err != nil {
		m.logger.Error("enqueue telegram status", "channel_id", snapshot.ChannelID, "error", err)
	}
}

func (m *Manager) restore(ctx context.Context) error {
	rows, err := m.db.QueryContext(ctx, `
		SELECT channel_id, bot_id, username, encrypted_token, state,
		       COALESCE(detail, ''), remote_account_id
		FROM telegram_sessions ORDER BY channel_id
	`)
	if err != nil {
		return fmt.Errorf("query telegram sessions: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var channelID, username, encrypted, detail, remoteID string
		var botID int64
		var state messaging.ConnectionStatus
		if err := rows.Scan(&channelID, &botID, &username, &encrypted, &state, &detail, &remoteID); err != nil {
			return fmt.Errorf("scan telegram session: %w", err)
		}
		plaintext, err := m.cipher.Decrypt(encrypted)
		if err != nil {
			return fmt.Errorf("decrypt telegram session %s: %w", channelID, err)
		}
		session := m.newSession(channelID, botID, username, string(plaintext), state, detail, remoteID)
		m.sessions[channelID] = session
		m.startPolling(session)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate telegram sessions: %w", err)
	}
	return nil
}

func (m *Manager) loadOffset(ctx context.Context, channelID string) (int64, error) {
	var offset int64
	err := m.db.QueryRowContext(ctx, `SELECT update_offset FROM telegram_sessions WHERE channel_id = ?`, channelID).Scan(&offset)
	return offset, err
}

func prepareDatabasePath(path string) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create telegram data directory: %w", err)
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		return fmt.Errorf("secure telegram data directory: %w", err)
	}
	return nil
}

func createTables(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS telegram_sessions (
			channel_id TEXT PRIMARY KEY,
			bot_id INTEGER NOT NULL UNIQUE,
			username TEXT NOT NULL DEFAULT '',
			encrypted_token TEXT NOT NULL,
			update_offset INTEGER NOT NULL DEFAULT 0,
			state TEXT NOT NULL,
			detail TEXT,
			remote_account_id TEXT NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
		CREATE TABLE IF NOT EXISTS adapter_commands (
			command_id TEXT PRIMARY KEY,
			provider_message_id TEXT NOT NULL,
			completed_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
		CREATE TABLE IF NOT EXISTS adapter_event_outbox (
			event_id TEXT PRIMARY KEY,
			payload BLOB NOT NULL,
			available_at INTEGER NOT NULL,
			attempts INTEGER NOT NULL DEFAULT 0
		);
	`)
	if err != nil {
		return fmt.Errorf("create telegram adapter tables: %w", err)
	}
	return nil
}

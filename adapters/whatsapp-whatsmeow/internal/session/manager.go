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
	"github.com/whatfunnel/whatfunnel/adapters/whatsapp-whatsmeow/internal/normalize"
	"github.com/whatfunnel/whatfunnel/packages/go-common/messaging"
	"go.mau.fi/whatsmeow"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

var (
	ErrAlreadyExists = errors.New("whatsapp session: channel already exists")
	ErrNotFound      = errors.New("whatsapp session: channel not found")
	ErrNotConnected  = errors.New("whatsapp session: channel not connected")
)

type EventPublisher interface {
	Publish(context.Context, messaging.Event) error
}

type Snapshot struct {
	ChannelID       string                     `json:"channel_id"`
	State           messaging.ConnectionStatus `json:"state"`
	Detail          string                     `json:"detail,omitempty"`
	QRData          string                     `json:"qr_data,omitempty"`
	RemoteAccountID string                     `json:"remote_account_id,omitempty"`
}

type clientSession struct {
	client *whatsmeow.Client
	sendMu sync.Mutex

	mu       sync.RWMutex
	snapshot Snapshot
}

type Manager struct {
	db          *sql.DB
	container   *sqlstore.Container
	publisher   EventPublisher
	mediaSource MediaSource
	logger      *slog.Logger

	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	eventWake chan struct{}

	mu       sync.RWMutex
	sessions map[string]*clientSession
}

func NewManager(
	parent context.Context,
	databasePath string,
	publisher EventPublisher,
	mediaSource MediaSource,
	logger *slog.Logger,
) (*Manager, error) {
	if parent == nil || publisher == nil || strings.TrimSpace(databasePath) == "" {
		return nil, errors.New("whatsapp session: invalid manager configuration")
	}
	if logger == nil {
		logger = slog.Default()
	}
	if err := prepareDatabasePath(databasePath); err != nil {
		return nil, err
	}

	dsn := fmt.Sprintf("file:%s?_foreign_keys=on&_busy_timeout=5000&_journal_mode=WAL", databasePath)
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("open session database: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	container := sqlstore.NewWithDB(db, "sqlite3", nil)
	if err := container.Upgrade(parent); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("upgrade whatsmeow store: %w", err)
	}
	if err := createAdapterTables(parent, db); err != nil {
		_ = db.Close()
		return nil, err
	}

	ctx, cancel := context.WithCancel(parent)
	manager := &Manager{
		db:          db,
		container:   container,
		publisher:   publisher,
		mediaSource: mediaSource,
		logger:      logger,
		ctx:         ctx,
		cancel:      cancel,
		eventWake:   make(chan struct{}, 1),
		sessions:    make(map[string]*clientSession),
	}
	manager.wg.Go(manager.publishEvents)
	if err := manager.restore(ctx); err != nil {
		_ = manager.Close()
		return nil, err
	}
	return manager, nil
}

func (m *Manager) Create(ctx context.Context, channelID string) (Snapshot, error) {
	channelID = strings.TrimSpace(channelID)
	if channelID == "" {
		return Snapshot{}, errors.New("whatsapp session: empty channel id")
	}

	m.mu.Lock()
	if _, exists := m.sessions[channelID]; exists {
		m.mu.Unlock()
		return Snapshot{}, ErrAlreadyExists
	}
	session := m.newSession(channelID, m.container.NewDevice())
	m.sessions[channelID] = session
	m.mu.Unlock()

	qrChannel, err := session.client.GetQRChannel(m.ctx)
	if err != nil {
		m.removeSession(channelID)
		return Snapshot{}, fmt.Errorf("prepare qr pairing: %w", err)
	}
	m.wg.Go(func() {
		m.consumeQR(session, qrChannel)
	})

	connectCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := session.client.ConnectContext(connectCtx); err != nil {
		m.setStatus(session, messaging.ConnectionError, "Could not connect to WhatsApp.", "")
		return session.copySnapshot(), fmt.Errorf("connect whatsapp: %w", err)
	}
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
	if session.client.IsLoggedIn() {
		logoutCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		if err := session.client.Logout(logoutCtx); err != nil {
			return fmt.Errorf("logout whatsapp session: %w", err)
		}
	} else {
		session.client.Disconnect()
		if err := session.client.Store.Delete(ctx); err != nil {
			return fmt.Errorf("delete whatsapp session: %w", err)
		}
	}
	if _, err := m.db.ExecContext(ctx, `DELETE FROM adapter_channels WHERE channel_id = ?`, channelID); err != nil {
		return fmt.Errorf("delete channel mapping: %w", err)
	}
	m.removeSession(channelID)
	return nil
}

func (m *Manager) Close() error {
	m.cancel()
	m.mu.RLock()
	sessions := make([]*clientSession, 0, len(m.sessions))
	for _, session := range m.sessions {
		sessions = append(sessions, session)
	}
	m.mu.RUnlock()
	for _, session := range sessions {
		session.client.Disconnect()
	}
	m.wg.Wait()
	return m.container.Close()
}

func (m *Manager) newSession(channelID string, device *store.Device) *clientSession {
	client := whatsmeow.NewClient(device, nil)
	session := &clientSession{
		client: client,
		snapshot: Snapshot{
			ChannelID: channelID,
			State:     messaging.ConnectionPending,
		},
	}
	client.AddEventHandler(func(event any) {
		m.handleEvent(channelID, session, event)
	})
	return session
}

func (m *Manager) handleEvent(channelID string, session *clientSession, event any) {
	switch event := event.(type) {
	case *events.Message:
		result, ok := normalize.Message(channelID, event)
		if !ok {
			return
		}
		if result.Downloadable != nil {
			providerRef := result.Event.Message.Media.ProviderRef
			if err := m.storeMedia(m.ctx, channelID, providerRef, result.Downloadable); err != nil {
				m.logger.Error("store whatsapp media reference", "channel_id", channelID, "error", err)
				result.Event.Message.ContentType = messaging.ContentNotice
				result.Event.Message.Text = "Open WhatsApp to view this media."
				result.Event.Message.NoticeCode = "media_unavailable"
				result.Event.Message.Media = nil
			}
		}
		m.enqueue(result.Event)
	case *events.Receipt:
		for _, normalized := range normalize.Receipt(channelID, event) {
			m.enqueue(normalized)
		}
	case *events.Connected:
		remoteID := ""
		if session.client.Store.ID != nil {
			remoteID = session.client.Store.ID.ToNonAD().String()
			if err := m.saveMapping(m.ctx, channelID, session.client.Store.ID.String()); err != nil {
				m.logger.Error("save whatsapp channel mapping", "channel_id", channelID, "error", err)
			}
		}
		m.setStatus(session, messaging.ConnectionConnected, "", remoteID)
	case *events.Disconnected:
		m.setStatus(session, messaging.ConnectionDisconnected, "WhatsApp connection was interrupted.", "")
	case *events.LoggedOut:
		m.setStatus(session, messaging.ConnectionError, "WhatsApp unlinked this device. Pair it again.", "")
	}
}

func (m *Manager) consumeQR(session *clientSession, qrChannel <-chan whatsmeow.QRChannelItem) {
	for {
		select {
		case <-m.ctx.Done():
			return
		case item, ok := <-qrChannel:
			if !ok {
				return
			}
			switch item.Event {
			case "code":
				m.setStatus(session, messaging.ConnectionAwaitingScan, "Scan this code in WhatsApp Linked Devices.", item.Code)
			case "success":
				m.setStatus(session, messaging.ConnectionConnecting, "Pairing complete. Connecting…", "")
			case "timeout":
				m.setStatus(session, messaging.ConnectionError, "The QR code expired. Start pairing again.", "")
			default:
				m.setStatus(session, messaging.ConnectionError, "WhatsApp pairing failed.", "")
			}
		}
	}
}

func (m *Manager) restore(ctx context.Context) error {
	rows, err := m.db.QueryContext(ctx, `SELECT channel_id, device_jid FROM adapter_channels ORDER BY channel_id`)
	if err != nil {
		return fmt.Errorf("query whatsapp channel mappings: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var channelID, deviceJID string
		if err := rows.Scan(&channelID, &deviceJID); err != nil {
			return fmt.Errorf("scan whatsapp channel mapping: %w", err)
		}
		jid, err := types.ParseJID(deviceJID)
		if err != nil {
			return fmt.Errorf("parse stored whatsapp jid: %w", err)
		}
		device, err := m.container.GetDevice(ctx, jid)
		if err != nil {
			return fmt.Errorf("load whatsapp device: %w", err)
		}
		if device == nil {
			continue
		}
		session := m.newSession(channelID, device)
		m.sessions[channelID] = session
		connectCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		err = session.client.ConnectContext(connectCtx)
		cancel()
		if err != nil {
			m.setStatus(session, messaging.ConnectionError, "Could not restore the WhatsApp connection.", "")
			m.logger.Warn("restore whatsapp connection", "channel_id", channelID, "error", err)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate whatsapp channel mappings: %w", err)
	}
	return nil
}

func (m *Manager) publishEvents() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-m.eventWake:
		case <-ticker.C:
		}
		for m.publishNextEvent() {
		}
	}
}

func (m *Manager) enqueue(event messaging.Event) {
	if err := event.Validate(); err != nil {
		m.logger.Error("reject invalid whatsapp event", "event_id", event.ID, "error", err)
		return
	}
	payload, err := json.Marshal(event)
	if err != nil {
		m.logger.Error("marshal whatsapp event", "event_id", event.ID, "error", err)
		return
	}
	if _, err := m.db.ExecContext(m.ctx, `
		INSERT INTO adapter_event_outbox (event_id, payload, available_at)
		VALUES (?, ?, ?)
		ON CONFLICT(event_id) DO NOTHING
	`, event.ID, payload, time.Now().UnixMilli()); err != nil {
		m.logger.Error("persist whatsapp event", "event_id", event.ID, "error", err)
		return
	}
	select {
	case <-m.ctx.Done():
	case m.eventWake <- struct{}{}:
	default:
	}
}

func (m *Manager) publishNextEvent() bool {
	var (
		eventID  string
		payload  []byte
		attempts int
	)
	err := m.db.QueryRowContext(m.ctx, `
		SELECT event_id, payload, attempts
		FROM adapter_event_outbox
		WHERE available_at <= ?
		ORDER BY created_at, event_id
		LIMIT 1
	`, time.Now().UnixMilli()).Scan(&eventID, &payload, &attempts)
	if errors.Is(err, sql.ErrNoRows) || m.ctx.Err() != nil {
		return false
	}
	if err != nil {
		m.logger.Error("load whatsapp event outbox", "error", err)
		return false
	}
	var event messaging.Event
	if err := json.Unmarshal(payload, &event); err != nil {
		m.logger.Error("discard corrupt whatsapp event outbox row", "event_id", eventID, "error", err)
		_, _ = m.db.ExecContext(m.ctx, `DELETE FROM adapter_event_outbox WHERE event_id = ?`, eventID)
		return true
	}
	publishCtx, cancel := context.WithTimeout(m.ctx, 10*time.Second)
	err = m.publisher.Publish(publishCtx, event)
	cancel()
	if err != nil {
		backoff := time.Second << min(attempts, 8)
		if backoff > 5*time.Minute {
			backoff = 5 * time.Minute
		}
		_, _ = m.db.ExecContext(m.ctx, `
			UPDATE adapter_event_outbox
			SET attempts = attempts + 1, available_at = ?
			WHERE event_id = ?
		`, time.Now().Add(backoff).UnixMilli(), eventID)
		if m.ctx.Err() == nil {
			m.logger.Error("publish whatsapp event", "event_id", eventID, "error", err)
		}
		return false
	}
	if _, err := m.db.ExecContext(m.ctx, `DELETE FROM adapter_event_outbox WHERE event_id = ?`, eventID); err != nil {
		m.logger.Error("complete whatsapp event", "event_id", eventID, "error", err)
		return false
	}
	return true
}

func (m *Manager) get(channelID string) (*clientSession, error) {
	m.mu.RLock()
	session := m.sessions[channelID]
	m.mu.RUnlock()
	if session == nil {
		return nil, ErrNotFound
	}
	return session, nil
}

func (m *Manager) removeSession(channelID string) {
	m.mu.Lock()
	session := m.sessions[channelID]
	delete(m.sessions, channelID)
	m.mu.Unlock()
	if session != nil {
		session.client.Disconnect()
	}
}

func (m *Manager) setStatus(session *clientSession, state messaging.ConnectionStatus, detail, qrData string) {
	session.mu.Lock()
	session.snapshot.State = state
	session.snapshot.Detail = detail
	session.snapshot.QRData = qrData
	snapshot := session.snapshot
	session.mu.Unlock()

	now := time.Now().UTC()
	event := messaging.Event{
		SchemaVersion: messaging.SchemaVersion,
		ID:            fmt.Sprintf("whatsapp:%s:status:%d", snapshot.ChannelID, now.UnixNano()),
		Kind:          messaging.EventChannelStatus,
		Provider:      messaging.ProviderWhatsApp,
		ChannelID:     snapshot.ChannelID,
		OccurredAt:    now,
		Status: &messaging.Status{
			State:  state,
			Detail: detail,
		},
	}
	m.enqueue(event)
}

func (s *clientSession) copySnapshot() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.snapshot
}

func (m *Manager) saveMapping(ctx context.Context, channelID, deviceJID string) error {
	_, err := m.db.ExecContext(ctx, `
		INSERT INTO adapter_channels (channel_id, device_jid, updated_at)
		VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(channel_id) DO UPDATE
		SET device_jid = excluded.device_jid, updated_at = CURRENT_TIMESTAMP
	`, channelID, deviceJID)
	if err != nil {
		return fmt.Errorf("upsert whatsapp channel mapping: %w", err)
	}
	return nil
}

func (m *Manager) storeMedia(
	ctx context.Context,
	channelID string,
	providerRef string,
	downloadable whatsmeow.DownloadableMessage,
) error {
	kind, message, err := marshalDownloadable(downloadable)
	if err != nil {
		return err
	}
	_, err = m.db.ExecContext(ctx, `
		INSERT INTO adapter_media (channel_id, provider_ref, media_kind, media_message, created_at)
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(channel_id, provider_ref) DO UPDATE
		SET media_kind = excluded.media_kind, media_message = excluded.media_message
	`, channelID, providerRef, kind, message)
	if err != nil {
		return fmt.Errorf("store whatsapp media: %w", err)
	}
	return nil
}

func marshalDownloadable(downloadable whatsmeow.DownloadableMessage) (string, []byte, error) {
	var kind string
	switch downloadable.(type) {
	case *waE2E.ImageMessage:
		kind = "image"
	case *waE2E.VideoMessage:
		kind = "video"
	case *waE2E.AudioMessage:
		kind = "audio"
	case *waE2E.DocumentMessage:
		kind = "document"
	default:
		return "", nil, fmt.Errorf("unsupported downloadable type %T", downloadable)
	}
	message, ok := downloadable.(proto.Message)
	if !ok {
		return "", nil, fmt.Errorf("downloadable type %T is not protobuf", downloadable)
	}
	data, err := proto.Marshal(message)
	if err != nil {
		return "", nil, fmt.Errorf("marshal whatsapp media: %w", err)
	}
	return kind, data, nil
}

func prepareDatabasePath(databasePath string) error {
	directory := filepath.Dir(databasePath)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create whatsapp database directory: %w", err)
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		return fmt.Errorf("secure whatsapp database directory: %w", err)
	}
	return nil
}

func createAdapterTables(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS adapter_channels (
			channel_id TEXT PRIMARY KEY,
			device_jid TEXT NOT NULL UNIQUE,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
		CREATE TABLE IF NOT EXISTS adapter_media (
			channel_id TEXT NOT NULL,
			provider_ref TEXT NOT NULL,
			media_kind TEXT NOT NULL,
			media_message BLOB NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (channel_id, provider_ref)
		);
		CREATE TABLE IF NOT EXISTS adapter_commands (
			command_id TEXT PRIMARY KEY,
			provider_message_id TEXT NOT NULL,
			completed_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
		CREATE TABLE IF NOT EXISTS adapter_event_outbox (
			event_id TEXT PRIMARY KEY,
			payload BLOB NOT NULL,
			attempts INTEGER NOT NULL DEFAULT 0,
			available_at INTEGER NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
	`)
	if err != nil {
		return fmt.Errorf("create whatsapp adapter tables: %w", err)
	}
	return nil
}

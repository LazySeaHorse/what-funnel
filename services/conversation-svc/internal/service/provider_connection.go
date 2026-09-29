package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/whatfunnel/whatfunnel/packages/go-common/audit"
	"github.com/whatfunnel/whatfunnel/packages/go-common/messaging"
	"github.com/whatfunnel/whatfunnel/packages/go-common/types"
)

var (
	ErrAdapterConnectionNotFound     = errors.New("adapter connection not found")
	ErrProviderConnectionLabelExists = errors.New("provider connection label already exists")
)

type AdapterSnapshot struct {
	ChannelID       string                     `json:"channel_id"`
	State           messaging.ConnectionStatus `json:"state"`
	Detail          string                     `json:"detail,omitempty"`
	QRData          string                     `json:"qr_data,omitempty"`
	RemoteAccountID string                     `json:"remote_account_id,omitempty"`
}

type AdapterControl interface {
	Create(context.Context, string, string) (AdapterSnapshot, error)
	Retry(context.Context, string, string) (AdapterSnapshot, error)
	Snapshot(context.Context, string) (AdapterSnapshot, error)
	Logout(context.Context, string) error
	List(context.Context) ([]AdapterSnapshot, error)
}

// OrphanEvictionPolicy controls when adapter sessions that have no channel in
// the database are logged out. Logging out a WhatsApp session unpairs the
// phone irreversibly, so the default is off: a restored, empty or lagging
// database must not be able to wipe live pairings.
type OrphanEvictionPolicy struct {
	// Enabled turns eviction on. When false nothing is ever logged out.
	Enabled bool
	// Grace is how long an adapter session must have been observed without a
	// matching channel before it is evicted.
	Grace time.Duration
}

type ConnectionService struct {
	pool       *pgxpool.Pool
	controls   map[messaging.Provider]AdapterControl
	controlsMu sync.RWMutex

	evictionMu     sync.Mutex
	evictionPolicy OrphanEvictionPolicy
	orphanSeen     map[string]time.Time
	now            func() time.Time
	// hasChannelData reports whether the database holds any channel for the
	// provider; an empty table means the database, not the adapter, is suspect.
	hasChannelData func(context.Context, messaging.Provider) (bool, error)
}

func NewConnectionService(pool *pgxpool.Pool) *ConnectionService {
	s := &ConnectionService{
		pool:       pool,
		controls:   make(map[messaging.Provider]AdapterControl),
		orphanSeen: make(map[string]time.Time),
		now:        time.Now,
	}
	s.hasChannelData = s.providerHasChannels
	return s
}

// ConfigureOrphanEviction sets the orphaned-session eviction policy.
func (s *ConnectionService) ConfigureOrphanEviction(policy OrphanEvictionPolicy) {
	s.evictionMu.Lock()
	defer s.evictionMu.Unlock()
	s.evictionPolicy = policy
}

func (s *ConnectionService) orphanPolicy() OrphanEvictionPolicy {
	s.evictionMu.Lock()
	defer s.evictionMu.Unlock()
	return s.evictionPolicy
}

func (s *ConnectionService) providerHasChannels(ctx context.Context, provider messaging.Provider) (bool, error) {
	var exists bool
	if err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM channels WHERE provider = $1)`, provider).Scan(&exists); err != nil {
		return false, fmt.Errorf("check provider channel data: %w", err)
	}
	return exists, nil
}

func (s *ConnectionService) RegisterAdapterControl(provider messaging.Provider, control AdapterControl) {
	s.controlsMu.Lock()
	defer s.controlsMu.Unlock()
	s.controls[provider] = control
}

func (s *ConnectionService) StartProviderConnection(
	ctx context.Context,
	accountID uuid.UUID,
	provider messaging.Provider,
	label, credential string,
) (*types.ProviderConnection, error) {
	label = strings.TrimSpace(label)
	if !provider.Valid() {
		return nil, invalidf("provider is not available")
	}
	if provider == messaging.ProviderTelegram && strings.TrimSpace(credential) == "" {
		return nil, invalidf("telegram bot token is required")
	}
	if label == "" || len(label) > 80 {
		return nil, invalidf("label must contain 1 to 80 characters")
	}
	control, err := s.adapterControl(provider)
	if err != nil {
		return nil, err
	}

	capabilities := providerCapabilities(provider)
	capabilitiesJSON, err := json.Marshal(capabilities)
	if err != nil {
		return nil, fmt.Errorf("marshal provider capabilities: %w", err)
	}
	connection := &types.ProviderConnection{
		AccountID:    accountID,
		Provider:     provider,
		Label:        label,
		State:        messaging.ConnectionPending,
		Capabilities: capabilities,
	}
	err = s.pool.QueryRow(ctx, `
		WITH channel AS (
			INSERT INTO channels (
				account_id, type, provider, label, status, capabilities, updated_at
			) VALUES ($1, $2, $2, $3, 'pending', $4, NOW())
			RETURNING id, created_at, updated_at
		), connection AS (
			INSERT INTO provider_connections (channel_id, account_id, provider, state)
			SELECT id, $1, $2, 'pending' FROM channel
		)
		SELECT id, created_at, updated_at FROM channel
	`, accountID, provider, label, capabilitiesJSON).Scan(
		&connection.ChannelID,
		&connection.CreatedAt,
		&connection.UpdatedAt,
	)
	if err != nil {
		if isProviderConnectionLabelConflict(err) {
			var existingID uuid.UUID
			var existingStatus messaging.ConnectionStatus
			queryErr := s.pool.QueryRow(ctx, `
				SELECT id, status FROM channels
				WHERE account_id = $1 AND provider = $2 AND LOWER(label) = LOWER($3)
			`, accountID, provider, label).Scan(&existingID, &existingStatus)

			if queryErr == nil && (existingStatus == messaging.ConnectionError || existingStatus == messaging.ConnectionPending) {
				return s.RetryProviderConnection(ctx, accountID, existingID, credential)
			}
			return nil, ErrProviderConnectionLabelExists
		}
		return nil, fmt.Errorf("create provider connection: %w", err)
	}

	snapshot, err := control.Create(ctx, connection.ChannelID.String(), credential)
	if err != nil {
		detail := "Could not connect this provider account. Check the credentials and try again."
		_ = s.updateProviderConnection(ctx, connection.ChannelID, messaging.ConnectionError, detail, "")
		connection.State = messaging.ConnectionError
		connection.Detail = detail
		return connection, fmt.Errorf("start provider adapter: %w", err)
	}
	applyAdapterSnapshot(connection, snapshot)
	if err := s.updateProviderConnection(ctx, connection.ChannelID, snapshot.State, snapshot.Detail, snapshot.RemoteAccountID); err != nil {
		return nil, err
	}
	return connection, nil
}

func (s *ConnectionService) RetryProviderConnection(
	ctx context.Context,
	accountID, channelID uuid.UUID,
	credential string,
) (*types.ProviderConnection, error) {
	connection, err := s.loadProviderConnection(ctx, accountID, channelID)
	if err != nil {
		return nil, err
	}
	control, err := s.adapterControl(connection.Provider)
	if err != nil {
		return nil, err
	}
	snapshot, err := control.Retry(ctx, channelID.String(), strings.TrimSpace(credential))
	if err != nil {
		detail := "Could not reconnect this provider account."
		_ = s.updateProviderConnection(ctx, channelID, messaging.ConnectionError, detail, connection.RemoteAccountID)
		connection.State = messaging.ConnectionError
		connection.Detail = detail
		return connection, fmt.Errorf("retry provider adapter: %w", err)
	}
	applyAdapterSnapshot(connection, snapshot)
	if err := s.updateProviderConnection(ctx, channelID, snapshot.State, snapshot.Detail, snapshot.RemoteAccountID); err != nil {
		return nil, err
	}
	return connection, nil
}

func (s *ConnectionService) GetProviderConnection(
	ctx context.Context,
	accountID, channelID uuid.UUID,
	refresh bool,
) (*types.ProviderConnection, error) {
	connection, err := s.loadProviderConnection(ctx, accountID, channelID)
	if err != nil {
		return nil, err
	}
	if !refresh {
		return connection, nil
	}
	control, err := s.adapterControl(connection.Provider)
	if err != nil {
		return connection, nil
	}
	snapshot, err := control.Snapshot(ctx, channelID.String())
	if err != nil {
		return connection, nil
	}
	applyAdapterSnapshot(connection, snapshot)
	if err := s.updateProviderConnection(ctx, channelID, snapshot.State, snapshot.Detail, snapshot.RemoteAccountID); err != nil {
		return nil, err
	}
	return connection, nil
}

func (s *ConnectionService) ListProviderConnections(ctx context.Context, accountID uuid.UUID) ([]*types.ProviderConnection, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT pc.channel_id, pc.account_id, pc.provider, ch.label, pc.state,
		       COALESCE(pc.detail, ''), COALESCE(pc.remote_account_id, ''),
		       ch.capabilities, pc.created_at, pc.updated_at
		FROM provider_connections pc
		JOIN channels ch ON ch.id = pc.channel_id
		WHERE pc.account_id = $1
		ORDER BY pc.created_at ASC
	`, accountID)
	if err != nil {
		return nil, fmt.Errorf("list provider connections: %w", err)
	}
	defer rows.Close()

	connections := []*types.ProviderConnection{}
	for rows.Next() {
		connection, err := scanProviderConnection(rows)
		if err != nil {
			return nil, err
		}
		connections = append(connections, connection)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate provider connections: %w", err)
	}
	return connections, nil
}

func (s *ConnectionService) DeleteProviderConnection(
	ctx context.Context,
	accountID, actorID, channelID uuid.UUID,
) error {
	connection, err := s.loadProviderConnection(ctx, accountID, channelID)
	if err != nil {
		return err
	}
	control, err := s.adapterControl(connection.Provider)
	if err != nil {
		return err
	}
	if err := control.Logout(ctx, channelID.String()); err != nil && !errors.Is(err, ErrAdapterConnectionNotFound) {
		return fmt.Errorf("unlink provider account: %w", err)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin provider connection deletion: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	result, err := tx.Exec(ctx, `DELETE FROM channels WHERE id = $1 AND account_id = $2`, channelID, accountID)
	if err != nil {
		return fmt.Errorf("delete provider connection: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrChannelNotFound
	}
	writer := audit.NewWriterFromTx(tx)
	if err := writer.Write(ctx, audit.Entry{
		AccountID:   accountID,
		ActorUserID: &actorID,
		Action:      "channel.deleted",
		TargetType:  "channel",
		TargetID:    &channelID,
		Metadata: map[string]any{
			"provider": connection.Provider,
			"label":    connection.Label,
		},
	}); err != nil {
		return fmt.Errorf("write provider deletion audit: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit provider connection deletion: %w", err)
	}
	return nil
}

// EvictOrphanedChannel logs an adapter session out when it has no channel in
// the database. It reports whether a logout actually happened. Nothing is
// evicted unless eviction is enabled, the database holds channel data for the
// provider, and the session has stayed orphaned for the policy's grace period.
func (s *ConnectionService) EvictOrphanedChannel(ctx context.Context, provider messaging.Provider, channelID string) (bool, error) {
	control, err := s.adapterControl(provider)
	if err != nil {
		return false, err
	}
	return s.evictIfOrphaned(ctx, provider, control, channelID)
}

func (s *ConnectionService) evictIfOrphaned(ctx context.Context, provider messaging.Provider, control AdapterControl, channelID string) (bool, error) {
	policy := s.orphanPolicy()
	if !policy.Enabled {
		return false, nil
	}
	key := string(provider) + "/" + channelID

	if channelUUID, err := uuid.Parse(channelID); err == nil {
		var exists bool
		if err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM channels WHERE id = $1 AND provider = $2)`, channelUUID, provider).Scan(&exists); err != nil {
			return false, fmt.Errorf("query channel existence: %w", err)
		}
		if exists {
			s.forgetOrphan(key)
			return false, nil
		}
	}
	hasData, err := s.hasChannelData(ctx, provider)
	if err != nil {
		return false, err
	}
	if !hasData {
		// No channels at all for this provider: far more likely a restored or
		// empty database than every session being an orphan.
		return false, nil
	}
	firstSeen := s.markOrphanSeen(key)
	if s.now().Sub(firstSeen) < policy.Grace {
		return false, nil
	}
	if err := control.Logout(ctx, channelID); err != nil {
		if errors.Is(err, ErrAdapterConnectionNotFound) {
			s.forgetOrphan(key)
			return false, nil
		}
		return false, fmt.Errorf("evict orphaned adapter channel %s: %w", channelID, err)
	}
	s.forgetOrphan(key)
	return true, nil
}

// markOrphanSeen records the first time a session was seen without a channel
// and returns that time.
func (s *ConnectionService) markOrphanSeen(key string) time.Time {
	s.evictionMu.Lock()
	defer s.evictionMu.Unlock()
	first, ok := s.orphanSeen[key]
	if !ok {
		first = s.now()
		s.orphanSeen[key] = first
	}
	return first
}

func (s *ConnectionService) forgetOrphan(key string) {
	s.evictionMu.Lock()
	defer s.evictionMu.Unlock()
	delete(s.orphanSeen, key)
}

// ReconcileAdapterConnections evicts adapter sessions that no longer have a
// channel, subject to the orphan eviction policy. It returns how many sessions
// were actually logged out; failed logouts are reported in the error and not
// counted.
func (s *ConnectionService) ReconcileAdapterConnections(ctx context.Context, provider messaging.Provider) (int, error) {
	control, err := s.adapterControl(provider)
	if err != nil {
		return 0, err
	}
	if !s.orphanPolicy().Enabled {
		return 0, nil
	}
	connections, err := control.List(ctx)
	if err != nil {
		return 0, fmt.Errorf("list adapter connections: %w", err)
	}
	evictedCount := 0
	var errs []error
	for _, conn := range connections {
		evicted, err := s.evictIfOrphaned(ctx, provider, control, conn.ChannelID)
		if err != nil {
			if ctx.Err() != nil {
				return evictedCount, err
			}
			errs = append(errs, err)
			continue
		}
		if evicted {
			evictedCount++
		}
	}
	return evictedCount, errors.Join(errs...)
}

func isProviderConnectionLabelConflict(err error) bool {
	var databaseError *pgconn.PgError
	return errors.As(err, &databaseError) &&
		databaseError.Code == "23505" &&
		databaseError.ConstraintName == "idx_channels_account_provider_label"
}

func (s *ConnectionService) adapterControl(provider messaging.Provider) (AdapterControl, error) {
	s.controlsMu.RLock()
	control := s.controls[provider]
	s.controlsMu.RUnlock()
	if control == nil {
		return nil, fmt.Errorf("provider %q is not configured", provider)
	}
	return control, nil
}

func (s *ConnectionService) loadProviderConnection(
	ctx context.Context,
	accountID, channelID uuid.UUID,
) (*types.ProviderConnection, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT pc.channel_id, pc.account_id, pc.provider, ch.label, pc.state,
		       COALESCE(pc.detail, ''), COALESCE(pc.remote_account_id, ''),
		       ch.capabilities, pc.created_at, pc.updated_at
		FROM provider_connections pc
		JOIN channels ch ON ch.id = pc.channel_id
		WHERE pc.channel_id = $1 AND pc.account_id = $2
	`, channelID, accountID)
	connection, err := scanProviderConnection(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrChannelNotFound
	}
	return connection, err
}

type providerConnectionScanner interface {
	Scan(...any) error
}

func scanProviderConnection(row providerConnectionScanner) (*types.ProviderConnection, error) {
	connection := &types.ProviderConnection{}
	var capabilitiesJSON []byte
	err := row.Scan(
		&connection.ChannelID,
		&connection.AccountID,
		&connection.Provider,
		&connection.Label,
		&connection.State,
		&connection.Detail,
		&connection.RemoteAccountID,
		&capabilitiesJSON,
		&connection.CreatedAt,
		&connection.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(capabilitiesJSON, &connection.Capabilities); err != nil {
		return nil, fmt.Errorf("decode provider capabilities: %w", err)
	}
	return connection, nil
}

func (s *ConnectionService) updateProviderConnection(
	ctx context.Context,
	channelID uuid.UUID,
	state messaging.ConnectionStatus,
	detail, remoteAccountID string,
) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin provider status update: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if _, err := tx.Exec(ctx, `
		UPDATE provider_connections
		SET state = $1, detail = NULLIF($2, ''), remote_account_id = NULLIF($3, ''), updated_at = NOW()
		WHERE channel_id = $4
	`, state, detail, remoteAccountID, channelID); err != nil {
		return fmt.Errorf("update provider connection: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE channels
		SET status = $1, status_detail = NULLIF($2, ''), remote_account_id = NULLIF($3, ''), updated_at = NOW()
		WHERE id = $4
	`, state, detail, remoteAccountID, channelID); err != nil {
		return fmt.Errorf("update channel status: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit provider status update: %w", err)
	}
	return nil
}

func providerCapabilities(provider messaging.Provider) messaging.Capabilities {
	switch provider {
	case messaging.ProviderWhatsApp:
		return messaging.Capabilities{
			Media:     true,
			Replies:   true,
			Reactions: true,
			Edits:     true,
			Deletes:   true,
			Receipts:  true,
		}
	case messaging.ProviderTelegram:
		return messaging.Capabilities{
			Media: true, Replies: true, Reactions: true,
			Edits: true, Deletes: true, Receipts: false,
		}
	}
	return messaging.Capabilities{}
}

func applyAdapterSnapshot(connection *types.ProviderConnection, snapshot AdapterSnapshot) {
	connection.State = snapshot.State
	connection.Detail = snapshot.Detail
	connection.QRData = snapshot.QRData
	connection.RemoteAccountID = snapshot.RemoteAccountID
}

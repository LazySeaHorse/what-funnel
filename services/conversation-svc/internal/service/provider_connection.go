package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/whatfunnel/whatfunnel/packages/go-common/audit"
	"github.com/whatfunnel/whatfunnel/packages/go-common/messaging"
	"github.com/whatfunnel/whatfunnel/packages/go-common/types"
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
}

func (s *Service) RegisterAdapterControl(provider messaging.Provider, control AdapterControl) {
	s.controlsMu.Lock()
	defer s.controlsMu.Unlock()
	s.controls[provider] = control
}

func (s *Service) StartProviderConnection(
	ctx context.Context,
	accountID uuid.UUID,
	provider messaging.Provider,
	label, credential string,
) (*types.ProviderConnection, error) {
	label = strings.TrimSpace(label)
	if !provider.Valid() {
		return nil, errors.New("provider is not available")
	}
	if provider == messaging.ProviderTelegram && strings.TrimSpace(credential) == "" {
		return nil, errors.New("telegram bot token is required")
	}
	if label == "" || len(label) > 80 {
		return nil, errors.New("label must contain 1 to 80 characters")
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

func (s *Service) RetryProviderConnection(
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

func (s *Service) GetProviderConnection(
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

func (s *Service) ListProviderConnections(ctx context.Context, accountID uuid.UUID) ([]*types.ProviderConnection, error) {
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

func (s *Service) DeleteProviderConnection(
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
	if err := control.Logout(ctx, channelID.String()); err != nil {
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

func (s *Service) adapterControl(provider messaging.Provider) (AdapterControl, error) {
	s.controlsMu.RLock()
	control := s.controls[provider]
	s.controlsMu.RUnlock()
	if control == nil {
		return nil, fmt.Errorf("provider %q is not configured", provider)
	}
	return control, nil
}

func (s *Service) loadProviderConnection(
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

func (s *Service) updateProviderConnection(
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

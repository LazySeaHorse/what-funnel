package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	matrixadapter "github.com/whatfunnel/whatfunnel/adapters/matrix-mautrix"
	"github.com/whatfunnel/whatfunnel/packages/go-common/types"
)

// BridgeConnectionConfig is runtime-only control-plane configuration. Keeping
// it out of channel credentials prevents operator secrets from being exposed to
// a tenant or persisted alongside its Matrix access token.
type BridgeConnectionConfig struct {
	Provisioning     matrixadapter.ProvisioningConfig
	BridgeIdentities map[string]string
}

type bridgePlatform struct {
	Name         string
	ChannelType  string
	StartCommand string
	InitialState string
}

var bridgePlatforms = map[string]bridgePlatform{
	"whatsapp":  {Name: "whatsapp", ChannelType: "matrix_whatsapp", StartCommand: "login qr", InitialState: "awaiting_scan"},
	"telegram":  {Name: "telegram", ChannelType: "matrix_telegram", StartCommand: "login qr", InitialState: "awaiting_scan"},
	"instagram": {Name: "instagram", ChannelType: "matrix_instagram", StartCommand: "login", InitialState: "awaiting_session"},
	"messenger": {Name: "messenger", ChannelType: "matrix_messenger", StartCommand: "login", InitialState: "awaiting_session"},
}

func (s *Service) ConfigureBridgeConnections(cfg BridgeConnectionConfig) {
	s.bridgeConfig = cfg
}

func (s *Service) bridgePlatform(platform string) (bridgePlatform, error) {
	spec, ok := bridgePlatforms[strings.ToLower(platform)]
	if !ok {
		return bridgePlatform{}, fmt.Errorf("unsupported bridge platform: %s", platform)
	}
	return spec, nil
}

func (s *Service) StartBridgeConnection(ctx context.Context, accountID uuid.UUID, platform string) (*types.BridgeConnection, error) {
	spec, err := s.bridgePlatform(platform)
	if err != nil {
		return nil, err
	}
	if s.bridgeConfig.Provisioning.HomeserverURL == "" || s.bridgeConfig.Provisioning.ServerName == "" || s.bridgeConfig.Provisioning.RegistrationSharedSecret == "" {
		return nil, fmt.Errorf("bridge setup is not configured by this server operator")
	}
	bridgeIdentity := s.bridgeConfig.BridgeIdentities[spec.Name]
	if bridgeIdentity == "" {
		return nil, fmt.Errorf("the %s bridge is not configured by this server operator", spec.Name)
	}

	var existingID uuid.UUID
	err = s.pool.QueryRow(ctx, `SELECT channel_id FROM channel_connections WHERE account_id = $1 AND platform = $2 ORDER BY created_at DESC LIMIT 1`, accountID, spec.Name).Scan(&existingID)
	if err == nil {
		// Reconcile the current attempt before deciding whether POST means
		// "continue" or "retry". Attempt-scoped history makes this refresh safe.
		existing, loadErr := s.GetBridgeConnection(ctx, accountID, existingID, true)
		if loadErr != nil {
			return nil, loadErr
		}
		if existing.State == "failed" || (bridgeSetupInProgress(existing.State) && existing.LastEventID == "") {
			return s.restartBridgeConnection(ctx, existing)
		}
		if existing.State != "cancelled" {
			return existing, nil
		}
		// Disconnect revokes the old Matrix access token. A cancelled connection
		// therefore cannot be restarted safely; continue below to provision an
		// entirely new bridge identity for this new setup attempt.
	}
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("find existing connection: %w", err)
	}

	// Each setup attempt owns a separate least-privilege Matrix identity. This
	// avoids reusing a token that was deliberately revoked on disconnect.
	username := "wf_" + strings.ReplaceAll(accountID.String(), "-", "")[:16] + "_" + spec.Name + "_" + uuid.NewString()[:8]
	creds, err := matrixadapter.ProvisionUser(ctx, s.bridgeConfig.Provisioning, username)
	if err != nil {
		return nil, err
	}
	adapter, err := s.matrixAdapterFor(spec.ChannelType)
	if err != nil {
		return nil, err
	}
	roomID, err := adapter.CreateManagementRoom(ctx, creds, bridgeIdentity)
	if err != nil {
		return nil, err
	}
	if err := adapter.WaitForManagementRoomReady(ctx, creds, roomID, bridgeIdentity); err != nil {
		return nil, err
	}
	creds.ManagementRoomID = roomID
	rawCredentials, err := json.Marshal(creds)
	if err != nil {
		return nil, err
	}
	channel, err := s.CreateChannel(ctx, accountID, spec.ChannelType, &bridgeIdentity, rawCredentials)
	if err != nil {
		return nil, err
	}
	commandEventID, commandErr := adapter.SendManagementCommand(ctx, creds, roomID, spec.StartCommand)

	connection := &types.BridgeConnection{
		ChannelID: channel.ID, AccountID: accountID, Platform: spec.Name, BridgeIdentity: bridgeIdentity,
		ManagementRoomID: roomID, State: spec.InitialState, Detail: setupDetail(spec.Name, spec.InitialState), LastEventID: commandEventID,
	}
	if commandErr != nil {
		connection.State = "failed"
		connection.Detail = commandErr.Error()
		_, _ = s.pool.Exec(ctx, `UPDATE channels SET status = 'error', status_detail = $1 WHERE id = $2`, connection.Detail, channel.ID)
	}
	err = s.pool.QueryRow(ctx, `
		INSERT INTO channel_connections (channel_id, account_id, platform, bridge_identity, management_room_id, state, detail, last_event_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NULLIF($8, ''))
		RETURNING created_at, updated_at
	`, connection.ChannelID, connection.AccountID, connection.Platform, connection.BridgeIdentity, connection.ManagementRoomID, connection.State, connection.Detail, connection.LastEventID).Scan(&connection.CreatedAt, &connection.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("record bridge connection: %w", err)
	}
	if commandErr != nil {
		return connection, commandErr
	}
	return connection, nil
}

func (s *Service) restartBridgeConnection(ctx context.Context, connection *types.BridgeConnection) (*types.BridgeConnection, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin bridge restart: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	err = tx.QueryRow(ctx, `
		SELECT channel_id, account_id, platform, bridge_identity, COALESCE(management_room_id, ''), state,
		       COALESCE(detail, ''), COALESCE(last_event_id, ''), created_at, updated_at
		FROM channel_connections
		WHERE channel_id = $1 AND account_id = $2
		FOR UPDATE
	`, connection.ChannelID, connection.AccountID).Scan(
		&connection.ChannelID, &connection.AccountID, &connection.Platform, &connection.BridgeIdentity,
		&connection.ManagementRoomID, &connection.State, &connection.Detail, &connection.LastEventID,
		&connection.CreatedAt, &connection.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("lock bridge connection for restart: %w", err)
	}
	// Another request may have restarted the connection while this request was
	// waiting for the row lock. In that case, reuse the attempt it created.
	if connection.State != "failed" && connection.LastEventID != "" {
		return connection, nil
	}
	if connection.State == "cancelled" || connection.State == "connected" {
		return connection, nil
	}

	spec, err := s.bridgePlatform(connection.Platform)
	if err != nil {
		return nil, err
	}
	var encrypted []byte
	err = tx.QueryRow(ctx, `SELECT bridge_credentials FROM channels WHERE id = $1 AND account_id = $2`, connection.ChannelID, connection.AccountID).Scan(&encrypted)
	if err != nil {
		return nil, fmt.Errorf("load bridge credentials for restart: %w", err)
	}
	rawCredentials, err := s.DecryptCredentials(encrypted)
	if err != nil {
		return nil, err
	}
	var creds matrixadapter.Credentials
	if err := json.Unmarshal(rawCredentials, &creds); err != nil {
		return nil, err
	}
	adapter, err := s.matrixAdapterFor(spec.ChannelType)
	if err != nil {
		return nil, err
	}
	commandEventID, err := adapter.SendManagementCommand(ctx, creds, connection.ManagementRoomID, spec.StartCommand)
	if err != nil {
		return nil, err
	}
	connection.State = spec.InitialState
	connection.Detail = setupDetail(spec.Name, spec.InitialState)
	connection.LastEventID = commandEventID
	err = tx.QueryRow(ctx, `
		UPDATE channel_connections
		SET state = $1, detail = $2, last_event_id = $3, updated_at = NOW()
		WHERE channel_id = $4 AND account_id = $5
		RETURNING updated_at
	`, connection.State, connection.Detail, connection.LastEventID, connection.ChannelID, connection.AccountID).Scan(&connection.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("record bridge restart: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE channels SET status = 'pending', status_detail = $1 WHERE id = $2 AND account_id = $3`, connection.Detail, connection.ChannelID, connection.AccountID); err != nil {
		return nil, fmt.Errorf("mark restarted channel pending: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit bridge restart: %w", err)
	}
	return connection, nil
}

// GetBridgeConnection optionally refreshes status from the bridge management
// room. Refresh is read-only apart from persisting the observed state.
func (s *Service) GetBridgeConnection(ctx context.Context, accountID, channelID uuid.UUID, refresh bool) (*types.BridgeConnection, error) {
	connection := &types.BridgeConnection{}
	err := s.pool.QueryRow(ctx, `
		SELECT channel_id, account_id, platform, bridge_identity, COALESCE(management_room_id, ''), state,
		       COALESCE(detail, ''), COALESCE(last_event_id, ''), created_at, updated_at
		FROM channel_connections WHERE channel_id = $1 AND account_id = $2
	`, channelID, accountID).Scan(
		&connection.ChannelID, &connection.AccountID, &connection.Platform, &connection.BridgeIdentity,
		&connection.ManagementRoomID, &connection.State, &connection.Detail, &connection.LastEventID,
		&connection.CreatedAt, &connection.UpdatedAt,
	)
	if err != nil {
		if isNoRows(err) {
			return nil, fmt.Errorf("bridge connection not found")
		}
		return nil, fmt.Errorf("load bridge connection: %w", err)
	}
	if !refresh || connection.State == "connected" || connection.State == "cancelled" || connection.State == "failed" {
		return connection, nil
	}
	// Rows created before attempt cursors were introduced cannot safely
	// distinguish current replies from old room history. Starting/retrying the
	// connection creates a fresh command boundary.
	if connection.LastEventID == "" {
		return connection, nil
	}
	creds, err := s.channelMatrixCredentials(ctx, accountID, channelID)
	if err != nil {
		return connection, nil
	}
	spec, err := s.bridgePlatform(connection.Platform)
	if err != nil {
		return connection, err
	}
	adapter, err := s.matrixAdapterFor(spec.ChannelType)
	if err != nil {
		return connection, err
	}
	messages, err := adapter.ReadManagementMessagesSince(ctx, creds, connection.ManagementRoomID, connection.LastEventID)
	if err != nil {
		return connection, nil
	}
	nextState, detail := interpretBridgeMessages(connection, messages)
	if nextState != connection.State || detail != connection.Detail {
		return s.persistObservedBridgeState(ctx, connection, nextState, detail)
	}
	return connection, nil
}

func (s *Service) persistObservedBridgeState(ctx context.Context, connection *types.BridgeConnection, nextState, detail string) (*types.BridgeConnection, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin bridge state update: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	result, err := tx.Exec(ctx, `
		UPDATE channel_connections
		SET state = $1, detail = $2, updated_at = NOW()
		WHERE channel_id = $3 AND account_id = $4 AND last_event_id = $5 AND state = $6
	`, nextState, detail, connection.ChannelID, connection.AccountID, connection.LastEventID, connection.State)
	if err != nil {
		return nil, fmt.Errorf("update bridge connection state: %w", err)
	}
	if result.RowsAffected() == 0 {
		if err := tx.Rollback(ctx); err != nil {
			return nil, fmt.Errorf("rollback stale bridge state update: %w", err)
		}
		return s.GetBridgeConnection(ctx, connection.AccountID, connection.ChannelID, false)
	}

	switch nextState {
	case "connected":
		if _, err := tx.Exec(ctx, `UPDATE channels SET status = 'connected', status_detail = NULL WHERE id = $1 AND account_id = $2`, connection.ChannelID, connection.AccountID); err != nil {
			return nil, fmt.Errorf("mark connected channel: %w", err)
		}
	case "failed":
		if _, err := tx.Exec(ctx, `UPDATE channels SET status = 'error', status_detail = $1 WHERE id = $2 AND account_id = $3`, detail, connection.ChannelID, connection.AccountID); err != nil {
			return nil, fmt.Errorf("mark failed channel: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit bridge state update: %w", err)
	}

	connection.State = nextState
	connection.Detail = detail
	connection.UpdatedAt = time.Now().UTC()
	return connection, nil
}

func (s *Service) ListBridgeConnections(ctx context.Context, accountID uuid.UUID, refresh bool) ([]*types.BridgeConnection, error) {
	rows, err := s.pool.Query(ctx, `SELECT channel_id FROM channel_connections WHERE account_id = $1 ORDER BY created_at DESC`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	connections := []*types.BridgeConnection{}
	for rows.Next() {
		var channelID uuid.UUID
		if err := rows.Scan(&channelID); err != nil {
			return nil, err
		}
		connection, err := s.GetBridgeConnection(ctx, accountID, channelID, refresh)
		if err != nil {
			return nil, err
		}
		connections = append(connections, connection)
	}
	return connections, rows.Err()
}

// SubmitBridgeSecret is limited to Meta's login hand-off. It is kept in memory
// only long enough to forward it to the bridge, and is never logged or stored
// in WhatFunnel's database. Mautrix itself owns its management-room retention.
func (s *Service) SubmitBridgeSecret(ctx context.Context, accountID, channelID uuid.UUID, secret string) (*types.BridgeConnection, error) {
	if len(strings.TrimSpace(secret)) == 0 || len(secret) > 128*1024 {
		return nil, fmt.Errorf("session data must be between 1 and 131072 bytes")
	}
	connection, err := s.GetBridgeConnection(ctx, accountID, channelID, false)
	if err != nil {
		return nil, err
	}
	if connection.Platform != "instagram" && connection.Platform != "messenger" {
		return nil, fmt.Errorf("this platform does not use a browser session hand-off")
	}
	if connection.State != "awaiting_session" && connection.State != "connecting" {
		return nil, fmt.Errorf("this connection is not awaiting a browser session")
	}
	creds, err := s.channelMatrixCredentials(ctx, accountID, channelID)
	if err != nil {
		return nil, err
	}
	spec, _ := s.bridgePlatform(connection.Platform)
	adapter, err := s.matrixAdapterFor(spec.ChannelType)
	if err != nil {
		return nil, err
	}
	if _, err := adapter.SendManagementCommand(ctx, creds, connection.ManagementRoomID, secret); err != nil {
		return nil, err
	}
	connection.State, connection.Detail = "connecting", "Session received. Waiting for the bridge to verify it."
	_, _ = s.pool.Exec(ctx, `UPDATE channel_connections SET state = $1, detail = $2, updated_at = NOW() WHERE channel_id = $3`, connection.State, connection.Detail, channelID)
	return connection, nil
}

func (s *Service) SubmitBridgeCode(ctx context.Context, accountID, channelID uuid.UUID, value string) (*types.BridgeConnection, error) {
	if len(strings.TrimSpace(value)) == 0 || len(value) > 1024 {
		return nil, fmt.Errorf("login response is required")
	}
	connection, err := s.GetBridgeConnection(ctx, accountID, channelID, false)
	if err != nil {
		return nil, err
	}
	if connection.Platform != "telegram" {
		return nil, fmt.Errorf("this platform does not accept a login code")
	}
	creds, err := s.channelMatrixCredentials(ctx, accountID, channelID)
	if err != nil {
		return nil, err
	}
	adapter, err := s.matrixAdapterFor("matrix_telegram")
	if err != nil {
		return nil, err
	}
	if _, err := adapter.SendManagementCommand(ctx, creds, connection.ManagementRoomID, value); err != nil {
		return nil, err
	}
	connection.State, connection.Detail = "connecting", "Code received. Waiting for Telegram to confirm the login."
	_, _ = s.pool.Exec(ctx, `UPDATE channel_connections SET state = $1, detail = $2, updated_at = NOW() WHERE channel_id = $3`, connection.State, connection.Detail, channelID)
	return connection, nil
}

func (s *Service) BridgeQRCode(ctx context.Context, accountID, channelID uuid.UUID) ([]byte, string, error) {
	// Refresh from the bridge so a stale DB state does not produce a spurious
	// 400. For example, if the bridge has already sent a new QR message the
	// state must be current before we decide whether to look for it.
	connection, err := s.GetBridgeConnection(ctx, accountID, channelID, true)
	if err != nil {
		return nil, "", err
	}
	if connection.State != "awaiting_scan" {
		return nil, "", fmt.Errorf("this connection is not waiting for a QR scan (current state: %s)", connection.State)
	}
	creds, err := s.channelMatrixCredentials(ctx, accountID, channelID)
	if err != nil {
		return nil, "", err
	}
	spec, err := s.bridgePlatform(connection.Platform)
	if err != nil {
		return nil, "", err
	}
	adapter, err := s.matrixAdapterFor(spec.ChannelType)
	if err != nil {
		return nil, "", err
	}
	messages, err := adapter.ReadManagementMessagesSince(ctx, creds, connection.ManagementRoomID, connection.LastEventID)
	if err != nil {
		return nil, "", err
	}
	for _, message := range messages {
		if message.Sender == connection.BridgeIdentity && message.MediaURL != "" {
			return adapter.DownloadMedia(ctx, creds, message.MediaURL)
		}
	}
	return nil, "", fmt.Errorf("the bridge has not issued a QR code yet")
}

func (s *Service) matrixAdapterFor(channelType string) (*matrixadapter.Adapter, error) {
	adapter, err := s.GetAdapter(channelType)
	if err != nil {
		return nil, err
	}
	matrix, ok := adapter.(*matrixadapter.Adapter)
	if !ok {
		return nil, fmt.Errorf("%s does not use a Matrix bridge adapter", channelType)
	}
	return matrix, nil
}

func (s *Service) channelMatrixCredentials(ctx context.Context, accountID, channelID uuid.UUID) (matrixadapter.Credentials, error) {
	var encrypted []byte
	err := s.pool.QueryRow(ctx, `SELECT bridge_credentials FROM channels WHERE id = $1 AND account_id = $2`, channelID, accountID).Scan(&encrypted)
	if err != nil {
		return matrixadapter.Credentials{}, err
	}
	raw, err := s.DecryptCredentials(encrypted)
	if err != nil {
		return matrixadapter.Credentials{}, err
	}
	var credentials matrixadapter.Credentials
	if err := json.Unmarshal(raw, &credentials); err != nil {
		return matrixadapter.Credentials{}, err
	}
	return credentials, nil
}

func setupDetail(platform, state string) string {
	switch state {
	case "awaiting_scan":
		return "Scan the QR code in your mobile app to finish linking " + platform + "."
	case "awaiting_session":
		return "Open a private browser window, sign in, and provide the bridge session data."
	default:
		return "Waiting for the bridge."
	}
}

func bridgeSetupInProgress(state string) bool {
	switch state {
	case "awaiting_scan", "awaiting_phone", "awaiting_code", "awaiting_session", "connecting":
		return true
	default:
		return false
	}
}

func interpretBridgeMessages(connection *types.BridgeConnection, messages []matrixadapter.BridgeMessage) (string, string) {
	for _, message := range messages {
		if message.Sender != connection.BridgeIdentity {
			continue
		}
		text := strings.ToLower(message.Body)
		switch {
		case strings.Contains(text, "successfully logged in"), strings.Contains(text, "login successful"), strings.Contains(text, "logged in successfully"):
			return "connected", "Connected"
		case strings.Contains(text, "two-factor"), strings.Contains(text, "2fa"), strings.Contains(text, "enter the code"), strings.Contains(text, "enter your password"):
			return "awaiting_code", "Enter the response requested by Telegram."
		case strings.Contains(text, "error"), strings.Contains(text, "failed"), strings.Contains(text, "invalid"):
			return "failed", strings.TrimSpace(message.Body)
		case message.MediaURL != "":
			return "awaiting_scan", "Scan the fresh QR code in your mobile app."
		}
	}
	return connection.State, connection.Detail
}

func isNoRows(err error) bool { return err == pgx.ErrNoRows }

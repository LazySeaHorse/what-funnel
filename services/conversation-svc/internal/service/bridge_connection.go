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
		existing, loadErr := s.GetBridgeConnection(ctx, accountID, existingID, false)
		if loadErr != nil {
			return nil, loadErr
		}
		if existing.State == "failed" {
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
	creds.ManagementRoomID = roomID
	rawCredentials, err := json.Marshal(creds)
	if err != nil {
		return nil, err
	}
	channel, err := s.CreateChannel(ctx, accountID, spec.ChannelType, &bridgeIdentity, rawCredentials)
	if err != nil {
		return nil, err
	}
	_, commandErr := adapter.SendManagementCommand(ctx, creds, roomID, spec.StartCommand)

	connection := &types.BridgeConnection{
		ChannelID: channel.ID, AccountID: accountID, Platform: spec.Name, BridgeIdentity: bridgeIdentity,
		ManagementRoomID: roomID, State: spec.InitialState, Detail: setupDetail(spec.Name, spec.InitialState),
	}
	if commandErr != nil {
		connection.State = "failed"
		connection.Detail = commandErr.Error()
		_, _ = s.pool.Exec(ctx, `UPDATE channels SET status = 'error', status_detail = $1 WHERE id = $2`, connection.Detail, channel.ID)
	}
	err = s.pool.QueryRow(ctx, `
		INSERT INTO channel_connections (channel_id, account_id, platform, bridge_identity, management_room_id, state, detail)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING created_at, updated_at
	`, connection.ChannelID, connection.AccountID, connection.Platform, connection.BridgeIdentity, connection.ManagementRoomID, connection.State, connection.Detail).Scan(&connection.CreatedAt, &connection.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("record bridge connection: %w", err)
	}
	if commandErr != nil {
		return connection, commandErr
	}
	return connection, nil
}

func (s *Service) restartBridgeConnection(ctx context.Context, connection *types.BridgeConnection) (*types.BridgeConnection, error) {
	spec, err := s.bridgePlatform(connection.Platform)
	if err != nil {
		return nil, err
	}
	creds, err := s.channelMatrixCredentials(ctx, connection.AccountID, connection.ChannelID)
	if err != nil {
		return nil, err
	}
	adapter, err := s.matrixAdapterFor(spec.ChannelType)
	if err != nil {
		return nil, err
	}
	if _, err := adapter.SendManagementCommand(ctx, creds, connection.ManagementRoomID, spec.StartCommand); err != nil {
		return nil, err
	}
	connection.State = spec.InitialState
	connection.Detail = setupDetail(spec.Name, spec.InitialState)
	_, err = s.pool.Exec(ctx, `
		UPDATE channel_connections SET state = $1, detail = $2, updated_at = NOW() WHERE channel_id = $3
	`, connection.State, connection.Detail, connection.ChannelID)
	if err != nil {
		return nil, err
	}
	_, _ = s.pool.Exec(ctx, `UPDATE channels SET status = 'pending', status_detail = $1 WHERE id = $2`, connection.Detail, connection.ChannelID)
	return connection, nil
}

// GetBridgeConnection optionally refreshes status from the bridge management
// room. Refresh is read-only apart from persisting the observed state.
func (s *Service) GetBridgeConnection(ctx context.Context, accountID, channelID uuid.UUID, refresh bool) (*types.BridgeConnection, error) {
	connection := &types.BridgeConnection{}
	err := s.pool.QueryRow(ctx, `
		SELECT channel_id, account_id, platform, bridge_identity, COALESCE(management_room_id, ''), state, COALESCE(detail, ''), created_at, updated_at
		FROM channel_connections WHERE channel_id = $1 AND account_id = $2
	`, channelID, accountID).Scan(&connection.ChannelID, &connection.AccountID, &connection.Platform, &connection.BridgeIdentity, &connection.ManagementRoomID, &connection.State, &connection.Detail, &connection.CreatedAt, &connection.UpdatedAt)
	if err != nil {
		if isNoRows(err) {
			return nil, fmt.Errorf("bridge connection not found")
		}
		return nil, fmt.Errorf("load bridge connection: %w", err)
	}
	if !refresh || connection.State == "connected" || connection.State == "cancelled" || connection.State == "failed" {
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
	messages, err := adapter.ReadManagementMessages(ctx, creds, connection.ManagementRoomID)
	if err != nil {
		return connection, nil
	}
	nextState, detail := interpretBridgeMessages(connection, messages)
	if nextState != connection.State || detail != connection.Detail {
		connection.State, connection.Detail = nextState, detail
		connection.UpdatedAt = time.Now().UTC()
		_, _ = s.pool.Exec(ctx, `UPDATE channel_connections SET state = $1, detail = $2, updated_at = NOW() WHERE channel_id = $3`, nextState, detail, channelID)
		if nextState == "connected" {
			_, _ = s.pool.Exec(ctx, `UPDATE channels SET status = 'connected', status_detail = NULL WHERE id = $1`, channelID)
		}
		if nextState == "failed" {
			_, _ = s.pool.Exec(ctx, `UPDATE channels SET status = 'error', status_detail = $1 WHERE id = $2`, detail, channelID)
		}
	}
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
	messages, err := adapter.ReadManagementMessages(ctx, creds, connection.ManagementRoomID)
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

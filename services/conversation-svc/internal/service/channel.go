package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	matrixadapter "github.com/whatfunnel/whatfunnel/adapters/matrix-mautrix"
	"github.com/whatfunnel/whatfunnel/packages/go-common/audit"
	"github.com/whatfunnel/whatfunnel/packages/go-common/types"
)

// CreateChannel creates a new channel record with encrypted credentials.
func (s *Service) CreateChannel(ctx context.Context, accountID uuid.UUID, channelType string, bridgeIdentity *string, rawCredentials []byte) (*types.Channel, error) {
	var encryptedCreds []byte
	var err error
	if len(rawCredentials) > 0 {
		encryptedCreds, err = s.EncryptCredentials(rawCredentials)
		if err != nil {
			return nil, fmt.Errorf("encrypt channel credentials: %w", err)
		}
	}

	ch := &types.Channel{}
	err = s.pool.QueryRow(ctx, `
		INSERT INTO channels (account_id, type, bridge_identity, bridge_credentials, status)
		VALUES ($1, $2, $3, $4, 'pending')
		RETURNING id, account_id, type, bridge_identity, bridge_credentials, status, status_detail, created_at
	`, accountID, channelType, bridgeIdentity, encryptedCreds).Scan(
		&ch.ID, &ch.AccountID, &ch.Type, &ch.BridgeIdentity,
		&ch.BridgeCredentials, &ch.Status, &ch.StatusDetail, &ch.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert channel: %w", err)
	}

	// Register adapter credentials immediately so outbound calls can proceed
	// without waiting for the next InitAdapters run.
	if len(rawCredentials) > 0 {
		if adapter, err := s.GetAdapter(channelType); err == nil {
			if configurable, ok := adapter.(interface {
				Configure(channelID string, config matrixadapter.ChannelConfig)
			}); ok {
				var mc matrixadapter.Credentials
				if json.Unmarshal(rawCredentials, &mc) == nil {
					config := matrixadapter.ChannelConfig{Credentials: mc}
					if bridgeIdentity != nil {
						config.BridgeIdentity = *bridgeIdentity
					}
					configurable.Configure(ch.ID.String(), config)
				}
			}
		}
	}

	return ch, nil
}

// GetChannel returns a channel scoped to the given account.
func (s *Service) GetChannel(ctx context.Context, accountID, channelID uuid.UUID) (*types.Channel, error) {
	ch := &types.Channel{}
	err := s.pool.QueryRow(ctx, `
		SELECT id, account_id, type, bridge_identity, bridge_credentials, status, status_detail, created_at
		FROM channels
		WHERE id = $1 AND account_id = $2
	`, channelID, accountID).Scan(
		&ch.ID, &ch.AccountID, &ch.Type, &ch.BridgeIdentity,
		&ch.BridgeCredentials, &ch.Status, &ch.StatusDetail, &ch.CreatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("channel not found")
		}
		return nil, err
	}
	return ch, nil
}

// ListChannels returns all channels for an account, enriching each with the
// live adapter status where available. Status sync is a best-effort read: a
// failed sync is silently skipped so the list always returns.
func (s *Service) ListChannels(ctx context.Context, accountID uuid.UUID) ([]*types.Channel, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, account_id, type, bridge_identity, bridge_credentials, status, status_detail, created_at
		FROM channels
		WHERE account_id = $1
		ORDER BY created_at ASC
	`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var channels []*types.Channel
	for rows.Next() {
		ch := &types.Channel{}
		if err := rows.Scan(
			&ch.ID, &ch.AccountID, &ch.Type, &ch.BridgeIdentity,
			&ch.BridgeCredentials, &ch.Status, &ch.StatusDetail, &ch.CreatedAt,
		); err != nil {
			return nil, err
		}

		// A user-disconnected channel must remain disconnected. Polling its
		// adapter would otherwise overwrite that explicit state with a stale
		// connection status.
		if ch.Status != "disconnected" && ch.Status != "pending" {
			s.syncChannelStatus(ctx, ch)
		}

		channels = append(channels, ch)
	}
	return channels, rows.Err()
}

// syncChannelStatus refreshes ch.Status and ch.StatusDetail from the live
// adapter and persists the result. Errors are logged but never returned so
// callers (e.g. ListChannels) are not affected by a momentary adapter failure.
func (s *Service) syncChannelStatus(ctx context.Context, ch *types.Channel) {
	adapter, err := s.GetAdapter(ch.Type)
	if err != nil {
		return
	}
	status := adapter.Status(ch.ID.String())
	ch.Status = status.Status
	ch.StatusDetail = &status.Detail
	if _, err := s.pool.Exec(ctx,
		`UPDATE channels SET status = $1, status_detail = $2 WHERE id = $3`,
		status.Status, status.Detail, ch.ID,
	); err != nil {
		fmt.Printf("syncChannelStatus: failed to persist status for channel %s: %v\n", ch.ID, err)
	}
}

// GetChannelStatus returns the live status for a single channel, falling back
// to the DB-persisted value when no adapter is registered.
func (s *Service) GetChannelStatus(ctx context.Context, accountID, channelID uuid.UUID) (types.ChannelStatus, error) {
	var chType string
	err := s.pool.QueryRow(ctx, `SELECT type FROM channels WHERE id = $1 AND account_id = $2`, channelID, accountID).Scan(&chType)
	if err != nil {
		if err == pgx.ErrNoRows {
			return types.ChannelStatus{}, fmt.Errorf("channel not found")
		}
		return types.ChannelStatus{}, err
	}

	adapter, err := s.GetAdapter(chType)
	if err != nil {
		// Fallback to DB status if no live adapter is running.
		var dbStatus string
		var rawDetail *string
		if err = s.pool.QueryRow(ctx,
			`SELECT status, status_detail FROM channels WHERE id = $1`, channelID,
		).Scan(&dbStatus, &rawDetail); err != nil {
			return types.ChannelStatus{}, err
		}
		detail := ""
		if rawDetail != nil {
			detail = *rawDetail
		}
		return types.ChannelStatus{Status: dbStatus, Detail: detail}, nil
	}

	status := adapter.Status(channelID.String())
	_, _ = s.pool.Exec(ctx,
		`UPDATE channels SET status = $1, status_detail = $2 WHERE id = $3`,
		status.Status, status.Detail, channelID,
	)
	return status, nil
}

// DisconnectChannel logs out of the remote bridge before revoking the locally
// stored Matrix access token. A failed remote logout leaves the channel intact
// so an administrator can retry instead of silently retaining a live session.
func (s *Service) DisconnectChannel(ctx context.Context, accountID, channelID uuid.UUID) error {
	var connection *types.BridgeConnection
	if candidate, err := s.GetBridgeConnection(ctx, accountID, channelID, false); err == nil {
		connection = candidate
		creds, err := s.channelMatrixCredentials(ctx, accountID, channelID)
		if err != nil {
			return fmt.Errorf("load bridge credentials for logout: %w", err)
		}
		spec, err := s.bridgePlatform(connection.Platform)
		if err != nil {
			return err
		}
		adapter, err := s.matrixAdapterFor(spec.ChannelType)
		if err != nil {
			return err
		}
		if _, err := adapter.SendManagementCommand(ctx, creds, connection.ManagementRoomID, "logout"); err != nil {
			return fmt.Errorf("log out of %s bridge: %w", connection.Platform, err)
		}
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var typeName string
	err = tx.QueryRow(ctx, `SELECT type FROM channels WHERE id = $1 AND account_id = $2`, channelID, accountID).Scan(&typeName)
	if err != nil {
		if err == pgx.ErrNoRows {
			return fmt.Errorf("channel not found")
		}
		return err
	}

	_, err = tx.Exec(ctx, `
		UPDATE channels
		SET status = 'disconnected', status_detail = 'Disconnected by admin', bridge_credentials = NULL
		WHERE id = $1 AND account_id = $2
	`, channelID, accountID)
	if err != nil {
		return fmt.Errorf("disconnect channel: %w", err)
	}

	aw := audit.NewWriterFromTx(tx)
	if err = aw.Write(ctx, audit.Entry{
		AccountID:  accountID,
		Action:     "channel.disconnected",
		TargetType: "channel",
		TargetID:   &channelID,
		Metadata:   map[string]any{},
	}); err != nil {
		return fmt.Errorf("write audit log: %w", err)
	}

	if connection != nil {
		if _, err := tx.Exec(ctx,
			`UPDATE channel_connections SET state = 'cancelled', detail = 'Disconnected by admin', updated_at = NOW() WHERE channel_id = $1`,
			channelID,
		); err != nil {
			return fmt.Errorf("cancel bridge connection: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}

	if matrix, err := s.matrixAdapterFor(typeName); err == nil {
		matrix.Remove(channelID.String())
	}
	return nil
}

// ChannelWebhookCredentials represents decrypted webhook verification credentials for a channel.
type ChannelWebhookCredentials struct {
	AppSecret     string `json:"app_secret,omitempty"`
	VerifyToken   string `json:"verify_token,omitempty"`
	SecretToken   string `json:"secret_token,omitempty"`
	WebhookSecret string `json:"webhook_secret,omitempty"`
}

// GetChannelWebhookCredentials retrieves and decrypts the credentials for a channel by ID.
func (s *Service) GetChannelWebhookCredentials(ctx context.Context, channelID uuid.UUID) (*ChannelWebhookCredentials, *types.Channel, error) {
	ch := &types.Channel{}
	err := s.pool.QueryRow(ctx, `
		SELECT id, account_id, type, bridge_identity, bridge_credentials, status, status_detail, created_at
		FROM channels
		WHERE id = $1
	`, channelID).Scan(
		&ch.ID, &ch.AccountID, &ch.Type, &ch.BridgeIdentity,
		&ch.BridgeCredentials, &ch.Status, &ch.StatusDetail, &ch.CreatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil, fmt.Errorf("channel not found")
		}
		return nil, nil, err
	}

	creds := &ChannelWebhookCredentials{}
	if len(ch.BridgeCredentials) > 0 && s.cipher != nil {
		decrypted, err := s.DecryptCredentials(ch.BridgeCredentials)
		if err == nil && len(decrypted) > 0 {
			plainCreds := decrypted
			var maybeStr string
			if json.Unmarshal(decrypted, &maybeStr) == nil {
				plainCreds = []byte(maybeStr)
			}
			var rawMap map[string]any
			if err := json.Unmarshal(plainCreds, &rawMap); err == nil {
				if v, ok := rawMap["app_secret"].(string); ok {
					creds.AppSecret = v
				}
				if v, ok := rawMap["secret"].(string); ok && creds.AppSecret == "" {
					creds.AppSecret = v
				}
				if v, ok := rawMap["verify_token"].(string); ok {
					creds.VerifyToken = v
				}
				if v, ok := rawMap["secret_token"].(string); ok {
					creds.SecretToken = v
				}
				if v, ok := rawMap["webhook_secret"].(string); ok {
					creds.WebhookSecret = v
				}
				if v, ok := rawMap["token"].(string); ok && creds.SecretToken == "" {
					creds.SecretToken = v
				}
			}
		}
	}
	return creds, ch, nil
}

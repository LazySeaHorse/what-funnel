package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/whatfunnel/whatfunnel/packages/go-common/types"
)

var ErrChannelNotFound = errors.New("channel not found")
var ErrUnsupportedSimulatorProvider = errors.New("unsupported simulator provider")

// EnsureSimulatorChannel returns a synthetic channel used only by the local
// simulation UI. It deliberately has no provider_connections row, so it can
// never be mistaken for a paired provider account or reach a live adapter.
func (s *Service) EnsureSimulatorChannel(ctx context.Context, accountID uuid.UUID, provider string) (*types.Channel, error) {
	if provider != "whatsapp" && provider != "telegram" {
		return nil, ErrUnsupportedSimulatorProvider
	}

	label := "Simulator " + provider
	channel := &types.Channel{}
	err := s.pool.QueryRow(ctx, `
		INSERT INTO channels (
			account_id, type, status, label, provider, remote_account_id, capabilities
		) VALUES ($1, $2, 'connected', $3, $2, $4, '{"media":true,"replies":true,"reactions":true}')
		ON CONFLICT (account_id, provider, (LOWER(label)))
			WHERE provider IS NOT NULL AND label IS NOT NULL
		DO UPDATE SET updated_at = channels.updated_at
		RETURNING id, account_id, type, status, status_detail, label, provider,
		          remote_account_id, capabilities, created_at, updated_at
	`, accountID, provider, label, "simulator:"+provider).Scan(
		&channel.ID, &channel.AccountID, &channel.Type, &channel.Status,
		&channel.StatusDetail, &channel.Label, &channel.Provider,
		&channel.RemoteAccountID, &channel.Capabilities,
		&channel.CreatedAt, &channel.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("ensure simulator channel: %w", err)
	}
	return channel, nil
}

// ListChannels is a read-only compatibility view used by the workspace and
// inbox. Channel lifecycle belongs exclusively to provider connections.
func (s *Service) ListChannels(ctx context.Context, accountID uuid.UUID) ([]*types.Channel, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, account_id, type, status, status_detail, label, provider,
		       remote_account_id, capabilities, created_at, updated_at
		FROM channels
		WHERE account_id = $1
		ORDER BY created_at
	`, accountID)
	if err != nil {
		return nil, fmt.Errorf("list channels: %w", err)
	}
	defer rows.Close()
	channels := make([]*types.Channel, 0)
	for rows.Next() {
		channel := &types.Channel{}
		if err := rows.Scan(
			&channel.ID, &channel.AccountID, &channel.Type, &channel.Status,
			&channel.StatusDetail, &channel.Label, &channel.Provider,
			&channel.RemoteAccountID, &channel.Capabilities,
			&channel.CreatedAt, &channel.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan channel: %w", err)
		}
		channels = append(channels, channel)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate channels: %w", err)
	}
	return channels, nil
}

func (s *Service) GetChannel(ctx context.Context, accountID, channelID uuid.UUID) (*types.Channel, error) {
	channel := &types.Channel{}
	err := s.pool.QueryRow(ctx, `
		SELECT id, account_id, type, status, status_detail, label, provider,
		       remote_account_id, capabilities, created_at, updated_at
		FROM channels
		WHERE id = $1 AND account_id = $2
	`, channelID, accountID).Scan(
		&channel.ID, &channel.AccountID, &channel.Type, &channel.Status,
		&channel.StatusDetail, &channel.Label, &channel.Provider,
		&channel.RemoteAccountID, &channel.Capabilities,
		&channel.CreatedAt, &channel.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrChannelNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get channel: %w", err)
	}
	return channel, nil
}

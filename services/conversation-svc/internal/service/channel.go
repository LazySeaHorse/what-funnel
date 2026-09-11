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

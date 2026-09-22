package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/whatfunnel/whatfunnel/packages/go-common/audit"
	"github.com/whatfunnel/whatfunnel/packages/go-common/types"
)

type rowQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func accountIDForChannel(ctx context.Context, q rowQuerier, channelID uuid.UUID) (uuid.UUID, error) {
	var accountID uuid.UUID
	err := q.QueryRow(ctx, `SELECT account_id FROM channels WHERE id = $1`, channelID).Scan(&accountID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, ErrChannelNotFound
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("lookup channel account: %w", err)
	}
	return accountID, nil
}

func createInitialLeadIfEnabled(ctx context.Context, tx pgx.Tx, resolver PipelineResolver, accountID, conversationID uuid.UUID) (*uuid.UUID, error) {
	var settingsRaw []byte
	var productMode string
	if err := tx.QueryRow(ctx, `SELECT product_mode, settings FROM accounts WHERE id = $1`, accountID).Scan(&productMode, &settingsRaw); err != nil {
		return nil, fmt.Errorf("get account settings for auto-lead: %w", err)
	}
	if !types.IsLeadTrackingEnabledForProduct(productMode, settingsRaw) {
		return nil, nil
	}

	if resolver == nil {
		resolver = &DBLeadPipelineResolver{}
	}
	pipeline, err := resolver.ResolveInitialPipeline(ctx, tx, accountID)
	if err != nil {
		return nil, fmt.Errorf("get lead pipeline for auto-lead: %w", err)
	}
	if pipeline == nil {
		return nil, nil
	}

	var leadID uuid.UUID
	err = tx.QueryRow(ctx, `
		INSERT INTO leads (account_id, conversation_id, pipeline_id, current_state_key)
		VALUES ($1, $2, $3, $4)
		RETURNING id
	`, accountID, conversationID, pipeline.ID, pipeline.FirstStateKey).Scan(&leadID)
	if err != nil {
		return nil, fmt.Errorf("auto-create lead: %w", err)
	}
	if _, err = tx.Exec(ctx, `
		INSERT INTO lead_state_history (account_id, lead_id, from_state, to_state)
		VALUES ($1, $2, NULL, $3)
	`, accountID, leadID, pipeline.FirstStateKey); err != nil {
		return nil, fmt.Errorf("insert lead state history for auto-lead: %w", err)
	}

	aw := audit.NewWriterFromTx(tx)
	if err = aw.Write(ctx, audit.Entry{
		AccountID:   accountID,
		ActorUserID: nil,
		Action:      "lead.created",
		TargetType:  "lead",
		TargetID:    &leadID,
		Metadata: map[string]any{
			"conversation_id":   conversationID,
			"current_state_key": pipeline.FirstStateKey,
			"auto":              true,
		},
	}); err != nil {
		return nil, fmt.Errorf("write auto-lead audit log: %w", err)
	}
	return &leadID, nil
}

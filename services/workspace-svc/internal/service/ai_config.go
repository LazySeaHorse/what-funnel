package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/whatfunnel/whatfunnel/packages/go-common/audit"
)

// UpdateAIProviderConfig encrypts and stores the AI provider config.
// The plaintext is never stored; only the AES-256-GCM ciphertext reaches Postgres.
func (svc *Service) UpdateAIProviderConfig(ctx context.Context, accountID, actorID uuid.UUID, plaintext string) error {
	encrypted, err := svc.cipher.Encrypt([]byte(plaintext))
	if err != nil {
		return fmt.Errorf("encrypt ai_provider_config: %w", err)
	}

	tx, err := svc.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	_, err = tx.Exec(ctx, `UPDATE accounts SET ai_provider_config = $1 WHERE id = $2`, encrypted, accountID)
	if err != nil {
		return fmt.Errorf("store ai_provider_config: %w", err)
	}

	aw := audit.NewWriterFromTx(tx)
	if err := aw.Write(ctx, audit.Entry{
		AccountID:   accountID,
		ActorUserID: &actorID,
		Action:      "account.ai_provider_config_updated",
		TargetType:  audit.TargetAccount,
		TargetID:    &accountID,
		Metadata:    map[string]any{"note": "encrypted value stored"},
	}); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// GetAIProviderConfig decrypts and returns the AI provider config plaintext.
// Returns empty string if not configured.
func (svc *Service) GetAIProviderConfig(ctx context.Context, accountID uuid.UUID) (string, error) {
	var encrypted *string
	err := svc.pool.QueryRow(ctx,
		`SELECT ai_provider_config FROM accounts WHERE id = $1`, accountID).
		Scan(&encrypted)
	if err != nil {
		return "", fmt.Errorf("get ai_provider_config: %w", err)
	}
	if encrypted == nil || *encrypted == "" {
		return "", nil
	}
	plain, err := svc.cipher.Decrypt(*encrypted)
	if err != nil {
		return "", fmt.Errorf("decrypt ai_provider_config: %w", err)
	}
	return string(plain), nil
}

// HasAIProviderConfig reports configuration presence without exposing or
// decrypting provider credentials.
func (svc *Service) HasAIProviderConfig(ctx context.Context, accountID uuid.UUID) (bool, error) {
	var configured bool
	err := svc.pool.QueryRow(ctx,
		`SELECT ai_provider_config IS NOT NULL AND ai_provider_config <> '' FROM accounts WHERE id = $1`,
		accountID).Scan(&configured)
	if err != nil {
		return false, fmt.Errorf("get ai provider status: %w", err)
	}
	return configured, nil
}

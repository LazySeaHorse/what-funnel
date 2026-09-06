package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	matrixadapter "github.com/whatfunnel/whatfunnel/adapters/matrix-mautrix"
	"github.com/whatfunnel/whatfunnel/packages/go-common/types"

	"github.com/google/uuid"
)

// RegisterAdapter associates a channel type with an adapter instance.
func (s *Service) RegisterAdapter(channelType string, adapter types.ChannelAdapter) {
	s.adaptersMu.Lock()
	defer s.adaptersMu.Unlock()
	s.adapters[channelType] = adapter
}

// GetAdapter retrieves the adapter for a channel type.
func (s *Service) GetAdapter(channelType string) (types.ChannelAdapter, error) {
	s.adaptersMu.RLock()
	defer s.adaptersMu.RUnlock()
	adapter, ok := s.adapters[channelType]
	if !ok {
		return nil, fmt.Errorf("no adapter registered for channel type: %s", channelType)
	}
	return adapter, nil
}

// InitAdapters loads all channels from the DB, decrypts credentials, and
// configures each registered adapter. A legacy double-encoded credential format
// is handled by attempting to unwrap a JSON-string before the final unmarshal.
func (s *Service) InitAdapters(ctx context.Context) error {
	rows, err := s.pool.Query(ctx, `SELECT id, type, COALESCE(bridge_identity, ''), bridge_credentials FROM channels`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var id uuid.UUID
		var channelType string
		var bridgeIdentity string
		var dbCreds []byte
		if err := rows.Scan(&id, &channelType, &bridgeIdentity, &dbCreds); err != nil {
			return err
		}
		if len(dbCreds) == 0 {
			continue
		}

		decrypted, err := s.DecryptCredentials(dbCreds)
		if err != nil {
			continue
		}

		adapter, err := s.GetAdapter(channelType)
		if err != nil {
			continue
		}
		configurable, ok := adapter.(interface {
			Configure(channelID string, config matrixadapter.ChannelConfig)
		})
		if !ok {
			continue
		}

		// Normalise legacy double-encoded format: if the bytes decode to a
		// JSON string, unwrap that string and use it as the real JSON object.
		plainCreds := decrypted
		var maybeStr string
		if json.Unmarshal(decrypted, &maybeStr) == nil {
			plainCreds = []byte(maybeStr)
		}

		var mc matrixadapter.Credentials
		if err := json.Unmarshal(plainCreds, &mc); err == nil {
			configurable.Configure(id.String(), matrixadapter.ChannelConfig{
				Credentials:    mc,
				BridgeIdentity: bridgeIdentity,
			})
		}
	}
	return nil
}

// EncryptCredentials encrypts raw credentials for storing in Postgres.
func (s *Service) EncryptCredentials(creds []byte) ([]byte, error) {
	ciphertext, err := s.cipher.Encrypt(creds)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt credentials: %w", err)
	}
	return json.Marshal(map[string]string{"encrypted_data": ciphertext})
}

// DecryptCredentials decrypts credentials retrieved from Postgres.
func (s *Service) DecryptCredentials(dbCreds []byte) ([]byte, error) {
	if len(dbCreds) == 0 {
		return nil, nil
	}
	var payload map[string]string
	if err := json.Unmarshal(dbCreds, &payload); err != nil {
		return nil, fmt.Errorf("failed to unmarshal encrypted credentials wrapper: %w", err)
	}
	ciphertext, ok := payload["encrypted_data"]
	if !ok {
		return nil, errors.New("invalid credentials structure: missing encrypted_data")
	}
	return s.cipher.Decrypt(ciphertext)
}

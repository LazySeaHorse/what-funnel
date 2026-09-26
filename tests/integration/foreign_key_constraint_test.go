package integration

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestForeignKeyConstraintEnforcement tests negative database constraints.
// It inserts orphaned records across primary entities to verify foreign key constraints
// trigger errors (PostgreSQL error 23503 foreign_key_violation).
func TestForeignKeyConstraintEnforcement(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	pool := testPool(t)
	ctx := context.Background()

	// Create a valid account for tests that need one valid FK and one invalid FK
	var validAccountID uuid.UUID
	err := pool.QueryRow(ctx, `
		INSERT INTO accounts (name)
		VALUES ('FK Constraint Test Account')
		RETURNING id;
	`).Scan(&validAccountID)
	require.NoError(t, err)

	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, "DELETE FROM accounts WHERE id = $1", validAccountID)
	})

	t.Run("Message with non-existent conversation_id triggers FK error", func(t *testing.T) {
		orphanedConvoID := uuid.New()
		_, err := pool.Exec(ctx, `
			INSERT INTO messages (account_id, conversation_id, direction, sender_type, content_type, content)
			VALUES ($1, $2, 'inbound', 'contact', 'text', '{"text":"orphaned"}')
		`, validAccountID, orphanedConvoID)

		require.Error(t, err, "Expected foreign key violation when inserting message with non-existent conversation")
		var pgErr *pgconn.PgError
		if assert.ErrorAs(t, err, &pgErr) {
			assert.Equal(t, "23503", pgErr.Code, "Expected foreign_key_violation SQLSTATE 23503")
			assert.Contains(t, pgErr.ConstraintName, "messages_conversation_id_fkey")
		}
	})

	t.Run("Message with non-existent account_id triggers FK error", func(t *testing.T) {
		orphanedAccountID := uuid.New()
		orphanedConvoID := uuid.New()
		_, err := pool.Exec(ctx, `
			INSERT INTO messages (account_id, conversation_id, direction, sender_type, content_type, content)
			VALUES ($1, $2, 'inbound', 'contact', 'text', '{"text":"orphaned"}')
		`, orphanedAccountID, orphanedConvoID)

		require.Error(t, err)
		var pgErr *pgconn.PgError
		if assert.ErrorAs(t, err, &pgErr) {
			assert.Equal(t, "23503", pgErr.Code)
		}
	})

	t.Run("Conversation with non-existent contact_id triggers FK error", func(t *testing.T) {
		// Create a valid channel first
		var channelID uuid.UUID
		err := pool.QueryRow(ctx, `
			INSERT INTO channels (account_id, type, status)
			VALUES ($1, 'webchat', 'connected')
			RETURNING id;
		`, validAccountID).Scan(&channelID)
		require.NoError(t, err)

		orphanedContactID := uuid.New()
		_, err = pool.Exec(ctx, `
			INSERT INTO conversations (account_id, contact_id, channel_id, status)
			VALUES ($1, $2, $3, 'open')
		`, validAccountID, orphanedContactID, channelID)

		require.Error(t, err)
		var pgErr *pgconn.PgError
		if assert.ErrorAs(t, err, &pgErr) {
			assert.Equal(t, "23503", pgErr.Code)
			assert.Contains(t, pgErr.ConstraintName, "conversations_contact_id_fkey")
		}
	})

	t.Run("Conversation with non-existent channel_id triggers FK error", func(t *testing.T) {
		// Create a valid channel then valid contact
		var channelID uuid.UUID
		err := pool.QueryRow(ctx, `
			INSERT INTO channels (account_id, type, status)
			VALUES ($1, 'webchat', 'connected')
			RETURNING id;
		`, validAccountID).Scan(&channelID)
		require.NoError(t, err)

		var contactID uuid.UUID
		err = pool.QueryRow(ctx, `
			INSERT INTO contacts (account_id, channel_id, external_identity, display_name)
			VALUES ($1, $2, $3, 'Test Person')
			RETURNING id;
		`, validAccountID, channelID, uuid.NewString()).Scan(&contactID)
		require.NoError(t, err)

		orphanedChannelID := uuid.New()
		_, err = pool.Exec(ctx, `
			INSERT INTO conversations (account_id, contact_id, channel_id, status)
			VALUES ($1, $2, $3, 'open')
		`, validAccountID, contactID, orphanedChannelID)

		require.Error(t, err)
		var pgErr *pgconn.PgError
		if assert.ErrorAs(t, err, &pgErr) {
			assert.Equal(t, "23503", pgErr.Code)
			assert.Contains(t, pgErr.ConstraintName, "conversations_channel_id_fkey")
		}
	})

	t.Run("User with non-existent account_id triggers FK error", func(t *testing.T) {
		orphanedAccountID := uuid.New()
		_, err := pool.Exec(ctx, `
			INSERT INTO users (account_id, email, password_hash, role)
			VALUES ($1, 'ghost@example.com', 'hashed', 'agent')
		`, orphanedAccountID)

		require.Error(t, err)
		var pgErr *pgconn.PgError
		if assert.ErrorAs(t, err, &pgErr) {
			assert.Equal(t, "23503", pgErr.Code)
			assert.Contains(t, pgErr.ConstraintName, "users_account_id_fkey")
		}
	})
}

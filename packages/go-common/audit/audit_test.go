package audit_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/whatfunnel/whatfunnel/packages/go-common/audit"
	"github.com/whatfunnel/whatfunnel/packages/go-common/db"
)

func TestRunInTx(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping database test in short mode")
	}
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://whatfunnel:whatfunnel@localhost:5432/whatfunnel?sslmode=disable"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := db.Connect(ctx, dsn)
	if err != nil {
		if os.Getenv("CI") != "" {
			t.Fatalf("database unavailable in CI: %v", err)
		}
		t.Skipf("skipping test: database unavailable: %v", err)
	}
	defer pool.Close()

	var accountID uuid.UUID
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO accounts (name) VALUES ('audit-runintx-test') RETURNING id`).Scan(&accountID))
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM accounts WHERE id = $1`, accountID)
	})

	count := func(action string) int {
		var n int
		require.NoError(t, pool.QueryRow(ctx,
			`SELECT count(*) FROM audit_logs WHERE account_id = $1 AND action = $2`, accountID, action).Scan(&n))
		return n
	}
	entry := func(action string) audit.Entry {
		return audit.Entry{AccountID: accountID, Action: action, TargetType: audit.TargetAccount, TargetID: &accountID}
	}

	t.Run("commits mutation and audit row together", func(t *testing.T) {
		err := audit.RunInTx(ctx, pool, func(tx pgx.Tx) (audit.Entry, error) {
			_, err := tx.Exec(ctx, `UPDATE accounts SET name = 'renamed' WHERE id = $1`, accountID)
			return entry("test.commit"), err
		})
		require.NoError(t, err)
		assert.Equal(t, 1, count("test.commit"))
		var name string
		require.NoError(t, pool.QueryRow(ctx, `SELECT name FROM accounts WHERE id = $1`, accountID).Scan(&name))
		assert.Equal(t, "renamed", name)
	})

	t.Run("callback error rolls back and writes no audit row", func(t *testing.T) {
		boom := errors.New("boom")
		err := audit.RunInTx(ctx, pool, func(tx pgx.Tx) (audit.Entry, error) {
			_, _ = tx.Exec(ctx, `UPDATE accounts SET name = 'rolled-back' WHERE id = $1`, accountID)
			return entry("test.rollback"), boom
		})
		require.ErrorIs(t, err, boom)
		assert.Equal(t, 0, count("test.rollback"))
		var name string
		require.NoError(t, pool.QueryRow(ctx, `SELECT name FROM accounts WHERE id = $1`, accountID).Scan(&name))
		assert.Equal(t, "renamed", name)
	})

	t.Run("failed audit write rolls the mutation back", func(t *testing.T) {
		err := audit.RunInTx(ctx, pool, func(tx pgx.Tx) (audit.Entry, error) {
			_, err := tx.Exec(ctx, `UPDATE accounts SET name = 'audit-fails' WHERE id = $1`, accountID)
			e := entry("test.audit_fails")
			e.AccountID = uuid.New() // violates the audit_logs foreign key
			return e, err
		})
		require.Error(t, err)
		var name string
		require.NoError(t, pool.QueryRow(ctx, `SELECT name FROM accounts WHERE id = $1`, accountID).Scan(&name))
		assert.Equal(t, "renamed", name)
	})
}

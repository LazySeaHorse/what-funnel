package db_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/whatfunnel/whatfunnel/packages/go-common/db"
)

// testPool returns a pgxpool connected to the test database, or skips the test
// if DATABASE_URL is not set. Integration tests require `make up` first.
func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://whatfunnel:whatfunnel@localhost:5432/whatfunnel?sslmode=disable"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := db.Connect(ctx, dsn)
	if err != nil {
		t.Skipf("skipping integration test: cannot connect to postgres: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// TestScopeNilAccountID ensures Scope rejects a nil UUID.
func TestScopeNilAccountID(t *testing.T) {
	pool := db.WrapPool(nil) // won't be called
	_, err := db.Scope(pool, uuid.Nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "accountID must not be nil")
}

// TestScopeAccountID ensures the bound account_id is returned correctly.
func TestScopeAccountID(t *testing.T) {
	pool := db.WrapPool(nil) // not used in this unit test
	id := uuid.New()
	sdb, err := db.Scope(pool, id)
	require.NoError(t, err)
	assert.Equal(t, id, sdb.AccountID())
}

// TestAppendAccountID verifies the helper appends the account_id and returns
// the correct 1-indexed position.
func TestAppendAccountID(t *testing.T) {
	pool := db.WrapPool(nil)
	id := uuid.New()
	sdb, err := db.Scope(pool, id)
	require.NoError(t, err)

	args := []any{"foo", 42}
	pos, args := sdb.AppendAccountID(args)
	assert.Equal(t, 3, pos)
	assert.Equal(t, id, args[2])
}

// TestCrossTenantIsolation is an integration test that proves a ScopedDB for
// Account A cannot read a row belonging to Account B, even when knowing the
// row's UUID.
//
// This test requires a running Postgres with the foundation schema applied
// (`make up && make migrate`).
func TestCrossTenantIsolation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	pgPool := testPool(t)
	pool := db.WrapPool(pgPool)
	ctx := context.Background()

	// Create two isolated accounts directly via SQL (bypassing service layer
	// so this test is self-contained).
	accountA := uuid.New()
	accountB := uuid.New()

	_, err := pgPool.Exec(ctx,
		`INSERT INTO accounts (id, name, plan) VALUES ($1, 'Account A', 'self_hosted'), ($2, 'Account B', 'self_hosted')`,
		accountA, accountB)
	require.NoError(t, err)

	// Insert a pipeline belonging to Account B.
	pipelineB := uuid.New()
	_, err = pgPool.Exec(ctx,
		`INSERT INTO lead_pipelines (id, account_id, name, states) VALUES ($1, $2, 'B Pipeline', '[]'::jsonb)`,
		pipelineB, accountB)
	require.NoError(t, err)

	// Attempt to read the pipeline scoped to Account A — must return no rows.
	sdbA, err := db.Scope(pool, accountA)
	require.NoError(t, err)

	args := []any{pipelineB}
	pos, args := sdbA.AppendAccountID(args)
	row := sdbA.QueryRow(ctx,
		fmt.Sprintf(`SELECT id FROM lead_pipelines WHERE id = $1 AND account_id = $%d`, pos),
		args...)

	var gotID uuid.UUID
	err = row.Scan(&gotID)
	assert.ErrorIs(t, err, pgx.ErrNoRows,
		"Account A must not be able to read Account B's pipeline row")

	// Cleanup
	t.Cleanup(func() {
		pgPool.Exec(context.Background(),
			`DELETE FROM lead_pipelines WHERE account_id IN ($1, $2)`, accountA, accountB)
		pgPool.Exec(context.Background(),
			`DELETE FROM accounts WHERE id IN ($1, $2)`, accountA, accountB)
	})
}

package handler_test

import (
	"context"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/whatfunnel/whatfunnel/packages/go-common/db"
)

type mockSessionStore struct {
	userID, accountID uuid.UUID
	role              string
	loggedIn          bool
}

func (m *mockSessionStore) GetUserID(*http.Request) (uuid.UUID, bool) {
	return m.userID, m.loggedIn
}
func (m *mockSessionStore) GetAccountID(*http.Request) (uuid.UUID, bool) {
	return m.accountID, m.loggedIn
}
func (m *mockSessionStore) GetRole(*http.Request) (string, bool) {
	return m.role, m.loggedIn
}

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
		t.Skipf("skipping integration test: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func setupTestTenant(t *testing.T, pool *pgxpool.Pool, name string) (uuid.UUID, uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	var accountID, userID uuid.UUID
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO accounts (name) VALUES ($1) RETURNING id`, name).Scan(&accountID))
	require.NoError(t, pool.QueryRow(ctx, `
		INSERT INTO users (account_id, email, password_hash, role)
		VALUES ($1, $2, 'hash', 'manager') RETURNING id
	`, accountID, name+"@example.com").Scan(&userID))
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM accounts WHERE id = $1`, accountID)
	})
	return accountID, userID
}

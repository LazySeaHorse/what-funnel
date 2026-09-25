package store_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/whatfunnel/whatfunnel/packages/go-common/db"
	"github.com/whatfunnel/whatfunnel/services/identity-svc/internal/store"
)

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
		t.Skipf("skipping store integration test: cannot connect to postgres: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestStore_Create_Load_Save(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	pool := testPool(t)
	s := store.New(pool)
	ctx := context.Background()

	// Create an account first
	accountID := uuid.New()
	slug := "test-store-acc-" + uuid.New().String()[:8]
	_, err := pool.Exec(ctx, `INSERT INTO accounts (id, name, slug) VALUES ($1, $2, $3)`, accountID, "Store Account", slug)
	require.NoError(t, err)
	t.Cleanup(func() {
		pool.Exec(context.Background(), `DELETE FROM users WHERE account_id = $1`, accountID)
		pool.Exec(context.Background(), `DELETE FROM accounts WHERE id = $1`, accountID)
	})

	email := "storeuser+" + uuid.New().String()[:8] + "@example.com"
	u := &store.User{
		AccountID:    accountID,
		Email:        email,
		Username:     "storer",
		PasswordHash: "initial-hash",
		Role:         "agent",
	}

	err = s.Create(ctx, u)
	require.NoError(t, err)

	// Test Load by PID (email)
	loaded, err := s.Load(ctx, email)
	require.NoError(t, err)
	assert.Equal(t, email, loaded.GetPID())

	loadedUser, ok := loaded.(*store.User)
	require.True(t, ok)
	assert.Equal(t, "initial-hash", loadedUser.GetPassword())
	assert.Equal(t, accountID, loadedUser.AccountID)
	assert.Equal(t, "storer", loadedUser.Username)
	assert.Equal(t, "agent", loadedUser.Role)

	// Test LoadByIdentifier with email
	loadedByEmail, err := s.LoadByIdentifier(ctx, email)
	require.NoError(t, err)
	assert.Equal(t, loadedUser.ID, loadedByEmail.ID)

	// Test LoadByIdentifier with slug-username
	identifier := slug + "-storer"
	loadedBySlug, err := s.LoadByIdentifier(ctx, identifier)
	require.NoError(t, err)
	assert.Equal(t, loadedUser.ID, loadedBySlug.ID)

	// Test Save (updates password and deletes sessions)
	loadedUser.PasswordHash = "updated-hash"
	err = s.Save(ctx, loadedUser)
	require.NoError(t, err)

	reloaded, err := s.Load(ctx, email)
	require.NoError(t, err)
	reloadedUser, ok := reloaded.(*store.User)
	require.True(t, ok)
	assert.Equal(t, "updated-hash", reloadedUser.GetPassword())
}

package store_test

import (
	"context"
	"os"
	"testing"
	"time"

	ab "github.com/aarondl/authboss/v3"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/whatfunnel/whatfunnel/packages/go-common/db"
	"github.com/whatfunnel/whatfunnel/packages/go-common/db/dbgen"
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
		if os.Getenv("CI") != "" {
			t.Fatalf("database unavailable in CI: %v", err)
		}
		t.Skipf("skipping store integration test: cannot connect to postgres: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// createUser inserts a user row for the lookup tests.
func createUser(t *testing.T, pool *pgxpool.Pool, u *store.User) {
	t.Helper()
	_, err := dbgen.New(pool).CreateUser(context.Background(), dbgen.CreateUserParams{
		AccountID:    u.AccountID,
		Email:        u.Email,
		Username:     u.Username,
		PasswordHash: u.PasswordHash,
		Role:         u.Role,
	})
	require.NoError(t, err)
}

func TestLoadByIdentifier(t *testing.T) {
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
	createUser(t, pool, &store.User{
		AccountID:    accountID,
		Email:        email,
		Username:     "storer",
		PasswordHash: "initial-hash",
		Role:         "agent",
	})

	// Email
	loaded, err := s.LoadByIdentifier(ctx, email)
	require.NoError(t, err)
	assert.Equal(t, email, loaded.Email)
	assert.Equal(t, "initial-hash", loaded.PasswordHash)
	assert.Equal(t, accountID, loaded.AccountID)
	assert.Equal(t, "storer", loaded.Username)
	assert.Equal(t, "agent", loaded.Role)
	assert.False(t, loaded.CreatedAt.IsZero())

	// slug-username (legacy hyphenated)
	loadedBySlug, err := s.LoadByIdentifier(ctx, slug+"-storer")
	require.NoError(t, err)
	assert.Equal(t, loaded, loadedBySlug)

	// slug/username (unambiguous slash)
	loadedBySlash, err := s.LoadByIdentifier(ctx, slug+"/storer")
	require.NoError(t, err)
	assert.Equal(t, loaded, loadedBySlash)

	// Unknown identifiers
	_, err = s.LoadByIdentifier(ctx, "nobody-"+email)
	assert.ErrorIs(t, err, ab.ErrUserNotFound)
}

func TestLoadByIdentifier_CrossTenantCollisionPrevention(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	ctx := context.Background()
	pool := testPool(t)
	s := store.New(pool)

	suffix := uuid.New().String()[:6]
	slug1 := "col-corp-" + suffix

	// Account 1: slug = "col-corp-xxx", username = "admin"
	// Combined legacy = "col-corp-xxx-admin"
	var accountID1 uuid.UUID
	err := pool.QueryRow(ctx, `
		INSERT INTO accounts (name, plan, product_mode, slug)
		VALUES ('Tenant 1', 'self_hosted', 'full_workspace', $1)
		RETURNING id
	`, slug1).Scan(&accountID1)
	require.NoError(t, err)

	user1 := &store.User{
		AccountID:    accountID1,
		Email:        "u1-" + suffix + "@example.com",
		Username:     "admin",
		PasswordHash: "hash-1",
		Role:         "manager",
	}
	createUser(t, pool, user1)

	t.Cleanup(func() {
		pool.Exec(ctx, `DELETE FROM users WHERE account_id = $1`, accountID1)
		pool.Exec(ctx, `DELETE FROM accounts WHERE id = $1`, accountID1)
	})

	// Before collision exists: single tenant legacy hyphenated lookup succeeds
	legacyIdent := slug1 + "-admin"
	loadedSingle, err := s.LoadByIdentifier(ctx, legacyIdent)
	require.NoError(t, err)
	assert.Equal(t, accountID1, loadedSingle.AccountID)
	assert.Equal(t, "admin", loadedSingle.Username)

	// Account 2: slug = "col-xxx", username = "corp-admin"
	// Combined legacy = "col-xxx-corp-admin" -> wait, to collide:
	// If slug1 is "col-corp" and user1 is "admin", combined is "col-corp-admin"
	// If slug2 is "col" and user2 is "corp-admin", combined is "col-corp-admin"!
	// Let's create exact collision:
	slugA := "t1-" + suffix + "-sub"
	userAname := "mgr"
	// combined = t1-<suffix>-sub-mgr

	slugB := "t1-" + suffix
	userBname := "sub-mgr"
	// combined = t1-<suffix>-sub-mgr! Identical!

	var accountIDA, accountIDB uuid.UUID
	err = pool.QueryRow(ctx, `
		INSERT INTO accounts (name, plan, product_mode, slug)
		VALUES ('Tenant A', 'self_hosted', 'full_workspace', $1)
		RETURNING id
	`, slugA).Scan(&accountIDA)
	require.NoError(t, err)

	err = pool.QueryRow(ctx, `
		INSERT INTO accounts (name, plan, product_mode, slug)
		VALUES ('Tenant B', 'self_hosted', 'full_workspace', $1)
		RETURNING id
	`, slugB).Scan(&accountIDB)
	require.NoError(t, err)

	t.Cleanup(func() {
		pool.Exec(ctx, `DELETE FROM users WHERE account_id IN ($1, $2)`, accountIDA, accountIDB)
		pool.Exec(ctx, `DELETE FROM accounts WHERE id IN ($1, $2)`, accountIDA, accountIDB)
	})

	userA := &store.User{
		AccountID:    accountIDA,
		Email:        "ua-" + suffix + "@example.com",
		Username:     userAname,
		PasswordHash: "hash-a",
		Role:         "manager",
	}
	createUser(t, pool, userA)

	userB := &store.User{
		AccountID:    accountIDB,
		Email:        "ub-" + suffix + "@example.com",
		Username:     userBname,
		PasswordHash: "hash-b",
		Role:         "agent",
	}
	createUser(t, pool, userB)

	collidingLegacy := slugA + "-" + userAname
	assert.Equal(t, collidingLegacy, slugB+"-"+userBname)

	// 1. Unambiguous slash lookups MUST cleanly isolate each tenant's user
	loadedA, err := s.LoadByIdentifier(ctx, slugA+"/"+userAname)
	require.NoError(t, err)
	assert.Equal(t, accountIDA, loadedA.AccountID)
	assert.Equal(t, userAname, loadedA.Username)

	loadedB, err := s.LoadByIdentifier(ctx, slugB+"/"+userBname)
	require.NoError(t, err)
	assert.Equal(t, accountIDB, loadedB.AccountID)
	assert.Equal(t, userBname, loadedB.Username)
	assert.NotEqual(t, loadedA.ID, loadedB.ID)
	assert.NotEqual(t, loadedA.AccountID, loadedB.AccountID)

	// 2. Ambiguous legacy hyphenated lookup MUST be rejected to prevent cross-tenant takeover/collision
	_, err = s.LoadByIdentifier(ctx, collidingLegacy)
	assert.ErrorIs(t, err, ab.ErrUserNotFound)

	// 3. Malformed slash identifiers return ErrUserNotFound
	_, err = s.LoadByIdentifier(ctx, "/"+userAname)
	assert.ErrorIs(t, err, ab.ErrUserNotFound)

	_, err = s.LoadByIdentifier(ctx, slugA+"/")
	assert.ErrorIs(t, err, ab.ErrUserNotFound)
}

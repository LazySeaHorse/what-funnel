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

	// Test LoadByIdentifier with slug-username (legacy hyphenated)
	identifier := slug + "-storer"
	loadedBySlug, err := s.LoadByIdentifier(ctx, identifier)
	require.NoError(t, err)
	assert.Equal(t, loadedUser.ID, loadedBySlug.ID)

	// Test LoadByIdentifier with slug/username (unambiguous slash)
	slashIdentifier := slug + "/storer"
	loadedBySlash, err := s.LoadByIdentifier(ctx, slashIdentifier)
	require.NoError(t, err)
	assert.Equal(t, loadedUser.ID, loadedBySlash.ID)

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
	err = s.Create(ctx, user1)
	require.NoError(t, err)

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
	err = s.Create(ctx, userA)
	require.NoError(t, err)

	userB := &store.User{
		AccountID:    accountIDB,
		Email:        "ub-" + suffix + "@example.com",
		Username:     userBname,
		PasswordHash: "hash-b",
		Role:         "agent",
	}
	err = s.Create(ctx, userB)
	require.NoError(t, err)

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

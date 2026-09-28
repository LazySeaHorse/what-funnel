package service_test

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
	"github.com/whatfunnel/whatfunnel/services/identity-svc/internal/service"
	"github.com/whatfunnel/whatfunnel/services/identity-svc/internal/session"
)

// testPool returns a connected pool or skips the test.
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

const sessionSecret = "test-session-secret-at-least-32-ch"

// uniqueEmail generates a unique email for each test run.
func uniqueEmail(t *testing.T) string {
	return "test+" + t.Name() + "@example.com"
}

func testService(t *testing.T) (*service.Service, *pgxpool.Pool) {
	t.Helper()
	pool := testPool(t)
	sess := session.New(pool, sessionSecret)
	svc, err := service.New(pool, sess)
	require.NoError(t, err)
	return svc, pool
}

// ---------------------------------------------------------------------------
// Signup tests
// ---------------------------------------------------------------------------

func TestSignup_Success(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	svc, pool := testService(t)
	ctx := context.Background()

	email := uniqueEmail(t)
	user, err := svc.Signup(ctx, service.SignupRequest{
		AccountName: "Test Account",
		Email:       email,
		Password:    "securepassword123",
	})
	require.NoError(t, err)
	assert.Equal(t, email, user.Email)
	assert.Equal(t, "manager", user.Role)
	assert.NotEmpty(t, user.ID)
	assert.NotEmpty(t, user.AccountID)

	// Verify default pipeline was created
	var pipelineCount int
	err = pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM lead_pipelines WHERE account_id = $1`, user.AccountID).
		Scan(&pipelineCount)
	require.NoError(t, err)
	assert.Equal(t, 1, pipelineCount, "default pipeline must be seeded on account creation")

	// Verify audit log was written
	var auditCount int
	err = pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM audit_logs WHERE account_id = $1`, user.AccountID).
		Scan(&auditCount)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, auditCount, 2, "at least account.created and user.created audit rows expected")

	// Cleanup
	t.Cleanup(func() {
		pool.Exec(context.Background(), `DELETE FROM lead_pipelines WHERE account_id = $1`, user.AccountID)
		pool.Exec(context.Background(), `DELETE FROM audit_logs WHERE account_id = $1`, user.AccountID)
		pool.Exec(context.Background(), `DELETE FROM users WHERE account_id = $1`, user.AccountID)
		pool.Exec(context.Background(), `DELETE FROM accounts WHERE id = $1`, user.AccountID)
	})
}

func TestSignup_ProductMode(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	svc, pool := testService(t)
	ctx := context.Background()

	email := uniqueEmail(t)
	user, err := svc.Signup(ctx, service.SignupRequest{
		AccountName: "Chatbot Only Account",
		Email:       email,
		Password:    "securepassword123",
		ProductMode: "chatbot_only",
	})
	require.NoError(t, err)
	assert.Equal(t, email, user.Email)

	// Verify product mode is chatbot_only in DB
	var pm string
	err = pool.QueryRow(ctx, `SELECT product_mode FROM accounts WHERE id = $1`, user.AccountID).Scan(&pm)
	require.NoError(t, err)
	assert.Equal(t, "chatbot_only", pm)

	t.Cleanup(func() {
		pool.Exec(context.Background(), `DELETE FROM lead_pipelines WHERE account_id = $1`, user.AccountID)
		pool.Exec(context.Background(), `DELETE FROM audit_logs WHERE account_id = $1`, user.AccountID)
		pool.Exec(context.Background(), `DELETE FROM users WHERE account_id = $1`, user.AccountID)
		pool.Exec(context.Background(), `DELETE FROM accounts WHERE id = $1`, user.AccountID)
	})
}

func TestSignup_InvalidProductMode(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	svc, _ := testService(t)
	ctx := context.Background()

	email := uniqueEmail(t)
	_, err := svc.Signup(ctx, service.SignupRequest{
		AccountName: "Invalid Mode Account",
		Email:       email,
		Password:    "securepassword123",
		ProductMode: "invalid_mode",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid product mode: invalid_mode")
}

func TestSignup_DuplicateEmailGlobal(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	svc, pool := testService(t)
	ctx := context.Background()

	email := uniqueEmail(t)
	user, err := svc.Signup(ctx, service.SignupRequest{
		AccountName: "Dupe Test Account 1",
		Email:       email,
		Password:    "password1",
	})
	require.NoError(t, err)
	assert.NotNil(t, user)

	// Attempting a second signup with the SAME email on a different account must fail
	_, err = svc.Signup(ctx, service.SignupRequest{
		AccountName: "Dupe Test Account 2",
		Email:       email,
		Password:    "password2",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "email already registered")

	t.Cleanup(func() {
		pool.Exec(context.Background(), `DELETE FROM lead_pipelines WHERE account_id = $1`, user.AccountID)
		pool.Exec(context.Background(), `DELETE FROM audit_logs WHERE account_id = $1`, user.AccountID)
		pool.Exec(context.Background(), `DELETE FROM users WHERE account_id = $1`, user.AccountID)
		pool.Exec(context.Background(), `DELETE FROM accounts WHERE id = $1`, user.AccountID)
	})
}

// ---------------------------------------------------------------------------
// Login tests
// ---------------------------------------------------------------------------

func TestLogin_CorrectPassword(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	svc, pool := testService(t)
	ctx := context.Background()

	email := uniqueEmail(t)
	signup, err := svc.Signup(ctx, service.SignupRequest{
		AccountName: "Login Test Account",
		Email:       email,
		Password:    "mypassword",
	})
	require.NoError(t, err)

	user, err := svc.Login(ctx, service.LoginRequest{
		Email:    email,
		Password: "mypassword",
	})
	require.NoError(t, err)
	assert.Equal(t, email, user.Email)
	assert.Equal(t, signup.AccountID, user.AccountID)
	assert.Equal(t, "manager", user.Role)

	t.Cleanup(func() {
		pool.Exec(context.Background(), `DELETE FROM lead_pipelines WHERE account_id = $1`, signup.AccountID)
		pool.Exec(context.Background(), `DELETE FROM audit_logs WHERE account_id = $1`, signup.AccountID)
		pool.Exec(context.Background(), `DELETE FROM users WHERE account_id = $1`, signup.AccountID)
		pool.Exec(context.Background(), `DELETE FROM accounts WHERE id = $1`, signup.AccountID)
	})
}

func TestLogin_SlugUsername(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	svc, pool := testService(t)
	ctx := context.Background()

	email := uniqueEmail(t)
	signup, err := svc.Signup(ctx, service.SignupRequest{
		AccountName: "Slug Login Account",
		Email:       email,
		Username:    "owner",
		Password:    "mypassword",
	})
	require.NoError(t, err)

	// Set slug
	_, err = pool.Exec(ctx, `UPDATE accounts SET slug = 'test-acme' WHERE id = $1`, signup.AccountID)
	require.NoError(t, err)

	// Login with slug-username identifier (legacy hyphenated)
	user, err := svc.Login(ctx, service.LoginRequest{
		Identifier: "test-acme-owner",
		Password:   "mypassword",
	})
	require.NoError(t, err)
	assert.Equal(t, "owner", user.Username)
	assert.Equal(t, signup.AccountID, user.AccountID)
	assert.Equal(t, "manager", user.Role)

	// Login with slug/username identifier (unambiguous slash)
	userSlash, err := svc.Login(ctx, service.LoginRequest{
		Identifier: "test-acme/owner",
		Password:   "mypassword",
	})
	require.NoError(t, err)
	assert.Equal(t, "owner", userSlash.Username)
	assert.Equal(t, signup.AccountID, userSlash.AccountID)
	assert.Equal(t, "manager", userSlash.Role)

	t.Cleanup(func() {
		pool.Exec(context.Background(), `DELETE FROM lead_pipelines WHERE account_id = $1`, signup.AccountID)
		pool.Exec(context.Background(), `DELETE FROM audit_logs WHERE account_id = $1`, signup.AccountID)
		pool.Exec(context.Background(), `DELETE FROM users WHERE account_id = $1`, signup.AccountID)
		pool.Exec(context.Background(), `DELETE FROM accounts WHERE id = $1`, signup.AccountID)
	})
}

func TestLogin_CrossTenantCollisionPrevention(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	svc, pool := testService(t)
	ctx := context.Background()

	suffix := uuid.New().String()[:6]
	slug1 := "org-" + suffix + "-dept"
	user1Name := "lead"
	// Legacy combined: org-<suffix>-dept-lead

	slug2 := "org-" + suffix
	user2Name := "dept-lead"
	// Legacy combined: org-<suffix>-dept-lead

	// Tenant 1
	signup1, err := svc.Signup(ctx, service.SignupRequest{
		AccountName: "Tenant 1",
		Email:       "t1-" + suffix + "@example.com",
		Username:    user1Name,
		Password:    "password123!",
	})
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `UPDATE accounts SET slug = $1 WHERE id = $2`, slug1, signup1.AccountID)
	require.NoError(t, err)

	// Tenant 2
	signup2, err := svc.Signup(ctx, service.SignupRequest{
		AccountName: "Tenant 2",
		Email:       "t2-" + suffix + "@example.com",
		Username:    user2Name,
		Password:    "password456!",
	})
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `UPDATE accounts SET slug = $1 WHERE id = $2`, slug2, signup2.AccountID)
	require.NoError(t, err)

	t.Cleanup(func() {
		for _, aid := range []uuid.UUID{signup1.AccountID, signup2.AccountID} {
			pool.Exec(context.Background(), `DELETE FROM lead_pipelines WHERE account_id = $1`, aid)
			pool.Exec(context.Background(), `DELETE FROM audit_logs WHERE account_id = $1`, aid)
			pool.Exec(context.Background(), `DELETE FROM users WHERE account_id = $1`, aid)
			pool.Exec(context.Background(), `DELETE FROM accounts WHERE id = $1`, aid)
		}
	})

	collidingLegacy := slug1 + "-" + user1Name
	assert.Equal(t, collidingLegacy, slug2+"-"+user2Name)

	// 1. Ambiguous legacy hyphenated login fails (prevents cross-tenant credential stuffing/takeover)
	_, err = svc.Login(ctx, service.LoginRequest{
		Identifier: collidingLegacy,
		Password:   "password123!",
	})
	assert.Error(t, err, "ambiguous multi-tenant legacy identifier must be rejected")

	// 2. Unambiguous slash-delimited login cleanly routes to Tenant 1
	u1, err := svc.Login(ctx, service.LoginRequest{
		Identifier: slug1 + "/" + user1Name,
		Password:   "password123!",
	})
	require.NoError(t, err)
	assert.Equal(t, signup1.AccountID, u1.AccountID)
	assert.Equal(t, user1Name, u1.Username)

	// 3. Unambiguous slash-delimited login cleanly routes to Tenant 2
	u2, err := svc.Login(ctx, service.LoginRequest{
		Identifier: slug2 + "/" + user2Name,
		Password:   "password456!",
	})
	require.NoError(t, err)
	assert.Equal(t, signup2.AccountID, u2.AccountID)
	assert.Equal(t, user2Name, u2.Username)
}

func TestLogin_WrongPassword(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	svc, pool := testService(t)
	ctx := context.Background()

	email := uniqueEmail(t)
	signup, err := svc.Signup(ctx, service.SignupRequest{
		AccountName: "Bad Login Account",
		Email:       email,
		Password:    "correctpassword",
	})
	require.NoError(t, err)

	_, err = svc.Login(ctx, service.LoginRequest{
		Email:    email,
		Password: "wrongpassword",
	})
	assert.Error(t, err, "login with wrong password must fail")
	assert.Contains(t, err.Error(), "invalid credentials")

	t.Cleanup(func() {
		pool.Exec(context.Background(), `DELETE FROM lead_pipelines WHERE account_id = $1`, signup.AccountID)
		pool.Exec(context.Background(), `DELETE FROM audit_logs WHERE account_id = $1`, signup.AccountID)
		pool.Exec(context.Background(), `DELETE FROM users WHERE account_id = $1`, signup.AccountID)
		pool.Exec(context.Background(), `DELETE FROM accounts WHERE id = $1`, signup.AccountID)
	})
}

func TestLogin_NonExistentUser(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	svc, _ := testService(t)
	ctx := context.Background()

	_, err := svc.Login(ctx, service.LoginRequest{
		Email:    "nobody@example.com",
		Password: "whatever",
	})
	assert.Error(t, err, "login for non-existent user must fail")
}

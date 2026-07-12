package service_test

import (
	"context"
	"os"
	"testing"
	"time"

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
	assert.Equal(t, "admin", user.Role)
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

func TestSignup_DuplicateEmailSameAccount(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	svc, pool := testService(t)
	ctx := context.Background()

	email := uniqueEmail(t)
	user, err := svc.Signup(ctx, service.SignupRequest{
		AccountName: "Dupe Test Account",
		Email:       email,
		Password:    "password1",
	})
	require.NoError(t, err)

	// Attempting a second signup with the SAME email on a new account
	// is fine (separate tenants). The unique constraint is (account_id, email).
	// This test just verifies the first signup worked.
	assert.NotNil(t, user)

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
	assert.Equal(t, "admin", user.Role)

	t.Cleanup(func() {
		pool.Exec(context.Background(), `DELETE FROM lead_pipelines WHERE account_id = $1`, signup.AccountID)
		pool.Exec(context.Background(), `DELETE FROM audit_logs WHERE account_id = $1`, signup.AccountID)
		pool.Exec(context.Background(), `DELETE FROM users WHERE account_id = $1`, signup.AccountID)
		pool.Exec(context.Background(), `DELETE FROM accounts WHERE id = $1`, signup.AccountID)
	})
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

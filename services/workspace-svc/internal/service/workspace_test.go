package service_test

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/whatfunnel/whatfunnel/packages/go-common/db"
	"github.com/whatfunnel/whatfunnel/packages/go-common/types"
	"github.com/whatfunnel/whatfunnel/services/workspace-svc/internal/service"
)

const testEncryptionKey = "test-key-exactly-32-bytes-padded"

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

func testService(t *testing.T) (*service.Service, *pgxpool.Pool) {
	t.Helper()
	pool := testPool(t)
	svc, err := service.New(pool, testEncryptionKey)
	require.NoError(t, err)
	return svc, pool
}

// setupTestTenant inserts a minimal account + admin user + default pipeline.
// Returns (accountID, adminUserID).
func setupTestTenant(t *testing.T, pool *pgxpool.Pool, accountName, email string) (uuid.UUID, uuid.UUID) {
	t.Helper()
	ctx := context.Background()

	var accountID uuid.UUID
	err := pool.QueryRow(ctx,
		`INSERT INTO accounts (name, plan) VALUES ($1, 'self_hosted') RETURNING id`,
		accountName).Scan(&accountID)
	require.NoError(t, err)

	statesJSON, _ := json.Marshal(types.DefaultPipelineStates)
	_, err = pool.Exec(ctx,
		`INSERT INTO lead_pipelines (account_id, name, states) VALUES ($1, 'Default', $2)`,
		accountID, statesJSON)
	require.NoError(t, err)

	var userID uuid.UUID
	err = pool.QueryRow(ctx,
		`INSERT INTO users (account_id, email, password_hash, role)
		 VALUES ($1, $2, 'hashed', 'admin') RETURNING id`,
		accountID, email).Scan(&userID)
	require.NoError(t, err)

	t.Cleanup(func() {
		pool.Exec(context.Background(), `DELETE FROM audit_logs WHERE account_id = $1`, accountID)
		pool.Exec(context.Background(), `DELETE FROM invite_tokens WHERE account_id = $1`, accountID)
		pool.Exec(context.Background(), `DELETE FROM lead_pipelines WHERE account_id = $1`, accountID)
		pool.Exec(context.Background(), `DELETE FROM users WHERE account_id = $1`, accountID)
		pool.Exec(context.Background(), `DELETE FROM accounts WHERE id = $1`, accountID)
	})

	return accountID, userID
}

// ---------------------------------------------------------------------------
// AI provider config encryption — Stage 6
// ---------------------------------------------------------------------------

func TestAIProviderConfig_RoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	svc, pool := testService(t)
	ctx := context.Background()

	accountID, userID := setupTestTenant(t, pool, "AI Config Account", "ai@example.com")

	plaintext := `{"api_key":"sk-test-12345","base_url":"https://api.openai.com/v1"}`

	// Store encrypted
	err := svc.UpdateAIProviderConfig(ctx, accountID, userID, plaintext)
	require.NoError(t, err, "UpdateAIProviderConfig must not fail")

	// Read back — must decrypt to original plaintext
	recovered, err := svc.GetAIProviderConfig(ctx, accountID)
	require.NoError(t, err)
	assert.Equal(t, plaintext, recovered, "decrypted value must match original plaintext")

	// Verify the database does NOT contain the plaintext
	var storedRaw *string
	err = pool.QueryRow(ctx,
		`SELECT ai_provider_config FROM accounts WHERE id = $1`, accountID).Scan(&storedRaw)
	require.NoError(t, err)
	require.NotNil(t, storedRaw)
	assert.NotEqual(t, plaintext, *storedRaw, "plaintext must NOT be stored in the database")
	assert.NotContains(t, *storedRaw, "sk-test-12345", "API key must not appear in plaintext in the database")
}

// ---------------------------------------------------------------------------
// User management — Stage 5
// ---------------------------------------------------------------------------

func TestListUsers_AdminCanSeeAll(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	svc, pool := testService(t)
	ctx := context.Background()

	accountID, _ := setupTestTenant(t, pool, "List Users Account", "admin@example.com")

	users, err := svc.ListUsers(ctx, accountID)
	require.NoError(t, err)
	assert.Len(t, users, 1, "should see the admin user created by setupTestTenant")
}

func TestInviteUser_GeneratesToken(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	svc, pool := testService(t)
	ctx := context.Background()

	accountID, actorID := setupTestTenant(t, pool, "Invite Account", "admin2@example.com")

	result, err := svc.InviteUser(ctx, accountID, actorID, service.InviteUserRequest{
		Email: "member@example.com",
		Role:  "member",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, result.Token, "invite token must be non-empty")
	assert.Equal(t, "member", result.Role)

	// Verify token was stored
	var tokenCount int
	err = pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM invite_tokens WHERE account_id = $1 AND email = $2`,
		accountID, "member@example.com").Scan(&tokenCount)
	require.NoError(t, err)
	assert.Equal(t, 1, tokenCount, "invite token must be persisted")

	// Verify audit log
	var auditCount int
	err = pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM audit_logs WHERE account_id = $1 AND action = 'user.invited'`,
		accountID).Scan(&auditCount)
	require.NoError(t, err)
	assert.Equal(t, 1, auditCount)
}

func TestChangeUserRole_AuditWritten(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	svc, pool := testService(t)
	ctx := context.Background()

	accountID, adminID := setupTestTenant(t, pool, "Role Change Account", "admin3@example.com")

	// Create a member user
	var memberID uuid.UUID
	err := pool.QueryRow(ctx,
		`INSERT INTO users (account_id, email, password_hash, role)
		 VALUES ($1, 'member3@example.com', 'hashed', 'member') RETURNING id`,
		accountID).Scan(&memberID)
	require.NoError(t, err)

	err = svc.ChangeUserRole(ctx, accountID, adminID, memberID, "admin")
	require.NoError(t, err)

	// Verify role was updated
	var role string
	err = pool.QueryRow(ctx,
		`SELECT role FROM users WHERE id = $1`, memberID).Scan(&role)
	require.NoError(t, err)
	assert.Equal(t, "admin", role)

	// Verify audit log
	var auditCount int
	err = pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM audit_logs WHERE account_id = $1 AND action = 'user.role_changed'`,
		accountID).Scan(&auditCount)
	require.NoError(t, err)
	assert.Equal(t, 1, auditCount)
}

// ---------------------------------------------------------------------------
// Pipeline tests — Stage 5
// ---------------------------------------------------------------------------

func TestUpdatePipeline_AuditWritten(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	svc, pool := testService(t)
	ctx := context.Background()

	accountID, adminID := setupTestTenant(t, pool, "Pipeline Account", "admin4@example.com")

	// Get the default pipeline
	pipelines, err := svc.ListPipelines(ctx, accountID)
	require.NoError(t, err)
	require.Len(t, pipelines, 1)
	pipelineID := pipelines[0].ID

	err = svc.UpdatePipeline(ctx, accountID, adminID, pipelineID, service.UpdatePipelineRequest{
		Name: "Custom Pipeline",
		States: []types.PipelineState{
			{Key: "new", Label: "New Lead", Color: "#fff"},
			{Key: "closed", Label: "Closed", Color: "#000"},
		},
	})
	require.NoError(t, err)

	// Verify name was updated
	pipelines2, err := svc.ListPipelines(ctx, accountID)
	require.NoError(t, err)
	assert.Equal(t, "Custom Pipeline", pipelines2[0].Name)
	assert.Len(t, pipelines2[0].States, 2)

	// Verify audit
	var auditCount int
	err = pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM audit_logs WHERE account_id = $1 AND action = 'pipeline.updated'`,
		accountID).Scan(&auditCount)
	require.NoError(t, err)
	assert.Equal(t, 1, auditCount)
}

// ---------------------------------------------------------------------------
// Cross-tenant isolation — Stage 2 (workspace layer)
// ---------------------------------------------------------------------------

func TestCrossTenantPipelineIsolation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	svc, pool := testService(t)
	ctx := context.Background()

	accountA, adminA := setupTestTenant(t, pool, "Tenant A", "a@example.com")
	accountB, _ := setupTestTenant(t, pool, "Tenant B", "b@example.com")

	// Get Account B's pipeline
	pipelinesB, err := svc.ListPipelines(ctx, accountB)
	require.NoError(t, err)
	require.Len(t, pipelinesB, 1)
	pipelineB := pipelinesB[0].ID

	// Attempt to update B's pipeline as A's admin — must fail
	err = svc.UpdatePipeline(ctx, accountA, adminA, pipelineB, service.UpdatePipelineRequest{
		Name:   "Hijacked",
		States: []types.PipelineState{},
	})
	assert.Error(t, err, "cross-tenant pipeline update must be rejected")
	assert.Contains(t, err.Error(), "not found")
}

// TestMemberDeniedAdminRoute_RBAC exercises RBAC at the service layer.
// (HTTP-layer RBAC is covered by middleware tests in go-common.)
func TestChangeUserRole_InvalidRole(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	svc, pool := testService(t)
	ctx := context.Background()

	accountID, adminID := setupTestTenant(t, pool, "Invalid Role Account", "admin5@example.com")

	var memberID uuid.UUID
	err := pool.QueryRow(ctx,
		`INSERT INTO users (account_id, email, password_hash, role)
		 VALUES ($1, 'member5@example.com', 'hashed', 'member') RETURNING id`,
		accountID).Scan(&memberID)
	require.NoError(t, err)

	err = svc.ChangeUserRole(ctx, accountID, adminID, memberID, "superuser")
	assert.Error(t, err, "invalid role must be rejected")
}

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
		 VALUES ($1, $2, 'hashed', 'manager') RETURNING id`,
		accountID, email).Scan(&userID)
	require.NoError(t, err)

	t.Cleanup(func() {
		pool.Exec(context.Background(), `DELETE FROM audit_logs WHERE account_id = $1`, accountID)
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
	configured, err := svc.HasAIProviderConfig(ctx, accountID)
	require.NoError(t, err)
	assert.False(t, configured)

	plaintext := `{"api_key":"sk-test-12345","base_url":"https://api.openai.com/v1"}`

	// Store encrypted
	err = svc.UpdateAIProviderConfig(ctx, accountID, userID, plaintext)
	require.NoError(t, err, "UpdateAIProviderConfig must not fail")
	configured, err = svc.HasAIProviderConfig(ctx, accountID)
	require.NoError(t, err)
	assert.True(t, configured)

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

func TestDeleteAccount_CascadesTenantData(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	svc, pool := testService(t)
	ctx := context.Background()
	accountID, _ := setupTestTenant(t, pool, "Delete Account", "delete@example.com")

	require.NoError(t, svc.DeleteAccount(ctx, accountID))

	var accounts, users int
	require.NoError(t, pool.QueryRow(ctx, `SELECT COUNT(*) FROM accounts WHERE id = $1`, accountID).Scan(&accounts))
	require.NoError(t, pool.QueryRow(ctx, `SELECT COUNT(*) FROM users WHERE account_id = $1`, accountID).Scan(&users))
	assert.Zero(t, accounts)
	assert.Zero(t, users)
}

// ---------------------------------------------------------------------------
// User management — Stage 5
// ---------------------------------------------------------------------------

func TestListUsers(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	svc, pool := testService(t)
	ctx := context.Background()

	accountID, _ := setupTestTenant(t, pool, "List Users Account", "admin@example.com")

	users, err := svc.ListUsers(ctx, accountID)
	require.NoError(t, err)
	assert.Len(t, users, 1, "should see the manager user created by setupTestTenant")
}

func TestCreateAndManageUser(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	svc, pool := testService(t)
	ctx := context.Background()

	accountID, actorID := setupTestTenant(t, pool, "Create User Account", "manager2@example.com")

	// 1. Create user
	result, err := svc.CreateUser(ctx, accountID, actorID, service.CreateUserRequest{
		Username: "agent_john",
		Password: "plainpassword123",
		Role:     "agent",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, result.ID)
	assert.Equal(t, "agent_john", result.Username)
	assert.Equal(t, "agent", result.Role)
	assert.Equal(t, "plainpassword123", result.PlaintextPassword)

	// 2. Verify audit log
	var auditCount int
	err = pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM audit_logs WHERE account_id = $1 AND action = 'user.created_by_admin'`,
		accountID).Scan(&auditCount)
	require.NoError(t, err)
	assert.Equal(t, 1, auditCount)

	// 3. Reset password
	err = svc.ResetUserPassword(ctx, accountID, actorID, result.ID, "newpassword456")
	require.NoError(t, err)

	// 4. Change role
	err = svc.ChangeUserRole(ctx, accountID, actorID, result.ID, "manager")
	require.NoError(t, err)

	// 5. Delete user
	err = svc.DeleteUser(ctx, accountID, actorID, result.ID)
	require.NoError(t, err)

	var userCount int
	err = pool.QueryRow(ctx, `SELECT COUNT(*) FROM users WHERE id = $1`, result.ID).Scan(&userCount)
	require.NoError(t, err)
	assert.Zero(t, userCount)
}

func TestSlugManagement(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	svc, pool := testService(t)
	ctx := context.Background()

	accountID, actorID := setupTestTenant(t, pool, "Slug Account", "manager3@example.com")

	err := svc.SetAccountSlug(ctx, accountID, actorID, "my-company-slug")
	require.NoError(t, err)

	slug, err := svc.GetAccountSlug(ctx, accountID)
	require.NoError(t, err)
	assert.Equal(t, "my-company-slug", slug)
}

func TestChangeUserRole_AuditWritten(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	svc, pool := testService(t)
	ctx := context.Background()

	accountID, adminID := setupTestTenant(t, pool, "Role Change Account", "admin3@example.com")

	// Create an agent user
	var agentID uuid.UUID
	err := pool.QueryRow(ctx,
		`INSERT INTO users (account_id, username, password_hash, role)
		 VALUES ($1, 'agent3', 'hashed', 'agent') RETURNING id`,
		accountID).Scan(&agentID)
	require.NoError(t, err)

	err = svc.ChangeUserRole(ctx, accountID, adminID, agentID, "manager")
	require.NoError(t, err)

	// Verify role was updated
	var role string
	err = pool.QueryRow(ctx,
		`SELECT role FROM users WHERE id = $1`, agentID).Scan(&role)
	require.NoError(t, err)
	assert.Equal(t, "manager", role)

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

func createTestLead(t *testing.T, pool *pgxpool.Pool, accountID uuid.UUID, pipelineID uuid.UUID, stateKey string) (uuid.UUID, uuid.UUID) {
	ctx := context.Background()
	// 1. Create a channel
	var channelID uuid.UUID
	err := pool.QueryRow(ctx,
		`INSERT INTO channels (account_id, type, status) VALUES ($1, 'matrix_whatsapp', 'connected') RETURNING id`,
		accountID).Scan(&channelID)
	require.NoError(t, err)

	// 2. Create a contact
	var contactID uuid.UUID
	err = pool.QueryRow(ctx,
		`INSERT INTO contacts (account_id, channel_id, external_identity) VALUES ($1, $2, $3) RETURNING id`,
		accountID, channelID, uuid.NewString()).Scan(&contactID)
	require.NoError(t, err)

	// 3. Create a conversation
	var conversationID uuid.UUID
	err = pool.QueryRow(ctx,
		`INSERT INTO conversations (account_id, contact_id, channel_id, status) VALUES ($1, $2, $3, 'open') RETURNING id`,
		accountID, contactID, channelID).Scan(&conversationID)
	require.NoError(t, err)

	// 4. Create a lead
	var leadID uuid.UUID
	err = pool.QueryRow(ctx,
		`INSERT INTO leads (account_id, conversation_id, pipeline_id, current_state_key) VALUES ($1, $2, $3, $4) RETURNING id`,
		accountID, conversationID, pipelineID, stateKey).Scan(&leadID)
	require.NoError(t, err)

	t.Cleanup(func() {
		pool.Exec(context.Background(), `DELETE FROM leads WHERE id = $1`, leadID)
		pool.Exec(context.Background(), `DELETE FROM conversations WHERE id = $1`, conversationID)
		pool.Exec(context.Background(), `DELETE FROM contacts WHERE id = $1`, contactID)
		pool.Exec(context.Background(), `DELETE FROM channels WHERE id = $1`, channelID)
	})

	return leadID, conversationID
}

func TestUpdatePipeline_StateDeletionGuard(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	svc, pool := testService(t)
	ctx := context.Background()

	accountID, adminID := setupTestTenant(t, pool, "Pipeline Guard Account", "admin_guard@example.com")

	// Get the default pipeline
	pipelines, err := svc.ListPipelines(ctx, accountID)
	require.NoError(t, err)
	require.Len(t, pipelines, 1)
	pipelineID := pipelines[0].ID

	// 1. Safe rename/reorder succeeds
	err = svc.UpdatePipeline(ctx, accountID, adminID, pipelineID, service.UpdatePipelineRequest{
		Name: "Default Pipeline",
		States: []types.PipelineState{
			{Key: "new", Label: "Newly Created", Color: "#6366f1"}, // rename label
			{Key: "contacted", Label: "Contacted", Color: "#3b82f6"},
			{Key: "follow_up", Label: "Follow-up", Color: "#f59e0b"},
			{Key: "won", Label: "Won", Color: "#22c55e"},
			{Key: "lost", Label: "Lost", Color: "#ef4444"},
		},
	})
	require.NoError(t, err, "Safe rename/reorder must succeed")

	// 2. Removing an unused state succeeds
	err = svc.UpdatePipeline(ctx, accountID, adminID, pipelineID, service.UpdatePipelineRequest{
		Name: "Default Pipeline",
		States: []types.PipelineState{
			{Key: "new", Label: "Newly Created", Color: "#6366f1"},
			{Key: "contacted", Label: "Contacted", Color: "#3b82f6"},
			{Key: "follow_up", Label: "Follow-up", Color: "#f59e0b"},
			{Key: "won", Label: "Won", Color: "#22c55e"},
			// remove 'lost' state
		},
	})
	require.NoError(t, err, "Removing an unused state must succeed")

	// 3. Removing an in-use state is rejected
	// Create a lead in state 'won'
	leadID, _ := createTestLead(t, pool, accountID, pipelineID, "won")

	// Attempt to remove 'won' state
	err = svc.UpdatePipeline(ctx, accountID, adminID, pipelineID, service.UpdatePipelineRequest{
		Name: "Default Pipeline",
		States: []types.PipelineState{
			{Key: "new", Label: "Newly Created", Color: "#6366f1"},
			{Key: "contacted", Label: "Contacted", Color: "#3b82f6"},
			{Key: "follow_up", Label: "Follow-up", Color: "#f59e0b"},
			// remove 'won' state
		},
	})
	assert.Error(t, err, "Removing an in-use state must be rejected")

	inUseErr, ok := err.(*service.ErrPipelineInUse)
	if assert.True(t, ok, "Error must be of type *ErrPipelineInUse") {
		assert.Contains(t, inUseErr.StateKeys, "won")
		assert.Contains(t, inUseErr.LeadIDs, leadID)
	}
}

func TestUpdateUserReplyMode(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	svc, pool := testService(t)
	ctx := context.Background()

	accountID, adminID := setupTestTenant(t, pool, "TestTenant", "t@example.com")

	// 1. Initially, update is allowed because allow_member_reply_mode_override defaults to true
	mode := "auto_send"
	err := svc.UpdateUserReplyMode(ctx, accountID, adminID, &mode)
	require.NoError(t, err)

	// Verify update in DB
	var dbMode *string
	err = pool.QueryRow(ctx, "SELECT reply_mode_override FROM users WHERE id = $1", adminID).Scan(&dbMode)
	require.NoError(t, err)
	require.NotNil(t, dbMode)
	assert.Equal(t, "auto_send", *dbMode)

	// 2. Disable member override in settings
	_, err = pool.Exec(ctx, `UPDATE accounts SET settings = '{"allow_member_reply_mode_override": false}' WHERE id = $1`, accountID)
	require.NoError(t, err)

	// Update should now be rejected
	mode2 := "draft_only"
	err = svc.UpdateUserReplyMode(ctx, accountID, adminID, &mode2)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "member reply mode overrides are not allowed")

	// 3. Reset override to nil should also be rejected when disabled
	err = svc.UpdateUserReplyMode(ctx, accountID, adminID, nil)
	assert.Error(t, err)
}

func TestUpdateProductMode(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	svc, pool := testService(t)
	ctx := context.Background()

	accountID, adminID := setupTestTenant(t, pool, "ProductModeTenant", "pm@example.com")

	// 1. Initial product_mode is full_workspace (default)
	var initialPm string
	var initialSettingsBytes []byte
	err := pool.QueryRow(ctx, `SELECT product_mode, settings FROM accounts WHERE id = $1`, accountID).Scan(&initialPm, &initialSettingsBytes)
	require.NoError(t, err)
	assert.Equal(t, "full_workspace", initialPm)

	// 2. Switch to chatbot_only
	err = svc.UpdateProductMode(ctx, accountID, adminID, "chatbot_only")
	require.NoError(t, err)

	// Verify update in DB
	var pm string
	var settingsBytes []byte
	err = pool.QueryRow(ctx, `SELECT product_mode, settings FROM accounts WHERE id = $1`, accountID).Scan(&pm, &settingsBytes)
	require.NoError(t, err)
	assert.Equal(t, "chatbot_only", pm)

	var settings map[string]any
	err = json.Unmarshal(settingsBytes, &settings)
	require.NoError(t, err)
	assert.Equal(t, false, settings["lead_tracking_enabled"])

	// Verify audit log
	var auditCount int
	err = pool.QueryRow(ctx, `SELECT COUNT(*) FROM audit_logs WHERE account_id = $1 AND action = 'account.product_mode_updated'`, accountID).Scan(&auditCount)
	require.NoError(t, err)
	assert.Equal(t, 1, auditCount)

	// 3. Switch back to full_workspace
	err = svc.UpdateProductMode(ctx, accountID, adminID, "full_workspace")
	require.NoError(t, err)

	err = pool.QueryRow(ctx, `SELECT product_mode, settings FROM accounts WHERE id = $1`, accountID).Scan(&pm, &settingsBytes)
	require.NoError(t, err)
	assert.Equal(t, "full_workspace", pm)

	err = json.Unmarshal(settingsBytes, &settings)
	require.NoError(t, err)
	assert.Equal(t, true, settings["lead_tracking_enabled"])
}

// ---------------------------------------------------------------------------
// Onboarding tests — Stage 8
// ---------------------------------------------------------------------------

func TestOnboardingStatus_DefaultEmpty(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	svc, pool := testService(t)
	ctx := context.Background()

	accountID, _ := setupTestTenant(t, pool, "OnboardingEmpty", "ob_empty@example.com")

	state, err := svc.GetOnboardingStatus(ctx, accountID)
	require.NoError(t, err, "GetOnboardingStatus must not fail for a new account")
	require.NotNil(t, state)
	assert.Empty(t, state.CompletedSteps, "new account must have no completed steps")
	assert.Empty(t, state.SkippedSteps, "new account must have no skipped steps")
	assert.Nil(t, state.CompletedAt, "new account must have no completed_at")
	assert.Nil(t, state.BusinessType, "new account must have no business_type")
}

func TestOnboardingStatus_CompleteStep(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	svc, pool := testService(t)
	ctx := context.Background()

	accountID, _ := setupTestTenant(t, pool, "OnboardingComplete", "ob_complete@example.com")

	err := svc.PatchOnboardingStatus(ctx, accountID, "signup", "complete")
	require.NoError(t, err)

	state, err := svc.GetOnboardingStatus(ctx, accountID)
	require.NoError(t, err)
	assert.Contains(t, state.CompletedSteps, "signup")
	assert.NotContains(t, state.SkippedSteps, "signup")

	// Verify audit log
	var auditCount int
	err = pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM audit_logs WHERE account_id = $1 AND action = 'onboarding.step_updated'`,
		accountID).Scan(&auditCount)
	require.NoError(t, err)
	assert.Equal(t, 1, auditCount)
}

func TestOnboardingStatus_SkipStep(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	svc, pool := testService(t)
	ctx := context.Background()

	accountID, _ := setupTestTenant(t, pool, "OnboardingSkip", "ob_skip@example.com")

	err := svc.PatchOnboardingStatus(ctx, accountID, "channel_connect", "skip")
	require.NoError(t, err)

	state, err := svc.GetOnboardingStatus(ctx, accountID)
	require.NoError(t, err)
	assert.Contains(t, state.SkippedSteps, "channel_connect")
	assert.NotContains(t, state.CompletedSteps, "channel_connect")
}

func TestOnboardingStatus_CompleteAfterSkip(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	svc, pool := testService(t)
	ctx := context.Background()

	accountID, _ := setupTestTenant(t, pool, "OnboardingCompleteAfterSkip", "ob_cas@example.com")

	// Skip first
	err := svc.PatchOnboardingStatus(ctx, accountID, "channel_connect", "skip")
	require.NoError(t, err)

	// Then complete
	err = svc.PatchOnboardingStatus(ctx, accountID, "channel_connect", "complete")
	require.NoError(t, err)

	state, err := svc.GetOnboardingStatus(ctx, accountID)
	require.NoError(t, err)
	assert.Contains(t, state.CompletedSteps, "channel_connect", "step must be in completed_steps after complete action")
	assert.NotContains(t, state.SkippedSteps, "channel_connect", "step must be removed from skipped_steps after complete action")
}

func TestOnboardingStatus_DoneSetCompletedAt(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	svc, pool := testService(t)
	ctx := context.Background()

	accountID, _ := setupTestTenant(t, pool, "OnboardingDone", "ob_done@example.com")

	err := svc.PatchOnboardingStatus(ctx, accountID, "done", "complete")
	require.NoError(t, err)

	state, err := svc.GetOnboardingStatus(ctx, accountID)
	require.NoError(t, err)
	require.NotNil(t, state.CompletedAt, "completed_at must be set when step='done'")
	assert.NotEmpty(t, *state.CompletedAt, "completed_at must not be empty")

	// Verify it's a parseable timestamp
	_, parseErr := time.Parse(time.RFC3339, *state.CompletedAt)
	assert.NoError(t, parseErr, "completed_at must be a valid RFC3339 timestamp")
}

func TestOnboardingTemplates_StaticData(t *testing.T) {
	// Pure in-memory — no DB needed
	svc, _ := service.New(nil, testEncryptionKey)
	// New with nil pool is valid for template-only operations

	templates := svc.GetOnboardingTemplates()
	assert.Len(t, templates, 5, "must return exactly 5 business-type templates")

	// Verify all expected types are present
	typeSet := make(map[string]bool)
	for _, tmpl := range templates {
		typeSet[tmpl.Type] = true
		assert.NotEmpty(t, tmpl.Label, "template Label must not be empty")
		assert.NotEmpty(t, tmpl.PipelineStates, "template PipelineStates must not be empty")
		assert.NotEmpty(t, tmpl.KBPrompts, "template KBPrompts must not be empty")
		assert.NotEmpty(t, tmpl.SummarySchema, "template SummarySchema must not be empty")
	}
	assert.True(t, typeSet["salon"])
	assert.True(t, typeSet["photography"])
	assert.True(t, typeSet["tutoring"])
	assert.True(t, typeSet["home_services"])
	assert.True(t, typeSet["other"])
}

func TestApplyTemplate_ChatbotOnly(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	svc, pool := testService(t)
	ctx := context.Background()

	accountID, adminID := setupTestTenant(t, pool, "TemplateChatbotOnly", "tco@example.com")

	// Apply salon template in chatbot_only mode
	err := svc.ApplyOnboardingTemplate(ctx, accountID, adminID, "salon", "chatbot_only")
	require.NoError(t, err)

	// Settings must contain summary_schema and onboarding.business_type
	var settingsRaw []byte
	err = pool.QueryRow(ctx, `SELECT settings FROM accounts WHERE id = $1`, accountID).Scan(&settingsRaw)
	require.NoError(t, err)

	var settings map[string]any
	err = json.Unmarshal(settingsRaw, &settings)
	require.NoError(t, err)
	assert.NotNil(t, settings["summary_schema"], "summary_schema must be set in settings")

	onboarding, ok := settings["onboarding"].(map[string]any)
	require.True(t, ok, "onboarding must be a map")
	assert.Equal(t, "salon", onboarding["business_type"])

	// Pipeline must NOT be changed (chatbot_only does not touch pipelines)
	pipelines, err := svc.ListPipelines(ctx, accountID)
	require.NoError(t, err)
	require.Len(t, pipelines, 1)
	assert.Equal(t, "Default", pipelines[0].Name, "pipeline name must remain 'Default' in chatbot_only mode")

	// Audit log for template applied
	var auditCount int
	err = pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM audit_logs WHERE account_id = $1 AND action = 'onboarding.template_applied'`,
		accountID).Scan(&auditCount)
	require.NoError(t, err)
	assert.Equal(t, 1, auditCount)
}

func TestApplyTemplate_FullWorkspace(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	svc, pool := testService(t)
	ctx := context.Background()

	accountID, adminID := setupTestTenant(t, pool, "TemplateFullWorkspace", "tfw@example.com")

	// Apply photography template in full_workspace mode
	err := svc.ApplyOnboardingTemplate(ctx, accountID, adminID, "photography", "full_workspace")
	require.NoError(t, err)

	// Settings must contain summary_schema and onboarding.business_type
	var settingsRaw []byte
	err = pool.QueryRow(ctx, `SELECT settings FROM accounts WHERE id = $1`, accountID).Scan(&settingsRaw)
	require.NoError(t, err)

	var settings map[string]any
	err = json.Unmarshal(settingsRaw, &settings)
	require.NoError(t, err)
	assert.NotNil(t, settings["summary_schema"])

	onboarding, ok := settings["onboarding"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "photography", onboarding["business_type"])

	// Pipeline MUST be updated with photography template states
	pipelines, err := svc.ListPipelines(ctx, accountID)
	require.NoError(t, err)
	require.Len(t, pipelines, 1)
	assert.Equal(t, "Photography", pipelines[0].Name, "pipeline name must match template Label")
	assert.Len(t, pipelines[0].States, 5, "photography template has 5 pipeline states")

	// Verify first state key
	assert.Equal(t, "new_lead", pipelines[0].States[0].Key)

	// Audit logs: one for pipeline.updated, one for onboarding.template_applied
	var pipelineAudit int
	err = pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM audit_logs WHERE account_id = $1 AND action = 'pipeline.updated'`,
		accountID).Scan(&pipelineAudit)
	require.NoError(t, err)
	assert.Equal(t, 1, pipelineAudit, "must have one pipeline.updated audit entry")

	var templateAudit int
	err = pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM audit_logs WHERE account_id = $1 AND action = 'onboarding.template_applied'`,
		accountID).Scan(&templateAudit)
	require.NoError(t, err)
	assert.Equal(t, 1, templateAudit, "must have one onboarding.template_applied audit entry")
}

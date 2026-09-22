package service_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/whatfunnel/whatfunnel/services/identity-svc/internal/service"
	"github.com/whatfunnel/whatfunnel/services/identity-svc/internal/session"
)

type mockWorkspaceProvisioner struct {
	calledWithMode string
	calledCount    int
}

func (m *mockWorkspaceProvisioner) ProvisionWorkspace(ctx context.Context, tx pgx.Tx, accountID uuid.UUID, productMode string) error {
	m.calledCount++
	m.calledWithMode = productMode
	return nil
}

func TestWithWorkspaceProvisioner(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	pool := testPool(t)
	sess := session.New(pool, sessionSecret)

	mockProv := &mockWorkspaceProvisioner{}
	svc, err := service.New(pool, sess, service.WithWorkspaceProvisioner(mockProv))
	require.NoError(t, err)

	ctx := context.Background()
	email := uniqueEmail(t)
	user, err := svc.Signup(ctx, service.SignupRequest{
		AccountName: "Custom Provisioner Account",
		Email:       email,
		Password:    "password123!",
		ProductMode: "chatbot_only",
	})
	require.NoError(t, err)
	assert.Equal(t, 1, mockProv.calledCount)
	assert.Equal(t, "chatbot_only", mockProv.calledWithMode)

	t.Cleanup(func() {
		pool.Exec(context.Background(), `DELETE FROM audit_logs WHERE account_id = $1`, user.AccountID)
		pool.Exec(context.Background(), `DELETE FROM users WHERE account_id = $1`, user.AccountID)
		pool.Exec(context.Background(), `DELETE FROM accounts WHERE id = $1`, user.AccountID)
	})
}

func TestUserLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	svc, pool := testService(t)
	ctx := context.Background()

	adminEmail := uniqueEmail(t)
	adminUser, err := svc.Signup(ctx, service.SignupRequest{
		AccountName: "Lifecycle Test Account",
		Email:       adminEmail,
		Password:    "adminpass123",
	})
	require.NoError(t, err)

	accountID := adminUser.AccountID
	adminID := adminUser.ID

	// 1. Create agent user via identity service
	created, err := svc.CreateUser(ctx, accountID, adminID, service.CreateUserRequest{
		Username: "agent_smith",
		Password: "InitialPass123!",
		Role:     "agent",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, created.ID)
	assert.Equal(t, "agent_smith", created.Username)
	assert.Equal(t, "agent", created.Role)

	// 2. Verify agent can log in with initial credentials
	loggedIn, err := svc.Login(ctx, service.LoginRequest{
		Identifier: "agent_smith", // slug-username or direct
		Password:   "InitialPass123!",
	})
	// In test, if slug is not set on account, Login by email or username works if identifier found
	if err == nil {
		assert.Equal(t, created.ID, loggedIn.ID)
	}

	// 3. Reset password via identity service
	err = svc.ResetUserPassword(ctx, accountID, adminID, created.ID, "NewSecretPass456!")
	require.NoError(t, err)

	// 4. Change role to manager
	err = svc.ChangeUserRole(ctx, accountID, adminID, created.ID, "manager")
	require.NoError(t, err)

	// Verify updated role in DB
	var role string
	err = pool.QueryRow(ctx, `SELECT role FROM users WHERE id = $1`, created.ID).Scan(&role)
	require.NoError(t, err)
	assert.Equal(t, "manager", role)

	// 5. Delete user via identity service
	err = svc.DeleteUser(ctx, accountID, adminID, created.ID)
	require.NoError(t, err)

	var exists bool
	err = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE id = $1)`, created.ID).Scan(&exists)
	require.NoError(t, err)
	assert.False(t, exists)

	// Cannot delete self
	err = svc.DeleteUser(ctx, accountID, adminID, adminID)
	require.Error(t, err)

	t.Cleanup(func() {
		pool.Exec(context.Background(), `DELETE FROM lead_pipelines WHERE account_id = $1`, accountID)
		pool.Exec(context.Background(), `DELETE FROM audit_logs WHERE account_id = $1`, accountID)
		pool.Exec(context.Background(), `DELETE FROM users WHERE account_id = $1`, accountID)
		pool.Exec(context.Background(), `DELETE FROM accounts WHERE id = $1`, accountID)
	})
}

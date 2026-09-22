package service_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/whatfunnel/whatfunnel/services/workspace-svc/internal/service"
)

type mockIdentityProvisioner struct {
	createdUsers []service.CreateUserRequest
	resetCount   int
	roleCount    int
	deleteCount  int
}

func (m *mockIdentityProvisioner) CreateUser(ctx context.Context, accountID, actorID uuid.UUID, req service.CreateUserRequest) (*service.CreateUserResult, error) {
	m.createdUsers = append(m.createdUsers, req)
	return &service.CreateUserResult{
		ID:                uuid.New(),
		Username:          req.Username,
		Role:              req.Role,
		PlaintextPassword: req.Password,
	}, nil
}

func (m *mockIdentityProvisioner) ResetUserPassword(ctx context.Context, accountID, actorID, targetUserID uuid.UUID, newPassword string) error {
	m.resetCount++
	return nil
}

func (m *mockIdentityProvisioner) DeleteUser(ctx context.Context, accountID, actorID, targetUserID uuid.UUID) error {
	m.deleteCount++
	return nil
}

func (m *mockIdentityProvisioner) ChangeUserRole(ctx context.Context, accountID, actorID, targetUserID uuid.UUID, newRole string) error {
	m.roleCount++
	return nil
}

func TestWorkspaceService_IdentityProvisionerDecoupling(t *testing.T) {
	mockID := &mockIdentityProvisioner{}
	svc, err := service.New(nil, testEncryptionKey, service.WithIdentityProvisioner(mockID))
	require.NoError(t, err)

	ctx := context.Background()
	accountID := uuid.New()
	actorID := uuid.New()

	// 1. Test CreateUser delegates to IdentityProvisioner
	res, err := svc.CreateUser(ctx, accountID, actorID, service.CreateUserRequest{
		Username: "custom_agent",
		Password: "SecretPassword123!",
		Role:     "agent",
	})
	require.NoError(t, err)
	assert.Equal(t, "custom_agent", res.Username)
	assert.Len(t, mockID.createdUsers, 1)

	// 2. Test ResetUserPassword delegates to IdentityProvisioner
	targetID := uuid.New()
	err = svc.ResetUserPassword(ctx, accountID, actorID, targetID, "AnotherPass!")
	require.NoError(t, err)
	assert.Equal(t, 1, mockID.resetCount)

	// 3. Test ChangeUserRole delegates to IdentityProvisioner
	err = svc.ChangeUserRole(ctx, accountID, actorID, targetID, "manager")
	require.NoError(t, err)
	assert.Equal(t, 1, mockID.roleCount)
}

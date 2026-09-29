package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/whatfunnel/whatfunnel/services/workspace-svc/internal/service"
)

type failingDeleteIdentity struct {
	*directIdentityProvisioner
}

func (failingDeleteIdentity) DeleteUser(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) error {
	return errors.New("identity unavailable")
}

func TestDeleteUser_IdentityFailureKeepsAssignments(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	pool := testPool(t)
	ctx := context.Background()
	accountID, adminID := setupTestTenant(t, pool, "DeleteOrder", "delete_order@example.com")

	direct := newDirectIdentityProvisioner(pool)
	res, err := direct.CreateUser(ctx, accountID, adminID, service.CreateUserRequest{
		Username: "order_agent", Password: "Password123!", Role: "agent",
	})
	require.NoError(t, err)

	var channelID, contactID, convoID uuid.UUID
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO channels (account_id, type, status) VALUES ($1, 'whatsapp', 'connected') RETURNING id`,
		accountID).Scan(&channelID))
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO contacts (account_id, channel_id, external_identity) VALUES ($1, $2, 'order-c') RETURNING id`,
		accountID, channelID).Scan(&contactID))
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO conversations (account_id, contact_id, channel_id, status, assigned_user_ids)
		 VALUES ($1, $2, $3, 'open', $4) RETURNING id`,
		accountID, contactID, channelID, []uuid.UUID{res.ID}).Scan(&convoID))

	svc, err := service.New(pool, testEncryptionKey,
		service.WithIdentityProvisioner(failingDeleteIdentity{direct}))
	require.NoError(t, err)

	require.Error(t, svc.DeleteUser(ctx, accountID, adminID, res.ID))

	var assigned []uuid.UUID
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT assigned_user_ids FROM conversations WHERE id = $1`, convoID).Scan(&assigned))
	assert.Equal(t, []uuid.UUID{res.ID}, assigned, "assignments must survive a failed identity delete")
}

func TestServiceWithoutIdentityURLFailsUserOperations(t *testing.T) {
	t.Setenv("IDENTITY_SVC_URL", "")
	svc, err := service.New(nil, testEncryptionKey)
	require.NoError(t, err)
	_, err = svc.CreateUser(context.Background(), uuid.New(), uuid.New(), service.CreateUserRequest{})
	assert.ErrorIs(t, err, service.ErrIdentityNotConfigured)

	_, err = service.NewIdentityProvisioner("")
	assert.ErrorIs(t, err, service.ErrIdentityNotConfigured)
}

package service_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/whatfunnel/whatfunnel/packages/go-common/middleware"
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

func TestHTTPIdentityProvisioner_HeadersAndAuth(t *testing.T) {
	const expectedToken = "test-internal-token-for-provisioner-32ch"
	accountID := uuid.New()
	actorID := uuid.New()
	targetUserID := uuid.New()

	type capturedRequest struct {
		path          string
		method        string
		internalToken string
		accountID     string
		userID        string
		userRole      string
	}

	var captured []capturedRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = append(captured, capturedRequest{
			path:          r.URL.Path,
			method:        r.Method,
			internalToken: r.Header.Get("X-Internal-Token"),
			accountID:     r.Header.Get("X-Account-ID"),
			userID:        r.Header.Get("X-User-ID"),
			userRole:      r.Header.Get("X-User-Role"),
		})

		switch r.URL.Path {
		case "/auth/users":
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"id":"` + targetUserID.String() + `","username":"alice","role":"agent","plaintext_password":"pwd"}`))
		default:
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status":"ok"}`))
		}
	}))
	defer server.Close()

	t.Setenv("INTERNAL_SERVICE_TOKEN", expectedToken)
	provisioner := service.NewHTTPIdentityProvisioner(server.URL)

	ctx := context.Background()

	// 1. CreateUser
	_, err := provisioner.CreateUser(ctx, accountID, actorID, service.CreateUserRequest{
		Username: "alice",
		Password: "Password123!",
		Role:     "agent",
	})
	require.NoError(t, err)

	// 2. ResetUserPassword
	err = provisioner.ResetUserPassword(ctx, accountID, actorID, targetUserID, "NewPassword123!")
	require.NoError(t, err)

	// 3. ChangeUserRole
	err = provisioner.ChangeUserRole(ctx, accountID, actorID, targetUserID, "manager")
	require.NoError(t, err)

	// 4. DeleteUser
	err = provisioner.DeleteUser(ctx, accountID, actorID, targetUserID)
	require.NoError(t, err)

	require.Len(t, captured, 4)
	for i, req := range captured {
		assert.Equal(t, expectedToken, req.internalToken, "req %d (%s %s) missing or wrong X-Internal-Token", i, req.method, req.path)
		assert.Equal(t, accountID.String(), req.accountID, "req %d (%s %s) wrong X-Account-ID", i, req.method, req.path)
		assert.Equal(t, actorID.String(), req.userID, "req %d (%s %s) missing or wrong X-User-ID", i, req.method, req.path)
		assert.Equal(t, "manager", req.userRole, "req %d (%s %s) missing or wrong X-User-Role", i, req.method, req.path)
	}
}

type dummySessionStore struct{}

func (d *dummySessionStore) GetUserID(r *http.Request) (uuid.UUID, bool) {
	return uuid.Nil, false
}
func (d *dummySessionStore) GetAccountID(r *http.Request) (uuid.UUID, bool) {
	return uuid.Nil, false
}
func (d *dummySessionStore) GetRole(r *http.Request) (string, bool) {
	return "", false
}

func TestHTTPIdentityProvisioner_AuthEnforcementWithMiddleware(t *testing.T) {
	const validToken = "shared-cluster-internal-token-32-chars"
	t.Setenv("INTERNAL_SERVICE_TOKEN", validToken)

	mw := middleware.NewSessionMiddleware(&dummySessionStore{})
	handler := mw.RequireAuthenticated(middleware.RequireAdmin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		actID, _ := middleware.AccountIDFromContext(r)
		uID, _ := middleware.UserIDFromContext(r)
		role, _ := middleware.RoleFromContext(r)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"id":"` + uID.String() + `","username":"` + role + `","role":"` + role + `","plaintext_password":"p"}`))
		_ = actID
	})))

	server := httptest.NewServer(handler)
	defer server.Close()

	ctx := context.Background()
	accountID := uuid.New()
	actorID := uuid.New()

	// 1. With matching configured secret
	provSuccess := service.NewHTTPIdentityProvisioner(server.URL, validToken)
	res, err := provSuccess.CreateUser(ctx, accountID, actorID, service.CreateUserRequest{
		Username: "bob",
		Password: "Password123!",
		Role:     "agent",
	})
	require.NoError(t, err)
	assert.Equal(t, actorID, res.ID)

	// 2. With invalid secret -> fails with unauthenticated
	provFail := service.NewHTTPIdentityProvisioner(server.URL, "wrong-secret-token")
	_, err = provFail.CreateUser(ctx, accountID, actorID, service.CreateUserRequest{
		Username: "bob",
		Password: "Password123!",
		Role:     "agent",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unauthenticated")
}



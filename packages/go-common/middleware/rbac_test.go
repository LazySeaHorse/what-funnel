package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/whatfunnel/whatfunnel/packages/go-common/middleware"
	"github.com/whatfunnel/whatfunnel/packages/go-common/types"
)

// fakeStore is a test double for the session store.
type fakeStore struct {
	userID    uuid.UUID
	accountID uuid.UUID
	role      string
	loggedIn  bool
}

func (f *fakeStore) GetUserID(r *http.Request) (uuid.UUID, bool) {
	return f.userID, f.loggedIn
}
func (f *fakeStore) GetAccountID(r *http.Request) (uuid.UUID, bool) {
	return f.accountID, f.loggedIn
}
func (f *fakeStore) GetRole(r *http.Request) (string, bool) {
	return f.role, f.loggedIn
}

func newRequest() *http.Request {
	r, _ := http.NewRequest(http.MethodGet, "/", nil)
	return r
}

// okHandler returns 200 so we can assert the request got through.
var okHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
})

// ---------------------------------------------------------------------------
// RequireAuthenticated tests
// ---------------------------------------------------------------------------

func TestRequireAuthenticated_RejectsUnauthenticated(t *testing.T) {
	store := &fakeStore{loggedIn: false}
	m := middleware.NewSessionMiddleware(store)

	rr := httptest.NewRecorder()
	m.RequireAuthenticated(okHandler).ServeHTTP(rr, newRequest())

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

func TestRequireAuthenticated_AllowsAuthenticated(t *testing.T) {
	store := &fakeStore{
		loggedIn:  true,
		userID:    uuid.New(),
		accountID: uuid.New(),
		role:      types.RoleMember,
	}
	m := middleware.NewSessionMiddleware(store)

	rr := httptest.NewRecorder()
	m.RequireAuthenticated(okHandler).ServeHTTP(rr, newRequest())

	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestRequireAuthenticated_InjectsContextValues(t *testing.T) {
	uid := uuid.New()
	aid := uuid.New()
	store := &fakeStore{
		loggedIn:  true,
		userID:    uid,
		accountID: aid,
		role:      types.RoleAdmin,
	}
	m := middleware.NewSessionMiddleware(store)

	var gotUserID uuid.UUID
	var gotAccountID uuid.UUID
	var gotRole string

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUserID, _ = middleware.UserIDFromContext(r)
		gotAccountID, _ = middleware.AccountIDFromContext(r)
		gotRole, _ = middleware.RoleFromContext(r)
		w.WriteHeader(http.StatusOK)
	})

	rr := httptest.NewRecorder()
	m.RequireAuthenticated(inner).ServeHTTP(rr, newRequest())

	assert.Equal(t, uid, gotUserID)
	assert.Equal(t, aid, gotAccountID)
	assert.Equal(t, types.RoleAdmin, gotRole)
}

// ---------------------------------------------------------------------------
// RequireRole tests
// ---------------------------------------------------------------------------

func TestRequireRole_AdminCanAccessAdminRoute(t *testing.T) {
	store := &fakeStore{
		loggedIn:  true,
		userID:    uuid.New(),
		accountID: uuid.New(),
		role:      types.RoleAdmin,
	}
	m := middleware.NewSessionMiddleware(store)

	rr := httptest.NewRecorder()
	handler := m.RequireAuthenticated(middleware.RequireAdmin(okHandler))
	handler.ServeHTTP(rr, newRequest())

	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestRequireRole_MemberDeniedOnAdminRoute(t *testing.T) {
	store := &fakeStore{
		loggedIn:  true,
		userID:    uuid.New(),
		accountID: uuid.New(),
		role:      types.RoleMember,
	}
	m := middleware.NewSessionMiddleware(store)

	rr := httptest.NewRecorder()
	handler := m.RequireAuthenticated(middleware.RequireAdmin(okHandler))
	handler.ServeHTTP(rr, newRequest())

	assert.Equal(t, http.StatusForbidden, rr.Code)
}

func TestRequireRole_MemberCanAccessMemberRoute(t *testing.T) {
	store := &fakeStore{
		loggedIn:  true,
		userID:    uuid.New(),
		accountID: uuid.New(),
		role:      types.RoleMember,
	}
	m := middleware.NewSessionMiddleware(store)

	rr := httptest.NewRecorder()
	// Route allows both roles
	handler := m.RequireAuthenticated(middleware.RequireRole(types.RoleAdmin, types.RoleMember)(okHandler))
	handler.ServeHTTP(rr, newRequest())

	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestRequireRole_UnauthenticatedDeniedOnRoleRoute(t *testing.T) {
	store := &fakeStore{loggedIn: false}
	m := middleware.NewSessionMiddleware(store)

	rr := httptest.NewRecorder()
	handler := m.RequireAuthenticated(middleware.RequireAdmin(okHandler))
	handler.ServeHTTP(rr, newRequest())

	// RequireAuthenticated fires first, returns 401
	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

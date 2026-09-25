package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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
		role:      types.RoleAgent,
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
		role:      types.RoleManager,
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
	assert.Equal(t, types.RoleManager, gotRole)
}

// ---------------------------------------------------------------------------
// RequireRole tests
// ---------------------------------------------------------------------------

func TestRequireRole_AdminCanAccessAdminRoute(t *testing.T) {
	store := &fakeStore{
		loggedIn:  true,
		userID:    uuid.New(),
		accountID: uuid.New(),
		role:      types.RoleManager,
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
		role:      types.RoleAgent,
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
		role:      types.RoleAgent,
	}
	m := middleware.NewSessionMiddleware(store)

	rr := httptest.NewRecorder()
	// Route allows both roles
	handler := m.RequireAuthenticated(middleware.RequireRole(types.RoleManager, types.RoleAgent)(okHandler))
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

type fakeRow struct {
	val string
	err error
}

func (f *fakeRow) Scan(dest ...any) error {
	if f.err != nil {
		return f.err
	}
	*(dest[0].(*string)) = f.val
	return nil
}

type fakeQueryer struct {
	row *fakeRow
}

func (f *fakeQueryer) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return f.row
}

func TestRequireProductMode_Allowed(t *testing.T) {
	store := &fakeStore{
		loggedIn:  true,
		userID:    uuid.New(),
		accountID: uuid.New(),
		role:      types.RoleAgent,
	}
	q := &fakeQueryer{row: &fakeRow{val: "full_workspace"}}
	m := middleware.NewSessionMiddlewareWithDB(store, q)

	rr := httptest.NewRecorder()
	handler := m.RequireAuthenticated(m.RequireProductMode("full_workspace")(okHandler))
	handler.ServeHTTP(rr, newRequest())

	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestRequireProductMode_Denied(t *testing.T) {
	store := &fakeStore{
		loggedIn:  true,
		userID:    uuid.New(),
		accountID: uuid.New(),
		role:      types.RoleAgent,
	}
	q := &fakeQueryer{row: &fakeRow{val: "chatbot_only"}}
	m := middleware.NewSessionMiddlewareWithDB(store, q)

	rr := httptest.NewRecorder()
	handler := m.RequireAuthenticated(m.RequireProductMode("full_workspace")(okHandler))
	handler.ServeHTTP(rr, newRequest())

	assert.Equal(t, http.StatusForbidden, rr.Code)
}

func TestRequireAuthenticated_InternalToken_Valid(t *testing.T) {
	t.Setenv("INTERNAL_SERVICE_TOKEN", "super-secret-service-token-12345678")
	store := &fakeStore{loggedIn: false}
	m := middleware.NewSessionMiddleware(store)

	targetAccountID := uuid.New()
	targetUserID := uuid.New()

	var gotAccountID uuid.UUID
	var gotUserID uuid.UUID
	var gotRole string

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAccountID, _ = middleware.AccountIDFromContext(r)
		gotUserID, _ = middleware.UserIDFromContext(r)
		gotRole, _ = middleware.RoleFromContext(r)
		w.WriteHeader(http.StatusOK)
	})

	rr := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/internal/data", nil)
	req.Header.Set("X-Internal-Token", "super-secret-service-token-12345678")
	req.Header.Set("X-Account-ID", targetAccountID.String())
	req.Header.Set("X-User-ID", targetUserID.String())
	req.Header.Set("X-User-Role", "manager")

	m.RequireAuthenticated(testHandler).ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, targetAccountID, gotAccountID)
	assert.Equal(t, targetUserID, gotUserID)
	assert.Equal(t, "manager", gotRole)
}

func TestRequireAuthenticated_InternalToken_Invalid(t *testing.T) {
	t.Setenv("INTERNAL_SERVICE_TOKEN", "super-secret-service-token-12345678")
	store := &fakeStore{loggedIn: false}
	m := middleware.NewSessionMiddleware(store)

	rr := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/internal/data", nil)
	req.Header.Set("X-Internal-Token", "wrong-token")
	req.Header.Set("X-Account-ID", uuid.New().String())

	m.RequireAuthenticated(okHandler).ServeHTTP(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

func TestRequireAuthenticated_InternalToken_InsecureProductionDefaultBlocked(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("INTERNAL_SERVICE_TOKEN", "")
	t.Setenv("SESSION_SECRET", "")
	store := &fakeStore{loggedIn: false}
	m := middleware.NewSessionMiddleware(store)

	rr := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/internal/data", nil)
	req.Header.Set("X-Internal-Token", "change-me-in-production-at-least-32-chars")
	req.Header.Set("X-Account-ID", uuid.New().String())

	m.RequireAuthenticated(okHandler).ServeHTTP(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}


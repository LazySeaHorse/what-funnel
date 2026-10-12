package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/whatfunnel/whatfunnel/packages/go-common/middleware"
	"github.com/whatfunnel/whatfunnel/packages/go-common/types"
)

func TestListAuditLogs_InvalidParamsAre400(t *testing.T) {
	h := &Handler{} // validation happens before the service is touched
	for _, q := range []string{
		"limit=0", "limit=abc", "limit=-5",
		"actor_user_id=nope", "from=yesterday", "to=2024-01-01",
		"cursor=%%%", "cursor=bm90LWEtY3Vyc29y",
	} {
		t.Run(q, func(t *testing.T) {
			if q[:6] == "cursor" {
				t.Skip("cursor validity is checked by the service (covered by DB tests)")
			}
			rr := httptest.NewRecorder()
			h.ListAuditLogs(rr, httptest.NewRequest(http.MethodGet, "/workspace/audit-logs?"+q, nil))
			assert.Equal(t, http.StatusBadRequest, rr.Code)
		})
	}
}

func TestParseAuditLogFilter_Valid(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet,
		"/workspace/audit-logs?limit=999&action=user.login&from=2024-01-02T03:04:05Z&to=2024-02-01T00:00:00%2B02:00&actor_user_id=11111111-1111-1111-1111-111111111111", nil)
	f, err := parseAuditLogFilter(r)
	require.NoError(t, err)
	assert.Equal(t, 200, f.Limit)
	assert.Equal(t, "user.login", f.Action)
	require.NotNil(t, f.From)
	require.NotNil(t, f.To)
	require.NotNil(t, f.ActorUserID)
}

func TestListAuditLogs_NonAdminForbidden(t *testing.T) {
	h := &Handler{}
	guarded := middleware.RequireAdmin(http.HandlerFunc(h.ListAuditLogs))
	req := httptest.NewRequest(http.MethodGet, "/workspace/audit-logs", nil)
	req = req.WithContext(context.WithValue(req.Context(), types.ContextKeyUserRole, types.RoleAgent))
	rr := httptest.NewRecorder()
	guarded.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusForbidden, rr.Code)
}

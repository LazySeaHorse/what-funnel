package handler

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/whatfunnel/whatfunnel/packages/go-common/middleware"
	"github.com/whatfunnel/whatfunnel/services/workspace-svc/internal/service"
)

// ListAuditLogs handles GET /workspace/audit-logs (admin).
func (h *Handler) ListAuditLogs(w http.ResponseWriter, r *http.Request) {
	accountID, _ := middleware.AccountIDFromContext(r)
	f, err := parseAuditLogFilter(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	page, err := h.svc.ListAuditLogs(r.Context(), accountID, f)
	if err != nil {
		if errors.Is(err, service.ErrInvalidAuditCursor) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to list audit logs")
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func parseAuditLogFilter(r *http.Request) (service.AuditLogFilter, error) {
	q := r.URL.Query()
	f := service.AuditLogFilter{Cursor: q.Get("cursor"), Action: q.Get("action")}
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return f, errors.New("invalid limit")
		}
		f.Limit = min(n, service.MaxAuditLogLimit)
	}
	if v := q.Get("actor_user_id"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			return f, errors.New("invalid actor_user_id")
		}
		f.ActorUserID = &id
	}
	for _, p := range []struct {
		name string
		dst  **time.Time
	}{{"from", &f.From}, {"to", &f.To}} {
		if v := q.Get(p.name); v != "" {
			t, err := time.Parse(time.RFC3339, v)
			if err != nil {
				return f, errors.New("invalid " + p.name)
			}
			*p.dst = &t
		}
	}
	return f, nil
}

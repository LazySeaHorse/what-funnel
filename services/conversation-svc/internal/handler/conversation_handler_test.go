package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/whatfunnel/whatfunnel/packages/go-common/types"
)

func TestHandler_ListConversations_AuthFailures(t *testing.T) {
	h := &Handler{}

	t.Run("missing account", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/conversations", nil)
		rr := httptest.NewRecorder()
		h.ListConversations(rr, req)
		assert.Equal(t, http.StatusUnauthorized, rr.Code)
		assert.Contains(t, rr.Body.String(), "missing account")
	})

	t.Run("missing user", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/conversations", nil)
		ctx := context.WithValue(req.Context(), types.ContextKeyAccountID, uuid.New())
		req = req.WithContext(ctx)
		rr := httptest.NewRecorder()
		h.ListConversations(rr, req)
		assert.Equal(t, http.StatusUnauthorized, rr.Code)
		assert.Contains(t, rr.Body.String(), "missing user")
	})

	t.Run("missing role", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/conversations", nil)
		ctx := context.WithValue(req.Context(), types.ContextKeyAccountID, uuid.New())
		ctx = context.WithValue(ctx, types.ContextKeyUserID, uuid.New())
		req = req.WithContext(ctx)
		rr := httptest.NewRecorder()
		h.ListConversations(rr, req)
		assert.Equal(t, http.StatusUnauthorized, rr.Code)
		assert.Contains(t, rr.Body.String(), "missing role")
	})

	t.Run("invalid filter", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/conversations?filter=bogus", nil)
		ctx := context.WithValue(req.Context(), types.ContextKeyAccountID, uuid.New())
		ctx = context.WithValue(ctx, types.ContextKeyUserID, uuid.New())
		ctx = context.WithValue(ctx, types.ContextKeyUserRole, "agent")
		req = req.WithContext(ctx)
		rr := httptest.NewRecorder()
		h.ListConversations(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.Contains(t, rr.Body.String(), "invalid filter")
	})
}

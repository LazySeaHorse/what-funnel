package handler

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/whatfunnel/whatfunnel/services/conversation-svc/internal/service"
)

func TestWriteServiceError(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantBody   string
	}{
		{"not found sentinel", fmt.Errorf("wrapped: %w", service.ErrNotFound), http.StatusNotFound, ""},
		{"forbidden sentinel", service.ErrForbidden, http.StatusForbidden, ""},
		{"validation sentinel", service.ErrValidation, http.StatusBadRequest, ""},
		{"conflict sentinel", service.ErrConflict, http.StatusConflict, ""},
		{"internal error hides detail", errors.New("pq: connection refused at 10.0.0.5"), http.StatusInternalServerError, "internal error"},
		{"message containing not found is still internal", errors.New("row not found in secret_table"), http.StatusInternalServerError, "internal error"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			writeServiceError(rr, httptest.NewRequest(http.MethodGet, "/x", nil), tc.err)
			assert.Equal(t, tc.wantStatus, rr.Code)
			if tc.wantBody != "" {
				assert.Contains(t, rr.Body.String(), tc.wantBody)
				assert.NotContains(t, rr.Body.String(), "10.0.0.5")
				assert.NotContains(t, rr.Body.String(), "secret_table")
			}
		})
	}
}

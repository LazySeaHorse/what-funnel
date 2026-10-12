package handler

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/whatfunnel/whatfunnel/services/conversation-svc/internal/service"
)

func TestWriteServiceError_CategorizedSentinels(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
		body   string
	}{
		{"label exists", service.ErrProviderConnectionLabelExists, http.StatusConflict, `{"error":"A channel connection with that label already exists."}`},
		{"wrapped label exists", fmt.Errorf("start: %w", service.ErrProviderConnectionLabelExists), http.StatusConflict, `{"error":"A channel connection with that label already exists."}`},
		{"unsupported simulator provider", service.ErrUnsupportedSimulatorProvider, http.StatusBadRequest, `{"error":"simulator supports whatsapp and telegram only"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			writeServiceError(rr, httptest.NewRequest(http.MethodPost, "/", nil), tc.err)
			assert.Equal(t, tc.status, rr.Code)
			assert.JSONEq(t, tc.body, rr.Body.String())
		})
	}
}

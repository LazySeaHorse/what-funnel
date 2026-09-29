package handler

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/whatfunnel/whatfunnel/packages/go-common/messaging"
	"github.com/whatfunnel/whatfunnel/packages/go-common/types"
	"github.com/whatfunnel/whatfunnel/services/conversation-svc/internal/adapterclient"
)

func TestWriteConnectionFailure(t *testing.T) {
	connection := &types.ProviderConnection{
		ChannelID: uuid.New(), Provider: messaging.ProviderTelegram,
		Detail: "Could not connect this provider account. Check the credentials and try again.",
	}

	t.Run("adapter rejects the input", func(t *testing.T) {
		rr := httptest.NewRecorder()
		cause := fmt.Errorf("start provider adapter: %w", &adapterclient.Error{Status: http.StatusUnprocessableEntity, Message: "Telegram rejected this bot token."})
		writeConnectionFailure(rr, httptest.NewRequest(http.MethodPost, "/channel-connections", nil), connection, cause)
		assert.Equal(t, http.StatusUnprocessableEntity, rr.Code)
		assert.Contains(t, rr.Body.String(), "Telegram rejected this bot token.")
		assert.Contains(t, rr.Body.String(), connection.ChannelID.String())
	})

	t.Run("adapter failure", func(t *testing.T) {
		rr := httptest.NewRecorder()
		cause := fmt.Errorf("start provider adapter: %w", &adapterclient.Error{Status: http.StatusInternalServerError, Message: "internal adapter detail"})
		writeConnectionFailure(rr, httptest.NewRequest(http.MethodPost, "/channel-connections", nil), connection, cause)
		assert.Equal(t, http.StatusBadGateway, rr.Code)
		assert.Contains(t, rr.Body.String(), connection.Detail)
		assert.NotContains(t, rr.Body.String(), "internal adapter detail")
	})
}

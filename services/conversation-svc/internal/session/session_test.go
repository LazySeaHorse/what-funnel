package session

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSession_NewAndOptions(t *testing.T) {
	secret := "01234567890123456789012345678901"

	t.Run("Default secure option is false", func(t *testing.T) {
		s := New(nil, secret)
		require.NotNil(t, s)
		assert.False(t, s.options.Secure)
		assert.True(t, s.options.HttpOnly)
		assert.Equal(t, "/", s.options.Path)
	})

	t.Run("Explicit secure true option", func(t *testing.T) {
		s := New(nil, secret, true)
		require.NotNil(t, s)
		assert.True(t, s.options.Secure)
	})
}

func TestSession_GetSession_ErrorCases(t *testing.T) {
	secret := "01234567890123456789012345678901"
	s := New(nil, secret)

	t.Run("Missing cookie returns no cookie error", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/conversations", nil)
		data, err := s.GetSession(r)
		assert.Nil(t, data)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no cookie")

		uid, ok := s.GetUserID(r)
		assert.False(t, ok)
		assert.Equal(t, uuid.Nil, uid)

		accID, ok := s.GetAccountID(r)
		assert.False(t, ok)
		assert.Equal(t, uuid.Nil, accID)

		role, ok := s.GetRole(r)
		assert.False(t, ok)
		assert.Empty(t, role)
	})

	t.Run("Invalid signature cookie returns error", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/conversations", nil)
		r.AddCookie(&http.Cookie{Name: sessionName, Value: "tampered-cookie-value"})

		data, err := s.GetSession(r)
		assert.Nil(t, data)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid signature")
	})
}

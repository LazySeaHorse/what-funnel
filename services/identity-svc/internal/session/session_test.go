package session

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSession_SecureCookieOption(t *testing.T) {
	secret := "01234567890123456789012345678901"

	t.Run("Secure true applies to session and csrf cookies", func(t *testing.T) {
		s := New(nil, secret, true)
		assert.True(t, s.options.Secure)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		encoded, err := s.codec.Encode(sessionName, "dummy-token")
		require.NoError(t, err)
		r.AddCookie(&http.Cookie{Name: sessionName, Value: encoded})

		// Calling DestroySession to check cookie flags
		err = s.DestroySession(w, r)
		require.NoError(t, err)

		cookies := w.Result().Cookies()
		require.NotEmpty(t, cookies)
		for _, c := range cookies {
			assert.True(t, c.Secure, "cookie %s must have Secure: true", c.Name)
		}
	})

	t.Run("Secure false applies to session and csrf cookies", func(t *testing.T) {
		s := New(nil, secret, false)
		assert.False(t, s.options.Secure)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		encoded, err := s.codec.Encode(sessionName, "dummy-token")
		require.NoError(t, err)
		r.AddCookie(&http.Cookie{Name: sessionName, Value: encoded})

		err = s.DestroySession(w, r)
		require.NoError(t, err)

		cookies := w.Result().Cookies()
		require.NotEmpty(t, cookies)
		for _, c := range cookies {
			assert.False(t, c.Secure, "cookie %s must have Secure: false", c.Name)
		}
	})
}

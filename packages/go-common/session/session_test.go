package session

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/whatfunnel/whatfunnel/packages/go-common/db"
)

const testSecret = "01234567890123456789012345678901"

func TestGetSession_ErrorCases(t *testing.T) {
	s := NewReader(nil, testSecret)

	t.Run("missing cookie", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/conversations", nil)
		data, err := s.GetSession(r)
		assert.Nil(t, data)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no cookie")
	})

	t.Run("tampered cookie", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/conversations", nil)
		r.AddCookie(&http.Cookie{Name: CookieName, Value: "tampered-cookie-value"})
		data, err := s.GetSession(r)
		assert.Nil(t, data)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid cookie signature")
	})

	t.Run("cookie signed with another secret", func(t *testing.T) {
		other := NewReader(nil, "another-secret-another-secret-123")
		encoded, err := other.EncodeToken("token")
		require.NoError(t, err)
		r := httptest.NewRequest(http.MethodGet, "/conversations", nil)
		r.AddCookie(&http.Cookie{Name: CookieName, Value: encoded})
		_, err = s.GetSession(r)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid cookie signature")
	})
}

func TestGetSession_Database(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping database test in short mode")
	}
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://whatfunnel:whatfunnel@localhost:5432/whatfunnel?sslmode=disable"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	pool, err := db.Connect(ctx, dsn)
	if err != nil {
		if os.Getenv("CI") != "" {
			t.Fatalf("database unavailable in CI: %v", err)
		}
		t.Skipf("skipping test: database unavailable: %v", err)
	}
	defer pool.Close()

	s := NewReader(pool, testSecret)
	suffix := time.Now().Format("150405.000000")
	valid, expired := "reader-valid-"+suffix, "reader-expired-"+suffix
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM sessions WHERE token IN ($1, $2)`, valid, expired)
	})
	_, err = pool.Exec(ctx, `INSERT INTO sessions (token, data, expiry) VALUES ($1, $2, NOW() + INTERVAL '1 hour')`,
		valid, []byte(`{"user_id":"u","account_id":"a","role":"agent"}`))
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO sessions (token, data, expiry) VALUES ($1, $2, NOW() - INTERVAL '1 hour')`,
		expired, []byte(`{}`))
	require.NoError(t, err)

	request := func(token string) *http.Request {
		encoded, err := s.EncodeToken(token)
		require.NoError(t, err)
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.AddCookie(&http.Cookie{Name: CookieName, Value: encoded})
		return r
	}

	data, err := s.GetSession(request(valid))
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"user_id": "u", "account_id": "a", "role": "agent"}, data)

	_, err = s.GetSession(request(expired))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expired")

	_, err = s.GetSession(request("no-such-token"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

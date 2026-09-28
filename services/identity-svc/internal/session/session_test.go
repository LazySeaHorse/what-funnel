package session

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/whatfunnel/whatfunnel/packages/go-common/db"
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

func TestJanitor_ConfigAndDefaults(t *testing.T) {
	secret := "01234567890123456789012345678901"
	s := New(nil, secret, false)

	t.Run("zero or negative interval defaults to 1 hour", func(t *testing.T) {
		j1 := NewJanitor(s, 0, nil)
		assert.Equal(t, time.Hour, j1.interval)
		assert.NotNil(t, j1.logger)

		j2 := NewJanitor(s, -10*time.Minute, nil)
		assert.Equal(t, time.Hour, j2.interval)
	})

	t.Run("custom interval and logger", func(t *testing.T) {
		var buf bytes.Buffer
		customLogger := slog.New(slog.NewJSONHandler(&buf, nil))
		j := NewJanitor(s, 15*time.Minute, customLogger)
		assert.Equal(t, 15*time.Minute, j.interval)
		assert.Equal(t, customLogger, j.logger)
	})
}

func TestJanitor_NilPool(t *testing.T) {
	secret := "01234567890123456789012345678901"
	s := New(nil, secret, false)
	j := NewJanitor(s, time.Hour, nil)

	count, err := j.PurgeOnce(context.Background())
	assert.Error(t, err)
	assert.Equal(t, int64(0), count)
	assert.Contains(t, err.Error(), "pool is nil")
}

func TestJanitor_RunContextCancellation(t *testing.T) {
	secret := "01234567890123456789012345678901"
	s := New(nil, secret, false)
	j := NewJanitor(s, 100*time.Millisecond, slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)))

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	err := j.Run(ctx)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestJanitor_DatabasePurge(t *testing.T) {
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
		t.Skipf("skipping test: database unavailable: %v", err)
	}
	defer pool.Close()

	secret := "01234567890123456789012345678901"
	s := New(pool, secret, false)
	j := NewJanitor(s, time.Hour, slog.Default())

	expiredToken := "test-expired-token-" + time.Now().Format("150405.000000")
	validToken := "test-valid-token-" + time.Now().Format("150405.000000")

	// Cleanup test tokens when done
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM sessions WHERE token IN ($1, $2)`, expiredToken, validToken)
	})

	// Insert one expired session and one valid session
	_, err = pool.Exec(ctx, `INSERT INTO sessions (token, data, expiry) VALUES ($1, $2, NOW() - INTERVAL '1 hour')`, expiredToken, []byte("{}"))
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO sessions (token, data, expiry) VALUES ($1, $2, NOW() + INTERVAL '1 hour')`, validToken, []byte("{}"))
	require.NoError(t, err)

	// Run purge
	count, err := j.PurgeOnce(ctx)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, count, int64(1))

	// Verify expired token was deleted
	var dummy int
	err = pool.QueryRow(ctx, `SELECT 1 FROM sessions WHERE token = $1`, expiredToken).Scan(&dummy)
	assert.Error(t, err)

	// Verify valid token still exists
	err = pool.QueryRow(ctx, `SELECT 1 FROM sessions WHERE token = $1`, validToken).Scan(&dummy)
	assert.NoError(t, err)
}


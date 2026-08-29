package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfig_EnvAndCookieSecure(t *testing.T) {
	// Setup base required envs
	t.Setenv("DATABASE_URL", "postgres://localhost/test")
	t.Setenv("SESSION_SECRET", "secret-must-be-32-chars-long-1234567")
	t.Setenv("ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")

	t.Run("Default development environment", func(t *testing.T) {
		os.Unsetenv("ENV")
		os.Unsetenv("ENVIRONMENT")
		os.Unsetenv("COOKIE_SECURE")

		cfg, err := Load()
		require.NoError(t, err)
		assert.Equal(t, "development", cfg.Env)
		assert.True(t, cfg.IsDevelopment())
		assert.False(t, cfg.IsProduction())
		assert.False(t, cfg.IsTesting())
		assert.False(t, cfg.CookieSecure)
	})

	t.Run("Production environment enables CookieSecure by default", func(t *testing.T) {
		t.Setenv("ENV", "production")
		os.Unsetenv("COOKIE_SECURE")

		cfg, err := Load()
		require.NoError(t, err)
		assert.True(t, cfg.IsProduction())
		assert.False(t, cfg.IsDevelopment())
		assert.True(t, cfg.CookieSecure)
	})

	t.Run("Testing environment", func(t *testing.T) {
		t.Setenv("ENV", "testing")
		os.Unsetenv("COOKIE_SECURE")

		cfg, err := Load()
		require.NoError(t, err)
		assert.True(t, cfg.IsTesting())
		assert.False(t, cfg.IsProduction())
		assert.False(t, cfg.CookieSecure)
	})

	t.Run("Explicit COOKIE_SECURE override in development", func(t *testing.T) {
		t.Setenv("ENV", "development")
		t.Setenv("COOKIE_SECURE", "true")

		cfg, err := Load()
		require.NoError(t, err)
		assert.True(t, cfg.IsDevelopment())
		assert.True(t, cfg.CookieSecure)
	})

	t.Run("Allowed origins parsing", func(t *testing.T) {
		t.Setenv("ALLOWED_ORIGINS", "https://app.whatfunnel.com, https://admin.whatfunnel.com ")

		cfg, err := Load()
		require.NoError(t, err)
		assert.Equal(t, []string{"https://app.whatfunnel.com", "https://admin.whatfunnel.com"}, cfg.AllowedOrigins)
	})
}

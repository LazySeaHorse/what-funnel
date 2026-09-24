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

	t.Run("Default media storage is disk", func(t *testing.T) {
		os.Unsetenv("MEDIA_STORAGE_BACKEND")
		os.Unsetenv("MEDIA_CACHE_PATH")

		cfg, err := Load()
		require.NoError(t, err)
		assert.Equal(t, "disk", cfg.MediaStorageBackend)
		assert.Equal(t, "/data/media", cfg.MediaCachePath)
	})

	t.Run("S3 media storage configuration with defaults and overrides", func(t *testing.T) {
		t.Setenv("MEDIA_STORAGE_BACKEND", "s3")
		t.Setenv("MEDIA_S3_ENDPOINT", "minio.internal:9000")
		t.Setenv("MEDIA_S3_BUCKET", "custom-bucket")
		t.Setenv("MEDIA_S3_ACCESS_KEY", "custom-user")
		t.Setenv("MEDIA_S3_SECRET_KEY", "custom-secret")
		t.Setenv("MEDIA_S3_USE_SSL", "true")
		t.Setenv("MEDIA_S3_REGION", "eu-central-1")

		cfg, err := Load()
		require.NoError(t, err)
		assert.Equal(t, "s3", cfg.MediaStorageBackend)
		assert.Equal(t, "minio.internal:9000", cfg.MediaS3Endpoint)
		assert.Equal(t, "custom-bucket", cfg.MediaS3Bucket)
		assert.Equal(t, "custom-user", cfg.MediaS3AccessKey)
		assert.Equal(t, "custom-secret", cfg.MediaS3SecretKey)
		assert.True(t, cfg.MediaS3UseSSL)
		assert.Equal(t, "eu-central-1", cfg.MediaS3Region)
	})
}


package config

import (
	"fmt"
	"os"
	"strings"
)

// Config holds all environment-sourced configuration.
type Config struct {
	DatabaseURL   string
	SessionSecret string
	// EncryptionKey is a 32-byte hex-encoded AES-256 key for sensitive fields.
	// In production, source this from a secrets manager.
	EncryptionKey string
	RedisURL      string
	Port          string
	LogLevel      string
	// Env is the runtime environment ("production", "development", "testing").
	Env            string
	CookieSecure   bool
	AllowedOrigins []string
	// Adapter control endpoints are internal-only and authenticated with one
	// deployment secret. Provider credentials and session data stay inside the
	// adapter container.
	WhatsAppAdapterURL  string
	TelegramAdapterURL  string
	AdapterSharedSecret string
	MediaStorageBackend string
	MediaCachePath      string
	MediaS3Endpoint     string
	MediaS3Bucket       string
	MediaS3AccessKey    string
	MediaS3SecretKey    string
	MediaS3UseSSL       bool
	MediaS3Region       string
}

// IsProduction returns true if running in a production environment.
func (c *Config) IsProduction() bool {
	return strings.EqualFold(c.Env, "production") || strings.EqualFold(c.Env, "prod")
}

// IsTesting returns true if running in a test environment.
func (c *Config) IsTesting() bool {
	return strings.EqualFold(c.Env, "testing") || strings.EqualFold(c.Env, "test")
}

// IsDevelopment returns true if running in a development environment.
func (c *Config) IsDevelopment() bool {
	return !c.IsProduction() && !c.IsTesting()
}

// Load reads configuration from environment variables, returning an error if
// any required variable is missing.
func Load() (*Config, error) {
	env := os.Getenv("ENV")
	if env == "" {
		env = os.Getenv("ENVIRONMENT")
	}
	if env == "" {
		env = os.Getenv("APP_ENV")
	}
	if env == "" {
		env = "development"
	}

	cookieSecure := false
	if cookieSecureStr := os.Getenv("COOKIE_SECURE"); cookieSecureStr != "" {
		cookieSecure = strings.EqualFold(cookieSecureStr, "true") || cookieSecureStr == "1"
	} else {
		cookieSecure = strings.EqualFold(env, "production") || strings.EqualFold(env, "prod")
	}

	var allowedOrigins []string
	if origins := os.Getenv("ALLOWED_ORIGINS"); origins != "" {
		for _, o := range strings.Split(origins, ",") {
			if trimmed := strings.TrimSpace(o); trimmed != "" {
				allowedOrigins = append(allowedOrigins, trimmed)
			}
		}
	}

	cfg := &Config{
		DatabaseURL:         os.Getenv("DATABASE_URL"),
		SessionSecret:       os.Getenv("SESSION_SECRET"),
		EncryptionKey:       os.Getenv("ENCRYPTION_KEY"),
		RedisURL:            os.Getenv("REDIS_URL"),
		Port:                os.Getenv("PORT"),
		LogLevel:            os.Getenv("LOG_LEVEL"),
		Env:                 env,
		CookieSecure:        cookieSecure,
		AllowedOrigins:      allowedOrigins,
		WhatsAppAdapterURL:  os.Getenv("WHATSAPP_ADAPTER_URL"),
		TelegramAdapterURL:  os.Getenv("TELEGRAM_ADAPTER_URL"),
		AdapterSharedSecret: os.Getenv("ADAPTER_SHARED_SECRET"),
		MediaStorageBackend: strings.ToLower(strings.TrimSpace(os.Getenv("MEDIA_STORAGE_BACKEND"))),
		MediaCachePath:      os.Getenv("MEDIA_CACHE_PATH"),
		MediaS3Endpoint:     os.Getenv("MEDIA_S3_ENDPOINT"),
		MediaS3Bucket:       os.Getenv("MEDIA_S3_BUCKET"),
		MediaS3AccessKey:    os.Getenv("MEDIA_S3_ACCESS_KEY"),
		MediaS3SecretKey:    os.Getenv("MEDIA_S3_SECRET_KEY"),
		MediaS3UseSSL:       strings.EqualFold(os.Getenv("MEDIA_S3_USE_SSL"), "true") || os.Getenv("MEDIA_S3_USE_SSL") == "1",
		MediaS3Region:       os.Getenv("MEDIA_S3_REGION"),
	}

	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}
	if cfg.SessionSecret == "" {
		return nil, fmt.Errorf("SESSION_SECRET is required")
	}
	if cfg.EncryptionKey == "" {
		return nil, fmt.Errorf("ENCRYPTION_KEY is required")
	}
	if cfg.RedisURL == "" {
		cfg.RedisURL = "localhost:6379"
	}
	if cfg.Port == "" {
		cfg.Port = "8080"
	}
	if cfg.LogLevel == "" {
		cfg.LogLevel = "info"
	}
	if cfg.MediaStorageBackend == "" {
		cfg.MediaStorageBackend = "disk"
	}
	if cfg.MediaCachePath == "" {
		cfg.MediaCachePath = "/data/media"
	}
	if cfg.MediaStorageBackend == "s3" || cfg.MediaStorageBackend == "minio" {
		if cfg.MediaS3Endpoint == "" {
			cfg.MediaS3Endpoint = "minio:9000"
		}
		if cfg.MediaS3Bucket == "" {
			cfg.MediaS3Bucket = "whatfunnel-media"
		}
		if cfg.MediaS3AccessKey == "" {
			if rootUser := os.Getenv("MINIO_ROOT_USER"); rootUser != "" {
				cfg.MediaS3AccessKey = rootUser
			} else {
				cfg.MediaS3AccessKey = "minioadmin"
			}
		}
		if cfg.MediaS3SecretKey == "" {
			if rootPassword := os.Getenv("MINIO_ROOT_PASSWORD"); rootPassword != "" {
				cfg.MediaS3SecretKey = rootPassword
			} else {
				cfg.MediaS3SecretKey = "minioadmin"
			}
		}
		if cfg.MediaS3Region == "" {
			cfg.MediaS3Region = "us-east-1"
		}
	}
	if cfg.WhatsAppAdapterURL == "" {
		cfg.WhatsAppAdapterURL = "http://whatsapp-adapter:8086"
	}
	if cfg.TelegramAdapterURL == "" {
		cfg.TelegramAdapterURL = "http://telegram-adapter:8087"
	}

	return cfg, nil
}

// MustLoad is like Load but panics on error — useful in main().
func MustLoad() *Config {
	cfg, err := Load()
	if err != nil {
		panic(fmt.Sprintf("config: %v", err))
	}
	return cfg
}

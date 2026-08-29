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
	// EncryptionKey is a 32-byte hex-encoded AES-256 key for ai_provider_config.
	// In production, source this from a secrets manager.
	EncryptionKey string
	RedisURL      string
	Port          string
	LogLevel      string
	// Env is the runtime environment ("production", "development", "testing").
	Env            string
	CookieSecure   bool
	AllowedOrigins []string
	// Matrix connection control is deliberately server-only. The shared secret
	// is used solely to create an isolated Matrix puppet user per channel.
	MatrixHomeserverURL            string
	MatrixServerName               string
	MatrixRegistrationSharedSecret string
	MatrixWhatsAppBridgeIdentity   string
	MatrixTelegramBridgeIdentity   string
	MatrixInstagramBridgeIdentity  string
	MatrixMessengerBridgeIdentity  string
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
		DatabaseURL:                    os.Getenv("DATABASE_URL"),
		SessionSecret:                  os.Getenv("SESSION_SECRET"),
		EncryptionKey:                  os.Getenv("ENCRYPTION_KEY"),
		RedisURL:                       os.Getenv("REDIS_URL"),
		Port:                           os.Getenv("PORT"),
		LogLevel:                       os.Getenv("LOG_LEVEL"),
		Env:                            env,
		CookieSecure:                   cookieSecure,
		AllowedOrigins:                 allowedOrigins,
		MatrixHomeserverURL:            os.Getenv("MATRIX_HOMESERVER_URL"),
		MatrixServerName:               os.Getenv("MATRIX_SERVER_NAME"),
		MatrixRegistrationSharedSecret: os.Getenv("MATRIX_REGISTRATION_SHARED_SECRET"),
		MatrixWhatsAppBridgeIdentity:   os.Getenv("MATRIX_WHATSAPP_BRIDGE_IDENTITY"),
		MatrixTelegramBridgeIdentity:   os.Getenv("MATRIX_TELEGRAM_BRIDGE_IDENTITY"),
		MatrixInstagramBridgeIdentity:  os.Getenv("MATRIX_INSTAGRAM_BRIDGE_IDENTITY"),
		MatrixMessengerBridgeIdentity:  os.Getenv("MATRIX_MESSENGER_BRIDGE_IDENTITY"),
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

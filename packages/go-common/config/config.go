package config

import (
	"fmt"
	"os"
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
}

// Load reads configuration from environment variables, returning an error if
// any required variable is missing.
func Load() (*Config, error) {
	cfg := &Config{
		DatabaseURL:   os.Getenv("DATABASE_URL"),
		SessionSecret: os.Getenv("SESSION_SECRET"),
		EncryptionKey: os.Getenv("ENCRYPTION_KEY"),
		RedisURL:      os.Getenv("REDIS_URL"),
		Port:          os.Getenv("PORT"),
		LogLevel:      os.Getenv("LOG_LEVEL"),
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

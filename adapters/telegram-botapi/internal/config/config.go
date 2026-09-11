package config

import (
	"errors"
	"os"
	"strings"
)

type Config struct {
	Port                    string
	RedisURL                string
	DatabasePath            string
	ConversationInternalURL string
	SharedSecret            string
	EncryptionKey           string
	ConsumerName            string
	BotAPIBaseURL           string
}

func Load() (Config, error) {
	config := Config{
		Port: valueOrDefault("PORT", "8087"), RedisURL: valueOrDefault("REDIS_URL", "redis:6379"),
		DatabasePath:            valueOrDefault("TELEGRAM_DATABASE_PATH", "/data/telegram.db"),
		ConversationInternalURL: valueOrDefault("CONVERSATION_INTERNAL_URL", "http://conversation-svc:8083"),
		SharedSecret:            strings.TrimSpace(os.Getenv("ADAPTER_SHARED_SECRET")),
		EncryptionKey:           strings.TrimSpace(os.Getenv("ENCRYPTION_KEY")),
		ConsumerName:            valueOrDefault("ADAPTER_CONSUMER_NAME", hostname()),
		BotAPIBaseURL:           valueOrDefault("TELEGRAM_BOT_API_URL", "https://api.telegram.org"),
	}
	if config.SharedSecret == "" {
		return Config{}, errors.New("ADAPTER_SHARED_SECRET is required")
	}
	if config.EncryptionKey == "" {
		return Config{}, errors.New("ENCRYPTION_KEY is required")
	}
	return config, nil
}

func valueOrDefault(name, defaultValue string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return defaultValue
}

func hostname() string {
	name, err := os.Hostname()
	if err != nil || strings.TrimSpace(name) == "" {
		return "telegram-adapter-1"
	}
	return name
}

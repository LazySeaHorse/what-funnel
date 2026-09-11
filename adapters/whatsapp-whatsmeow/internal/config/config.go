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
	ConsumerName            string
}

func Load() (Config, error) {
	config := Config{
		Port:                    valueOrDefault("PORT", "8086"),
		RedisURL:                valueOrDefault("REDIS_URL", "redis:6379"),
		DatabasePath:            valueOrDefault("WHATSAPP_DATABASE_PATH", "/data/whatsapp.db"),
		ConversationInternalURL: valueOrDefault("CONVERSATION_INTERNAL_URL", "http://conversation-svc:8083"),
		SharedSecret:            strings.TrimSpace(os.Getenv("ADAPTER_SHARED_SECRET")),
		ConsumerName:            valueOrDefault("ADAPTER_CONSUMER_NAME", hostname()),
	}
	if config.SharedSecret == "" {
		return Config{}, errors.New("ADAPTER_SHARED_SECRET is required")
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
		return "whatsapp-adapter-1"
	}
	return name
}

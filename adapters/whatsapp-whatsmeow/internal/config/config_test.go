package config

import "testing"

func TestLoad(t *testing.T) {
	t.Setenv("ADAPTER_SHARED_SECRET", "test-secret")
	t.Setenv("PORT", "")
	t.Setenv("REDIS_URL", "")
	t.Setenv("WHATSAPP_DATABASE_PATH", "")
	t.Setenv("CONVERSATION_INTERNAL_URL", "")

	config, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if config.Port != "8086" {
		t.Errorf("port = %q, want 8086", config.Port)
	}
	if config.RedisURL != "redis:6379" {
		t.Errorf("redis url = %q, want redis:6379", config.RedisURL)
	}
}

func TestLoadRequiresSharedSecret(t *testing.T) {
	t.Setenv("ADAPTER_SHARED_SECRET", "")
	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want error")
	}
}

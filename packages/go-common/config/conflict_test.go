package config

import "testing"

// The method and the package-level check must agree even when ENV and APP_ENV
// conflict; production wins.
func TestConfigIsProductionMatchesHelperOnConflictingEnv(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("ENV", "development")
	t.Setenv("ENVIRONMENT", "")
	cfg := &Config{Env: "development"}
	if !cfg.IsProduction() {
		t.Fatal("Config.IsProduction must report production when APP_ENV=production")
	}
	if cfg.IsDevelopment() {
		t.Fatal("a production process is not a development one")
	}
}

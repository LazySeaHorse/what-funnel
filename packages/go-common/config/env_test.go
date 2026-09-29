package config

import "testing"

func TestIsProductionHelper(t *testing.T) {
	cases := map[string]bool{"production": true, "PROD": true, "Production": true, "development": false, "": false, "staging": false}
	for env, want := range cases {
		t.Setenv("APP_ENV", env)
		t.Setenv("ENV", "")
		t.Setenv("ENVIRONMENT", "")
		if got := IsProduction(); got != want {
			t.Errorf("APP_ENV=%q: got %v, want %v", env, got, want)
		}
	}
}

func TestSimulationRoutesEnabled(t *testing.T) {
	t.Setenv("ENABLE_SIMULATION_ROUTES", "")
	if SimulationRoutesEnabled() {
		t.Fatal("must default to disabled")
	}
	t.Setenv("ENABLE_SIMULATION_ROUTES", "true")
	if !SimulationRoutesEnabled() {
		t.Fatal("expected enabled")
	}
}

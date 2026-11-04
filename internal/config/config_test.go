package config

import (
	"strings"
	"testing"
	"time"
)

func env(vars map[string]string) func(string) string {
	return func(k string) string { return vars[k] }
}

func validProd() map[string]string {
	return map[string]string{
		"DATABASE_URL":      "postgres://u:p@localhost:5432/db",
		"SESSION_SECRET":    strings.Repeat("x", 64),
		"APPLE_TEAM_ID":     "TEAM",
		"APPLE_KEY_ID":      "KEY",
		"APPLE_PRIVATE_KEY": "PRIVATE",
	}
}

func TestLoadProductionDefaults(t *testing.T) {
	cfg, err := load(env(validProd()))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !cfg.IsProduction() {
		t.Errorf("Env = %q, want production by default", cfg.Env)
	}
	if cfg.Port != "8080" {
		t.Errorf("Port = %q, want 8080", cfg.Port)
	}
	if cfg.ShutdownTimeout != 25*time.Second {
		t.Errorf("ShutdownTimeout = %s, want 25s", cfg.ShutdownTimeout)
	}
}

func TestLoadDevelopmentNeedsOnlyDatabaseURL(t *testing.T) {
	_, err := load(env(map[string]string{
		"APP_ENV":      "development",
		"DATABASE_URL": "postgresql://localhost/db",
	}))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
}

func TestLoadReportsEveryError(t *testing.T) {
	_, err := load(env(map[string]string{
		"PORT":             "http",
		"LOG_LEVEL":        "loud",
		"SHUTDOWN_TIMEOUT": "45s",
	}))
	if err == nil {
		t.Fatal("expected error")
	}
	for _, want := range []string{
		"PORT", "DATABASE_URL is required", "LOG_LEVEL", "SHUTDOWN_TIMEOUT",
		"SESSION_SECRET", "APPLE_TEAM_ID", "APPLE_KEY_ID", "APPLE_PRIVATE_KEY",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error missing %q:\n%v", want, err)
		}
	}
}

func TestLoadRejectsNonPostgresURL(t *testing.T) {
	vars := validProd()
	vars["DATABASE_URL"] = "mysql://localhost/db"
	if _, err := load(env(vars)); err == nil || !strings.Contains(err.Error(), "postgres://") {
		t.Fatalf("err = %v, want postgres URL error", err)
	}
}

func TestLoadRejectsUnknownEnv(t *testing.T) {
	vars := validProd()
	vars["APP_ENV"] = "staging"
	if _, err := load(env(vars)); err == nil || !strings.Contains(err.Error(), "APP_ENV") {
		t.Fatalf("err = %v, want APP_ENV error", err)
	}
}

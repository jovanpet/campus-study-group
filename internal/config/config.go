// Package config loads and validates application configuration from the
// environment. Load fails fast with every problem at once so a misconfigured
// dyno crashes on boot instead of serving broken requests.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	EnvProduction  = "production"
	EnvDevelopment = "development"

	minSessionSecretLen = 32
)

type Config struct {
	Env             string
	AppName         string
	Port            string
	DatabaseURL     string
	SessionSecret   string
	LogLevel        slog.Level
	ShutdownTimeout time.Duration

	Apple Apple
}

type Apple struct {
	TeamID     string
	KeyID      string
	PrivateKey string
}

func (c Config) IsProduction() bool { return c.Env == EnvProduction }

// Load reads configuration from the process environment.
func Load() (Config, error) {
	return load(os.Getenv)
}

func load(getenv func(string) string) (Config, error) {
	var errs []error

	env := getOr(getenv, "APP_ENV", EnvProduction)
	if env != EnvProduction && env != EnvDevelopment {
		errs = append(errs, fmt.Errorf("APP_ENV must be %q or %q, got %q", EnvProduction, EnvDevelopment, env))
	}

	cfg := Config{
		Env:           env,
		AppName:       getOr(getenv, "APP_NAME", "campus-study-group"),
		Port:          getOr(getenv, "PORT", "8080"),
		DatabaseURL:   getenv("DATABASE_URL"),
		SessionSecret: getenv("SESSION_SECRET"),
		Apple: Apple{
			TeamID:     getenv("APPLE_TEAM_ID"),
			KeyID:      getenv("APPLE_KEY_ID"),
			PrivateKey: getenv("APPLE_PRIVATE_KEY"),
		},
	}

	if p, err := strconv.Atoi(cfg.Port); err != nil || p < 1 || p > 65535 {
		errs = append(errs, fmt.Errorf("PORT must be a number between 1 and 65535, got %q", cfg.Port))
	}

	if cfg.DatabaseURL == "" {
		errs = append(errs, errors.New("DATABASE_URL is required"))
	} else if u, err := url.Parse(cfg.DatabaseURL); err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.Host == "" {
		errs = append(errs, errors.New("DATABASE_URL must be a postgres:// URL"))
	}

	if err := cfg.LogLevel.UnmarshalText([]byte(getOr(getenv, "LOG_LEVEL", "info"))); err != nil {
		errs = append(errs, fmt.Errorf("LOG_LEVEL: %w", err))
	}

	timeout, err := time.ParseDuration(getOr(getenv, "SHUTDOWN_TIMEOUT", "25s"))
	switch {
	case err != nil:
		errs = append(errs, fmt.Errorf("SHUTDOWN_TIMEOUT: %w", err))
	case timeout <= 0 || timeout >= 30*time.Second:
		// Heroku sends SIGKILL 30s after SIGTERM.
		errs = append(errs, fmt.Errorf("SHUTDOWN_TIMEOUT must be between 0s and 30s, got %s", timeout))
	}
	cfg.ShutdownTimeout = timeout

	if cfg.IsProduction() {
		if len(cfg.SessionSecret) < minSessionSecretLen {
			errs = append(errs, fmt.Errorf("SESSION_SECRET must be at least %d characters in production", minSessionSecretLen))
		}
		for name, v := range map[string]string{
			"APPLE_TEAM_ID":     cfg.Apple.TeamID,
			"APPLE_KEY_ID":      cfg.Apple.KeyID,
			"APPLE_PRIVATE_KEY": cfg.Apple.PrivateKey,
		} {
			if v == "" {
				errs = append(errs, fmt.Errorf("%s is required in production", name))
			}
		}
	}

	if len(errs) > 0 {
		return Config{}, fmt.Errorf("invalid config: %w", errors.Join(errs...))
	}
	return cfg, nil
}

func getOr(getenv func(string) string, key, fallback string) string {
	if v := strings.TrimSpace(getenv(key)); v != "" {
		return v
	}
	return fallback
}

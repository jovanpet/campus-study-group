package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/jovanpet/campus-study-group/internal/config"
	"github.com/jovanpet/campus-study-group/internal/example"
	"github.com/jovanpet/campus-study-group/internal/httpapi"
	"github.com/jovanpet/campus-study-group/internal/store"
)

func main() {
	cmd := "serve"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	if err := run(cmd); err != nil {
		os.Exit(1)
	}
}
func run(cmd string) error {
	cfg, err := config.Load()
	if err != nil {
		// No configured logger yet: still emit JSON so the error parses in prod logs.
		slog.New(slog.NewJSONHandler(os.Stderr, nil)).Error("startup failed", "error", err.Error())
		return err
	}

	log := newLogger(cfg).With("app", cfg.AppName)
	slog.SetDefault(log)
	log.Info("starting", "env", cfg.Env, "port", cfg.Port)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()

	db, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("database unavailable", "error", err)
		return err
	}
	defer db.Close()

	switch cmd {
	case "serve":
		exampleHandler := example.NewHandler(example.NewService(example.NewPostgresRepository(db)))

		handler := httpapi.NewHandler(log, exampleHandler)
		srv := httpapi.New(fmt.Sprintf(":%s", cfg.Port), handler, log, cfg.ShutdownTimeout)
		if err := srv.Run(ctx); err != nil {
			log.Error("server error", "error", err)
			return err
		}
	case "migrate":
		if err := store.Migrate(ctx, db); err != nil {
			log.Error("migration failed", "error", err)
			return err
		}
	default:
		log.Error("unknown command", "command", cmd)
	}
	return nil
}

// newLogger returns JSON logs in production (parseable by Heroku log drains)
// and readable text logs in development.
func newLogger(cfg config.Config) *slog.Logger {
	opts := &slog.HandlerOptions{Level: cfg.LogLevel}
	if cfg.IsProduction() {
		return slog.New(slog.NewJSONHandler(os.Stdout, opts))
	}
	return slog.New(slog.NewTextHandler(os.Stdout, opts))
}

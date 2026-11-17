package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jovanpet/campus-study-group/internal/auth"
	"github.com/jovanpet/campus-study-group/internal/config"
	"github.com/jovanpet/campus-study-group/internal/httpapi"
	"github.com/jovanpet/campus-study-group/internal/store"
	"github.com/jovanpet/campus-study-group/internal/user"
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
		handler, err := newHandler(cfg, db, log)
		if err != nil {
			log.Error("startup failed", "error", err)
			return err
		}
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

// newHandler wires every domain module into the root HTTP handler.
func newHandler(cfg config.Config, db *pgxpool.Pool, log *slog.Logger) (http.Handler, error) {
	// Production config requires the Apple credentials; in development they are
	// optional and sign-in goes through POST /auth/dev instead.
	var apple auth.Apple
	if cfg.Apple.ClientID != "" {
		client, err := auth.NewAppleClient(cfg.Apple)
		if err != nil {
			return nil, err
		}
		apple = client
	} else {
		log.Warn("Apple sign-in is not configured; POST /auth/apple will return 503")
	}

	users := user.NewPostgresRepository(db)
	authSvc, err := auth.NewService(apple, users, auth.NewPostgresTokenRepository(db), cfg.SessionSecret)
	if err != nil {
		return nil, err
	}

	return httpapi.NewHandler(log,
		auth.NewHandler(authSvc, !cfg.IsProduction()),
		user.NewHandler(user.NewService(users), auth.RequireUser(authSvc)),
	), nil
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

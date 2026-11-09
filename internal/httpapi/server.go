// Package httpapi owns the HTTP layer: the server lifecycle, routing,
// middleware and JSON helpers shared by every domain handler.
package httpapi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"
)

type Server struct {
	http            *http.Server
	log             *slog.Logger
	shutdownTimeout time.Duration
}

func New(addr string, handler http.Handler, log *slog.Logger, shutdownTimeout time.Duration) *Server {
	return &Server{
		http: &http.Server{
			Addr:              addr,
			Handler:           handler,
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       15 * time.Second,
			WriteTimeout:      15 * time.Second,
			IdleTimeout:       60 * time.Second,
			ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelError),
		},
		log:             log,
		shutdownTimeout: shutdownTimeout,
	}
}

// Run serves until ctx is cancelled (e.g. SIGTERM from a Heroku dyno restart),
// then stops accepting connections and waits up to shutdownTimeout for
// in-flight requests to finish.
func (s *Server) Run(ctx context.Context) error {
	ln, err := net.Listen("tcp", s.http.Addr)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	return s.serve(ctx, ln)
}

func (s *Server) serve(ctx context.Context, ln net.Listener) error {
	errCh := make(chan error, 1)
	go func() {
		s.log.Info("server listening", "addr", ln.Addr().String())
		errCh <- s.http.Serve(ln)
	}()

	select {
	case err := <-errCh:
		return fmt.Errorf("serve: %w", err)
	case <-ctx.Done():
	}

	s.log.Info("shutting down", "timeout", s.shutdownTimeout.String())
	shutdownCtx, cancel := context.WithTimeout(context.Background(), s.shutdownTimeout)
	defer cancel()
	if err := s.http.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	if err := <-errCh; !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve: %w", err)
	}
	s.log.Info("server stopped")
	return nil
}

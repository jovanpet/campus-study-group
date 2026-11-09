package httpapi

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestHealth(t *testing.T) {
	rec := httptest.NewRecorder()
	NewHandler(discardLogger()).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "OK" {
		t.Fatalf("GET /health = %d %q, want 200 \"OK\"", rec.Code, rec.Body.String())
	}
}

func TestShutdownWaitsForInFlightRequests(t *testing.T) {
	started := make(chan struct{})
	s := New("", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		time.Sleep(200 * time.Millisecond)
		_, _ = w.Write([]byte("done"))
	}), discardLogger(), 5*time.Second)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	runErr := make(chan error, 1)
	go func() { runErr <- s.serve(ctx, ln) }()

	respCh := make(chan string, 1)
	go func() {
		resp, err := http.Get("http://" + ln.Addr().String())
		if err != nil {
			respCh <- "error: " + err.Error()
			return
		}
		defer func() { _ = resp.Body.Close() }()
		b, _ := io.ReadAll(resp.Body)
		respCh <- string(b)
	}()

	<-started
	cancel() // simulate SIGTERM mid-request

	if got := <-respCh; got != "done" {
		t.Errorf("in-flight response = %q, want %q", got, "done")
	}
	if err := <-runErr; err != nil {
		t.Errorf("serve returned %v, want nil", err)
	}
}

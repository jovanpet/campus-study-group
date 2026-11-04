package httpapi

import (
	"log/slog"
	"net/http"
)

// Module is implemented by each domain package's HTTP handler.
type Module interface {
	RegisterRoutes(mux *http.ServeMux)
}

// NewHandler builds the root handler: shared routes, every module's routes,
// and request logging around all of it.
func NewHandler(log *slog.Logger, modules ...Module) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", handleHealth)
	for _, m := range modules {
		m.RegisterRoutes(mux)
	}
	return logRequests(log, mux)
}

func handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("OK"))
}

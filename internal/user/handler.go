package user

import (
	"errors"
	"log/slog"
	"net/http"
	"uuid"

	"github.com/jovanpet/campus-study-group/internal/httpapi"
)

type Handler struct {
	svc         *Service
	requireUser func(http.Handler) http.Handler
}

// NewHandler takes the auth middleware (auth.RequireUser) rather than
// importing auth, which itself depends on this package.
func NewHandler(svc *Service, requireUser func(http.Handler) http.Handler) *Handler {
	return &Handler{svc: svc, requireUser: requireUser}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("GET /me", h.requireUser(http.HandlerFunc(h.me)))
	mux.Handle("PUT /me/profile", h.requireUser(http.HandlerFunc(h.completeProfile)))
	mux.Handle("GET /schools", h.requireUser(http.HandlerFunc(h.listSchools)))
	mux.Handle("GET /majors", h.requireUser(http.HandlerFunc(h.listMajors)))
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	id, ok := httpapi.UserID(r.Context())
	if !ok {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	u, err := h.svc.Get(r.Context(), id)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, u)
}

func (h *Handler) completeProfile(w http.ResponseWriter, r *http.Request) {
	id, ok := httpapi.UserID(r.Context())
	if !ok {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req struct {
		Username    string    `json:"username"`
		DisplayName string    `json:"display_name"`
		SchoolID    uuid.UUID `json:"school_id"`
		MajorID     uuid.UUID `json:"major_id"`
		Degree      Degree    `json:"degree"`
	}
	if err := httpapi.DecodeJSON(w, r, &req); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	u, err := h.svc.CompleteProfile(r.Context(), id, Profile(req))
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, u)
}

func (h *Handler) listSchools(w http.ResponseWriter, r *http.Request) {
	schools, err := h.svc.ListSchools(r.Context())
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, schools)
}

func (h *Handler) listMajors(w http.ResponseWriter, r *http.Request) {
	majors, err := h.svc.ListMajors(r.Context())
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, majors)
}

// writeServiceError maps domain errors to HTTP statuses; anything unexpected
// is logged and hidden behind a generic 500.
func writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrInvalid):
		httpapi.WriteError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrUsernameTaken):
		httpapi.WriteError(w, http.StatusConflict, err.Error())
	case errors.Is(err, ErrNotFound):
		// The token outlived its user (deleted account).
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized")
	default:
		slog.ErrorContext(r.Context(), "user request failed", "error", err)
		httpapi.WriteError(w, http.StatusInternalServerError, "internal error")
	}
}

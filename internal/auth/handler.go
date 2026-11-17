package auth

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/jovanpet/campus-study-group/internal/httpapi"
	"github.com/jovanpet/campus-study-group/internal/user"
)

type Handler struct {
	svc      *Service
	devLogin bool
}

// NewHandler builds the auth routes. devLogin adds POST /auth/dev, which signs
// in without Apple; it must be false in production.
func NewHandler(svc *Service, devLogin bool) *Handler {
	return &Handler{svc: svc, devLogin: devLogin}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	requireUser := RequireUser(h.svc)
	mux.HandleFunc("POST /auth/apple", h.signInWithApple)
	mux.Handle("POST /auth/logout", requireUser(http.HandlerFunc(h.logout)))
	mux.Handle("DELETE /me", requireUser(http.HandlerFunc(h.deleteAccount)))
	if h.devLogin {
		mux.HandleFunc("POST /auth/dev", h.signInDev)
	}
}

type signInResponse struct {
	Token string    `json:"token"`
	User  user.User `json:"user"`
}

func (h *Handler) signInWithApple(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IdentityToken     string `json:"identity_token"`
		AuthorizationCode string `json:"authorization_code"`
		Nonce             string `json:"nonce"`
		DisplayName       string `json:"display_name"`
	}
	if err := httpapi.DecodeJSON(w, r, &req); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	token, u, err := h.svc.SignInWithApple(r.Context(), SignInInput(req))
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, signInResponse{Token: token, User: u})
}

func (h *Handler) signInDev(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"name"`
		DisplayName string `json:"display_name"`
	}
	if err := httpapi.DecodeJSON(w, r, &req); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	token, u, err := h.svc.SignInDev(r.Context(), req.Name, req.DisplayName)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, signInResponse{Token: token, User: u})
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Logout(r.Context(), bearerToken(r)); err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) deleteAccount(w http.ResponseWriter, r *http.Request) {
	userID, ok := httpapi.UserID(r.Context())
	if !ok {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if err := h.svc.DeleteAccount(r.Context(), userID); err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrInvalidRequest):
		httpapi.WriteError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrInvalidAppleToken):
		// Details (which check failed) go to the log, not the caller.
		slog.InfoContext(r.Context(), "apple identity token rejected", "error", err)
		httpapi.WriteError(w, http.StatusUnauthorized, ErrInvalidAppleToken.Error())
	case errors.Is(err, ErrInvalidGrant):
		httpapi.WriteError(w, http.StatusUnauthorized, ErrInvalidGrant.Error())
	case errors.Is(err, ErrNotConfigured):
		httpapi.WriteError(w, http.StatusServiceUnavailable, err.Error())
	case errors.Is(err, ErrAppleUnavailable):
		slog.ErrorContext(r.Context(), "apple request failed", "error", err)
		httpapi.WriteError(w, http.StatusBadGateway, ErrAppleUnavailable.Error())
	default:
		slog.ErrorContext(r.Context(), "auth request failed", "error", err)
		httpapi.WriteError(w, http.StatusInternalServerError, "internal error")
	}
}

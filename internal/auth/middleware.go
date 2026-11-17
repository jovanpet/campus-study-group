package auth

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/jovanpet/campus-study-group/internal/httpapi"
)

// RequireUser returns middleware that rejects requests without a valid
// "Authorization: Bearer <token>" header and puts the user's ID on the
// request context (read it with httpapi.UserID).
func RequireUser(svc *Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID, err := svc.Authenticate(r.Context(), bearerToken(r))
			switch {
			case errors.Is(err, ErrInvalidToken):
				w.Header().Set("WWW-Authenticate", "Bearer")
				httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized")
			case err != nil:
				slog.ErrorContext(r.Context(), "authenticate", "error", err)
				httpapi.WriteError(w, http.StatusInternalServerError, "internal error")
			default:
				next.ServeHTTP(w, r.WithContext(httpapi.WithUserID(r.Context(), userID)))
			}
		})
	}
}

func bearerToken(r *http.Request) string {
	scheme, token, ok := strings.Cut(r.Header.Get("Authorization"), " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return ""
	}
	return strings.TrimSpace(token)
}

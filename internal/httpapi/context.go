package httpapi

import (
	"context"
	"uuid"
)

type userIDKey struct{}

// WithUserID returns a context carrying the authenticated user's ID. The auth
// middleware sets it; handler tests can set it directly to skip real auth.
func WithUserID(ctx context.Context, id uuid.UUID) context.Context {
	return context.WithValue(ctx, userIDKey{}, id)
}

// UserID returns the authenticated user's ID, if the request passed auth.
func UserID(ctx context.Context) (uuid.UUID, bool) {
	id, ok := ctx.Value(userIDKey{}).(uuid.UUID)
	return id, ok
}

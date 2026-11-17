package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"

	"github.com/jovanpet/campus-study-group/internal/store"
)

const (
	// tokenTTL is how long an API token lasts without being used.
	tokenTTL = 90 * 24 * time.Hour
	// tokenRefreshAfter bounds how often using a token writes a new expiry.
	tokenRefreshAfter = 24 * time.Hour
)

// newToken returns a random API token and the hash to store for it.
func newToken() (plain string, hash []byte) {
	b := make([]byte, 32)
	_, _ = rand.Read(b) // never fails
	plain = base64.RawURLEncoding.EncodeToString(b)
	return plain, hashToken(plain)
}

// hashToken needs no salt or slow hash: tokens are 256 bits of randomness.
func hashToken(plain string) []byte {
	sum := sha256.Sum256([]byte(plain))
	return sum[:]
}

type TokenRepository interface {
	Create(ctx context.Context, userID uuid.UUID, hash []byte, expiresAt time.Time) error
	// Lookup returns the token's user and expiry, or ErrInvalidToken if the
	// token is unknown or expired at now.
	Lookup(ctx context.Context, hash []byte, now time.Time) (uuid.UUID, time.Time, error)
	Extend(ctx context.Context, hash []byte, expiresAt time.Time) error
	Delete(ctx context.Context, hash []byte) error
}

type PostgresTokenRepository struct {
	db store.DB
}

func NewPostgresTokenRepository(db store.DB) *PostgresTokenRepository {
	return &PostgresTokenRepository{db: db}
}

// Create stores the token and clears out the user's expired ones.
func (r *PostgresTokenRepository) Create(ctx context.Context, userID uuid.UUID, hash []byte, expiresAt time.Time) error {
	if _, err := r.db.Exec(ctx, `DELETE FROM auth_tokens WHERE user_id = $1 AND expires_at <= now()`, userID); err != nil {
		return fmt.Errorf("prune tokens: %w", err)
	}
	_, err := r.db.Exec(ctx,
		`INSERT INTO auth_tokens (user_id, token_hash, expires_at) VALUES ($1, $2, $3)`,
		userID, hash, expiresAt)
	if err != nil {
		return fmt.Errorf("create token: %w", err)
	}
	return nil
}

func (r *PostgresTokenRepository) Lookup(ctx context.Context, hash []byte, now time.Time) (uuid.UUID, time.Time, error) {
	var (
		userID    uuid.UUID
		expiresAt time.Time
	)
	err := r.db.QueryRow(ctx,
		`SELECT user_id, expires_at FROM auth_tokens WHERE token_hash = $1 AND expires_at > $2`,
		hash, now,
	).Scan(&userID, &expiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.UUID{}, time.Time{}, ErrInvalidToken
	}
	if err != nil {
		return uuid.UUID{}, time.Time{}, fmt.Errorf("lookup token: %w", err)
	}
	return userID, expiresAt, nil
}

func (r *PostgresTokenRepository) Extend(ctx context.Context, hash []byte, expiresAt time.Time) error {
	if _, err := r.db.Exec(ctx, `UPDATE auth_tokens SET expires_at = $2 WHERE token_hash = $1`, hash, expiresAt); err != nil {
		return fmt.Errorf("extend token: %w", err)
	}
	return nil
}

func (r *PostgresTokenRepository) Delete(ctx context.Context, hash []byte) error {
	if _, err := r.db.Exec(ctx, `DELETE FROM auth_tokens WHERE token_hash = $1`, hash); err != nil {
		return fmt.Errorf("delete token: %w", err)
	}
	return nil
}

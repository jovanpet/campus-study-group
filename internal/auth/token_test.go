package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jovanpet/campus-study-group/internal/store/storetest"
	"github.com/jovanpet/campus-study-group/internal/user"
)

func TestPostgresTokenRepository(t *testing.T) {
	ctx := context.Background()
	tx := storetest.Tx(t)
	users := user.NewPostgresRepository(tx)
	repo := NewPostgresTokenRepository(tx)

	u, err := users.UpsertByAppleSub(ctx, "token-test-sub", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	_, hash := newToken()
	_, expired := newToken()

	if err := repo.Create(ctx, u.ID, expired, now.Add(-time.Hour)); err != nil {
		t.Fatalf("Create expired: %v", err)
	}
	if _, _, err := repo.Lookup(ctx, expired, now); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("expired Lookup: %v", err)
	}
	if err := repo.Create(ctx, u.ID, hash, now.Add(time.Hour)); err != nil {
		t.Fatalf("Create: %v", err)
	}
	var count int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM auth_tokens WHERE user_id = $1`, u.ID).Scan(&count); err != nil || count != 1 {
		t.Errorf("tokens = %d (%v), want 1: Create should prune expired ones", count, err)
	}

	userID, expiresAt, err := repo.Lookup(ctx, hash, now)
	if err != nil || userID != u.ID || expiresAt.Sub(now.Add(time.Hour)).Abs() > time.Second {
		t.Fatalf("Lookup = %v, %v, %v", userID, expiresAt, err)
	}
	if _, _, err := repo.Lookup(ctx, hash, now.Add(2*time.Hour)); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("Lookup past expiry: %v", err)
	}
	if err := repo.Extend(ctx, hash, now.Add(3*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := repo.Lookup(ctx, hash, now.Add(2*time.Hour)); err != nil {
		t.Errorf("Lookup after Extend: %v", err)
	}

	// Deleting the user takes their tokens with it.
	if err := users.Delete(ctx, u.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := repo.Lookup(ctx, hash, now); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("Lookup after user delete: %v", err)
	}

	_, other := newToken()
	if err := repo.Delete(ctx, other); err != nil {
		t.Errorf("Delete unknown token: %v", err)
	}
}

package example

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/jovanpet/campus-study-group/internal/store"
)

// testTx returns a transaction with the schema applied. It is rolled back when
// the test ends, so tests never see each other's rows. Requires DATABASE_URL
// (set by the Postgres service container in CI); skipped otherwise.
func testTx(t *testing.T) pgx.Tx {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set; skipping Postgres test")
	}
	ctx := context.Background()

	pool, err := store.Open(ctx, url)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(pool.Close)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(ctx) })

	schema, err := os.ReadFile("../../migrations/0001_create_example_items.up.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	if _, err := tx.Exec(ctx, string(schema)); err != nil {
		t.Fatalf("apply migration: %v", err)
	}
	return tx
}

func TestPostgresRepository(t *testing.T) {
	ctx := context.Background()
	repo := NewPostgresRepository(testTx(t))

	created, err := repo.Create(ctx, "linear algebra")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID == 0 || created.CreatedAt.IsZero() {
		t.Fatalf("Create returned %+v, want ID and CreatedAt set", created)
	}

	got, err := repo.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "linear algebra" {
		t.Errorf("Get name = %q, want %q", got.Name, "linear algebra")
	}

	if _, err := repo.Get(ctx, created.ID+1000); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get missing: err = %v, want ErrNotFound", err)
	}

	items, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 1 {
		t.Errorf("List returned %d items, want 1", len(items))
	}
}

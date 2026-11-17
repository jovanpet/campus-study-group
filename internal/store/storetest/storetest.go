// Package storetest provides a Postgres transaction for repository tests.
package storetest

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/jovanpet/campus-study-group/internal/store"
)

// migrateLockID serialises migrations across test binaries, which `go test
// ./...` runs in parallel against the same database.
const migrateLockID = 7203114

// Tx returns a transaction on a migrated database. It is rolled back when the
// test ends, so tests never see each other's rows. Requires DATABASE_URL (set
// by the Postgres service container in CI); the test is skipped otherwise.
func Tx(t *testing.T) pgx.Tx {
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

	lock, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire conn: %v", err)
	}
	if _, err := lock.Exec(ctx, `SELECT pg_advisory_lock($1)`, migrateLockID); err != nil {
		lock.Release()
		t.Fatalf("lock migrations: %v", err)
	}
	err = store.Migrate(ctx, pool)
	_, _ = lock.Exec(ctx, `SELECT pg_advisory_unlock($1)`, migrateLockID)
	lock.Release()
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(ctx) })
	return tx
}

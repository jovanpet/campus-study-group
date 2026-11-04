package example

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/jovanpet/campus-study-group/internal/store"
)

type Repository interface {
	Create(ctx context.Context, name string) (Item, error)
	Get(ctx context.Context, id int64) (Item, error)
	List(ctx context.Context) ([]Item, error)
}

type PostgresRepository struct {
	db store.DB
}

func NewPostgresRepository(db store.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) Create(ctx context.Context, name string) (Item, error) {
	var it Item
	err := r.db.QueryRow(ctx,
		`INSERT INTO example_items (name) VALUES ($1) RETURNING id, name, created_at`,
		name,
	).Scan(&it.ID, &it.Name, &it.CreatedAt)
	if err != nil {
		return Item{}, fmt.Errorf("insert example item: %w", err)
	}
	return it, nil
}

func (r *PostgresRepository) Get(ctx context.Context, id int64) (Item, error) {
	var it Item
	err := r.db.QueryRow(ctx,
		`SELECT id, name, created_at FROM example_items WHERE id = $1`,
		id,
	).Scan(&it.ID, &it.Name, &it.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Item{}, ErrNotFound
	}
	if err != nil {
		return Item{}, fmt.Errorf("get example item %d: %w", id, err)
	}
	return it, nil
}

func (r *PostgresRepository) List(ctx context.Context) ([]Item, error) {
	rows, err := r.db.Query(ctx, `SELECT id, name, created_at FROM example_items ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list example items: %w", err)
	}
	items, err := pgx.CollectRows(rows, pgx.RowToStructByPos[Item])
	if err != nil {
		return nil, fmt.Errorf("scan example items: %w", err)
	}
	return items, nil
}

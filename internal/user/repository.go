package user

import (
	"context"
	"errors"
	"fmt"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/jovanpet/campus-study-group/internal/store"
)

type Repository interface {
	// UpsertByAppleSub finds or creates the user for an Apple account. An
	// existing display name is kept; refreshTokenEnc replaces the stored one
	// only when non-nil.
	UpsertByAppleSub(ctx context.Context, appleSub string, displayName *string, refreshTokenEnc []byte) (User, error)
	Get(ctx context.Context, id uuid.UUID) (User, error)
	CompleteProfile(ctx context.Context, id uuid.UUID, p Profile) (User, error)
	// AppleRefreshToken returns the encrypted Apple refresh token, nil if none.
	AppleRefreshToken(ctx context.Context, id uuid.UUID) ([]byte, error)
	Delete(ctx context.Context, id uuid.UUID) error
	ListSchools(ctx context.Context) ([]School, error)
	ListMajors(ctx context.Context) ([]Major, error)
}

type PostgresRepository struct {
	db store.DB
}

func NewPostgresRepository(db store.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) UpsertByAppleSub(ctx context.Context, appleSub string, displayName *string, refreshTokenEnc []byte) (User, error) {
	var id uuid.UUID
	err := r.db.QueryRow(ctx, `
		INSERT INTO users (apple_sub, display_name, apple_refresh_token_enc)
		VALUES ($1, $2, $3)
		ON CONFLICT (apple_sub) DO UPDATE SET
			display_name = COALESCE(users.display_name, EXCLUDED.display_name),
			apple_refresh_token_enc = COALESCE(EXCLUDED.apple_refresh_token_enc, users.apple_refresh_token_enc)
		RETURNING id`,
		appleSub, displayName, refreshTokenEnc,
	).Scan(&id)
	if err != nil {
		return User{}, fmt.Errorf("upsert user: %w", err)
	}
	return r.Get(ctx, id)
}

func (r *PostgresRepository) Get(ctx context.Context, id uuid.UUID) (User, error) {
	var (
		u                     User
		schoolID, majorID     *uuid.UUID
		schoolName, majorName *string
	)
	// username (citext) and degree (enum) are cast because pgx has no codec
	// registered for extension and user-defined types.
	err := r.db.QueryRow(ctx, `
		SELECT u.id, u.apple_sub, u.username::text, u.display_name, u.degree::text,
		       s.id, s.name, m.id, m.name
		FROM users u
		LEFT JOIN schools s ON s.id = u.school_id
		LEFT JOIN majors m ON m.id = u.major_id
		WHERE u.id = $1`, id,
	).Scan(&u.ID, &u.AppleSub, &u.UserName, &u.DisplayName, &u.Degree,
		&schoolID, &schoolName, &majorID, &majorName)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("get user: %w", err)
	}
	if schoolID != nil && schoolName != nil {
		u.School = &School{ID: *schoolID, Name: *schoolName}
	}
	if majorID != nil && majorName != nil {
		u.Major = &Major{ID: *majorID, Name: *majorName}
	}
	return u, nil
}

func (r *PostgresRepository) CompleteProfile(ctx context.Context, id uuid.UUID, p Profile) (User, error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE users
		SET username = $2, display_name = $3, school_id = $4, major_id = $5, degree = $6::degree
		WHERE id = $1`,
		id, p.Username, p.DisplayName, p.SchoolID, p.MajorID, string(p.Degree),
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			switch pgErr.Code {
			case "23505": // unique_violation
				return User{}, ErrUsernameTaken
			case "23503": // foreign_key_violation
				return User{}, fmt.Errorf("%w: unknown school or major", ErrInvalid)
			}
		}
		return User{}, fmt.Errorf("complete profile: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return User{}, ErrNotFound
	}
	return r.Get(ctx, id)
}

func (r *PostgresRepository) AppleRefreshToken(ctx context.Context, id uuid.UUID) ([]byte, error) {
	var enc []byte
	err := r.db.QueryRow(ctx, `SELECT apple_refresh_token_enc FROM users WHERE id = $1`, id).Scan(&enc)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get apple refresh token: %w", err)
	}
	return enc, nil
}

func (r *PostgresRepository) Delete(ctx context.Context, id uuid.UUID) error {
	if _, err := r.db.Exec(ctx, `DELETE FROM users WHERE id = $1`, id); err != nil {
		return fmt.Errorf("delete user: %w", err)
	}
	return nil
}

func (r *PostgresRepository) ListSchools(ctx context.Context) ([]School, error) {
	rows, err := r.db.Query(ctx, `SELECT id, name FROM schools ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list schools: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (School, error) {
		var s School
		return s, row.Scan(&s.ID, &s.Name)
	})
}

func (r *PostgresRepository) ListMajors(ctx context.Context) ([]Major, error) {
	rows, err := r.db.Query(ctx, `SELECT id, name FROM majors ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list majors: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Major, error) {
		var m Major
		return m, row.Scan(&m.ID, &m.Name)
	})
}

package user

import (
	"context"
	"errors"
	"fmt"
	"uuid"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"
	"github.com/jovanpet/campus-study-group/internal/store"
)

type Repository interface {
	UpsertByAppleSub(ctx context.Context, appleSub string, refreshToken, displayName *string) (User, error)
	Get(ctx context.Context, id uuid.UUID) (User, error)
	List(ctx context.Context) ([]User, error)
}

type UserRepository struct{ db store.DB }

func NewPostgresRepository(db store.DB) *UserRepository { return &UserRepository{db: db} }

// UpsertByAppleSub finds or creates the user for an Apple sign-in. Apple only
// sends the name on the first sign-in, so an existing display name is kept,
// and a nil refresh token leaves the stored one in place.
func (r *UserRepository) UpsertByAppleSub(ctx context.Context, appleSub string, refreshToken, displayName *string) (User, error) {
	sql, args, err := sq.Insert("users").
		Columns("apple_sub", "apple_refresh_token", "display_name").
		Values(appleSub, refreshToken, displayName).
		Suffix(`ON CONFLICT (apple_sub) DO UPDATE SET
			apple_refresh_token = COALESCE(EXCLUDED.apple_refresh_token, users.apple_refresh_token),
			display_name = COALESCE(users.display_name, EXCLUDED.display_name)
			RETURNING id`).
		PlaceholderFormat(sq.Dollar).
		ToSql()
	if err != nil {
		return User{}, fmt.Errorf("build upsert user: %w", err)
	}
	var id uuid.UUID
	if err := r.db.QueryRow(ctx, sql, args...).Scan(&id); err != nil {
		return User{}, fmt.Errorf("upsert user: %w", err)
	}
	return r.Get(ctx, id)
}

func (r *UserRepository) Get(ctx context.Context, id uuid.UUID) (User, error) {
	sql, args, err := sq.Select(
		"u.id",
		"u.apple_sub",
		"u.apple_refresh_token",
		"u.username",
		"u.display_name",
		"u.degree::text",
		"s.id",
		"s.name",
		"m.id",
		"m.name",
	).
		From("users u").
		LeftJoin("schools s ON s.id = u.school_id").
		LeftJoin("majors m ON m.id = u.major_id").
		// Not sq.Eq: squirrel expands array types like uuid.UUID into IN (...).
		Where("u.id = ?", id).
		PlaceholderFormat(sq.Dollar).
		ToSql()
	if err != nil {
		return User{}, fmt.Errorf("build get user: %w", err)
	}

	var (
		user                  User
		schoolID, majorID     *uuid.UUID
		schoolName, majorName *string
	)
	err = r.db.QueryRow(ctx, sql, args...).Scan(
		&user.ID,
		&user.AppleSub,
		&user.AppleRefreshToken,
		&user.UserName,
		&user.DisplayName,
		&user.Degree,
		&schoolID,
		&schoolName,
		&majorID,
		&majorName,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("get user %s: %w", id, err)
	}
	if schoolID != nil {
		user.School = &School{ID: *schoolID, Name: *schoolName}
	}
	if majorID != nil {
		user.Major = &Major{ID: *majorID, Name: *majorName}
	}
	return user, nil
}

func (r *UserRepository) List(ctx context.Context) ([]User, error) {
	return []User{}, nil
}

type SchoolRepository struct{ db store.DB }

func NewSchoolRepository(db store.DB) *SchoolRepository { return &SchoolRepository{db: db} }

func (r *SchoolRepository) Get(ctx context.Context, id uuid.UUID) (School, error) {
	sql, args, err := sq.Select("id", "name").
		From("schools").
		Where("id = ?", id).
		PlaceholderFormat(sq.Dollar).
		ToSql()
	if err != nil {
		return School{}, fmt.Errorf("build get school: %w", err)
	}

	var school School
	err = r.db.QueryRow(ctx, sql, args...).Scan(&school.ID, &school.Name)
	if errors.Is(err, pgx.ErrNoRows) {
		return School{}, ErrNotFound
	}
	if err != nil {
		return School{}, fmt.Errorf("get school %s: %w", id, err)
	}
	return school, nil
}

func (r *SchoolRepository) List(ctx context.Context) ([]School, error) {

}

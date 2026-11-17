package user

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
	"uuid"
)

const maxDisplayNameLen = 50

var usernameRE = regexp.MustCompile(`^[a-z0-9_.]{3,20}$`)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (User, error) {
	return s.repo.Get(ctx, id)
}

// CompleteProfile validates and saves the profile. Setting the username is
// what marks the user as onboarded; calling it again edits the profile.
func (s *Service) CompleteProfile(ctx context.Context, id uuid.UUID, p Profile) (User, error) {
	p.Username = strings.ToLower(strings.TrimSpace(p.Username))
	p.DisplayName = strings.TrimSpace(p.DisplayName)

	switch {
	case !usernameRE.MatchString(p.Username):
		return User{}, fmt.Errorf("%w: username must be 3-20 characters from a-z, 0-9, underscore and dot", ErrInvalid)
	case p.DisplayName == "" || utf8.RuneCountInString(p.DisplayName) > maxDisplayNameLen:
		return User{}, fmt.Errorf("%w: display_name must be 1-%d characters", ErrInvalid, maxDisplayNameLen)
	case !p.Degree.valid():
		return User{}, fmt.Errorf("%w: degree must be %q, %q or %q", ErrInvalid, Bachelor, Master, PhD)
	case p.SchoolID == uuid.Nil() || p.MajorID == uuid.Nil():
		return User{}, fmt.Errorf("%w: school_id and major_id are required", ErrInvalid)
	}
	return s.repo.CompleteProfile(ctx, id, p)
}

func (s *Service) ListSchools(ctx context.Context) ([]School, error) {
	return s.repo.ListSchools(ctx)
}

func (s *Service) ListMajors(ctx context.Context) ([]Major, error) {
	return s.repo.ListMajors(ctx)
}

// Package user manages user accounts and profiles.
// Follow the layout in internal/example (repository, service, handler, tests).
package user

import (
	"errors"
	"uuid"
)

// User is created at first Apple sign-in; the profile fields stay nil until
// onboarding fills them in.
type User struct {
	ID                uuid.UUID `json:"id"`
	AppleSub          string    `json:"-"`            // stable Apple user identifier
	AppleRefreshToken *string   `json:"-"`            // Apple refresh token; nil when not stored
	UserName          *string   `json:"username"`     // handle for the user; nil until onboarded
	DisplayName       *string   `json:"display_name"` // full name for the user
	Degree            *Degree   `json:"degree"`
	School            *School   `json:"school,omitempty"`
	Major             *Major    `json:"major,omitempty"`
}

// IsOnboarded reports whether the user has completed onboarding. The schema
// only allows a username once the rest of the profile is set.
func (u User) IsOnboarded() bool { return u.UserName != nil }

type School struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

type Major struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

type Degree string

const (
	Bachelor Degree = "undergraduate"
	Master   Degree = "graduate"
	PhD      Degree = "phd"
)

var (
	ErrNotFound = errors.New("user not found")
	ErrInvalid  = errors.New("invalid user")
)

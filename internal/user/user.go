// Package user manages user accounts and profiles.
package user

import (
	"encoding/json"
	"errors"
	"uuid"
)

// User is created at first Apple sign-in; the profile fields stay nil until
// onboarding fills them in.
type User struct {
	ID          uuid.UUID `json:"id"`
	AppleSub    string    `json:"-"`            // stable Apple user identifier
	UserName    *string   `json:"username"`     // handle for the user; nil until onboarded
	DisplayName *string   `json:"display_name"` // full name for the user
	Degree      *Degree   `json:"degree"`
	School      *School   `json:"school"`
	Major       *Major    `json:"major"`
}

// IsOnboarded reports whether the user has completed onboarding. The schema
// only allows a username once the rest of the profile is set.
func (u User) IsOnboarded() bool { return u.UserName != nil }

// MarshalJSON adds "onboarded" so clients don't have to infer it from username.
func (u User) MarshalJSON() ([]byte, error) {
	type plain User
	return json.Marshal(struct {
		plain
		Onboarded bool `json:"onboarded"`
	}{plain(u), u.IsOnboarded()})
}

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

func (d Degree) valid() bool { return d == Bachelor || d == Master || d == PhD }

// Profile is what onboarding collects.
type Profile struct {
	Username    string
	DisplayName string
	SchoolID    uuid.UUID
	MajorID     uuid.UUID
	Degree      Degree
}

var (
	ErrNotFound      = errors.New("user not found")
	ErrInvalid       = errors.New("invalid user")
	ErrUsernameTaken = errors.New("username is taken")
)

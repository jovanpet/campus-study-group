// Package user manages user accounts and profiles.
// Follow the layout in internal/example (repository, service, handler, tests).
package user

import "time"

type User struct {
	ID          int64     `json:"id"`
	AppleSub    string    `json:"-"` // stable Apple user identifier
	Email       string    `json:"email,omitempty"`
	DisplayName string    `json:"display_name"`
	CreatedAt   time.Time `json:"created_at"`
}

// TODO: Repository, PostgresRepository, Service, Handler.

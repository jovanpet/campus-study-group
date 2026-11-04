// Package friend manages friend requests and friendships between users.
// Follow the layout in internal/example (repository, service, handler, tests).
package friend

import "time"

type Status string

const (
	StatusPending  Status = "pending"
	StatusAccepted Status = "accepted"
)

type Friendship struct {
	RequesterID int64     `json:"requester_id"`
	AddresseeID int64     `json:"addressee_id"`
	Status      Status    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
}

// TODO: Repository, PostgresRepository, Service, Handler.

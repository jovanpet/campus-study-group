// Package session manages study sessions: scheduling, joining and attendance.
// (API auth sessions live in internal/auth.)
// Follow the layout in internal/example (repository, service, handler, tests).
package session

import "time"

type StudySession struct {
	ID        int64     `json:"id"`
	HostID    int64     `json:"host_id"`
	Title     string    `json:"title"`
	Location  string    `json:"location"`
	StartsAt  time.Time `json:"starts_at"`
	EndsAt    time.Time `json:"ends_at"`
	CreatedAt time.Time `json:"created_at"`
}

// TODO: Repository, PostgresRepository, Service, Handler.

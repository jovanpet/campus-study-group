// Package example is a reference slice showing how a domain package is laid
// out. Copy its shape when filling in user, friend, session, etc.:
//
//	example.go        model + domain errors
//	repository.go     Repository interface + Postgres implementation (store.DB)
//	service.go        business rules; depends only on Repository
//	handler.go        HTTP routes; depends only on Service
//	*_test.go         service/handler tests use a fake repo; repository tests hit Postgres
package example

import (
	"errors"
	"time"
)

var (
	ErrNotFound = errors.New("item not found")
	ErrInvalid  = errors.New("invalid item")
)

type Item struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

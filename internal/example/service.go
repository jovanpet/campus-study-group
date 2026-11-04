package example

import (
	"context"
	"fmt"
	"strings"
)

const maxNameLen = 100

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Create(ctx context.Context, name string) (Item, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > maxNameLen {
		return Item{}, fmt.Errorf("%w: name must be 1-%d characters", ErrInvalid, maxNameLen)
	}
	return s.repo.Create(ctx, name)
}

func (s *Service) Get(ctx context.Context, id int64) (Item, error) {
	return s.repo.Get(ctx, id)
}

func (s *Service) List(ctx context.Context) ([]Item, error) {
	return s.repo.List(ctx)
}

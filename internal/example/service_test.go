package example

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// fakeRepo is an in-memory Repository for testing the service and handler
// without a database.
type fakeRepo struct {
	items []Item
}

func (f *fakeRepo) Create(_ context.Context, name string) (Item, error) {
	it := Item{ID: int64(len(f.items) + 1), Name: name, CreatedAt: time.Now()}
	f.items = append(f.items, it)
	return it, nil
}

func (f *fakeRepo) Get(_ context.Context, id int64) (Item, error) {
	for _, it := range f.items {
		if it.ID == id {
			return it, nil
		}
	}
	return Item{}, ErrNotFound
}

func (f *fakeRepo) List(_ context.Context) ([]Item, error) {
	return f.items, nil
}

func TestServiceCreate(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr error
	}{
		{name: "trims whitespace", input: "  algebra  ", want: "algebra"},
		{name: "rejects empty", input: "   ", wantErr: ErrInvalid},
		{name: "rejects too long", input: strings.Repeat("a", maxNameLen+1), wantErr: ErrInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewService(&fakeRepo{})
			got, err := svc.Create(context.Background(), tt.input)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if got.Name != tt.want {
				t.Errorf("Name = %q, want %q", got.Name, tt.want)
			}
		})
	}
}

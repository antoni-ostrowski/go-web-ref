package todo

import (
	"context"
	"fmt"
	"strings"
)

// Todo is domain model. No HTTP, no HTML.
type Todo struct {
	ID    string
	Title string
	Done  bool
}

// Repository is PORT — defined by domain, implemented by global db.Repo.
// Service depends on interface, not concrete *db.Repo, so tests can pass any fake.
type Repository interface {
	List(ctx context.Context) ([]Todo, error)
	Save(ctx context.Context, t Todo) (Todo, error)
	Get(ctx context.Context, id string) (Todo, error)
	Update(ctx context.Context, t Todo) error
	Delete(ctx context.Context, id string) error
}

// Service holds business rules. Pointer, holds Repository interface.
// No net/http, no templ — pure Go, testable with fake Repo.
type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// Pure funcs — isolated logic, test without DB/repo.
// Handler or Service can call them. No side effects.

func ValidateTitle(title string) error {
	if strings.TrimSpace(title) == "" {
		return fmt.Errorf("title cannot be empty")
	}
	return nil
}

// List returns domain data, not VM. Handler builds VM.
func (s *Service) List(ctx context.Context) ([]Todo, error) {
	return s.repo.List(ctx)
}

// Add validates, creates domain object, persists, returns fresh list.
func (s *Service) Add(ctx context.Context, title string) ([]Todo, error) {
	if err := ValidateTitle(title); err != nil {
		return nil, err
	}
	t := Todo{Title: strings.TrimSpace(title)}
	if _, err := s.repo.Save(ctx, t); err != nil {
		return nil, fmt.Errorf("save todo: %w", err)
	}
	return s.repo.List(ctx)
}

// Toggle flips Done. Returns fresh list.
func (s *Service) Toggle(ctx context.Context, id string) ([]Todo, error) {
	t, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	t.Done = !t.Done
	if err := s.repo.Update(ctx, t); err != nil {
		return nil, err
	}
	return s.repo.List(ctx)
}

// Delete removes by ID. Returns fresh list.
func (s *Service) Delete(ctx context.Context, id string) ([]Todo, error) {
	if err := s.repo.Delete(ctx, id); err != nil {
		return nil, err
	}
	return s.repo.List(ctx)
}

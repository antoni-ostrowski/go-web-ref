package todo

import (
	"context"
	"errors"
	"fmt"
	"strings"

	db "go-htmx-todo/internal/db/sqlc"

	"github.com/jackc/pgx/v5"
)

// TodoStore is the persistence seam for the todo domain.
//
// The service depends on this narrow interface instead of on the concrete
// *db.Queries so tests can substitute an in-memory fake and run the whole
// behavior suite with no database. sqlc's generated *db.Queries satisfies
// this interface exactly, so production wiring stays NewService(db.New(pool))
// — no adapter package needed.
//
// Keep the interface narrow: only what this domain actually calls. A new
// domain gets its own store interface rather than a growing global one.
type TodoStore interface {
	ListTodos(ctx context.Context) ([]db.Todo, error)
	CreateTodo(ctx context.Context, title string) (db.Todo, error)
	GetTodo(ctx context.Context, id int64) (db.Todo, error)
	UpdateTodo(ctx context.Context, arg db.UpdateTodoParams) error
	DeleteTodo(ctx context.Context, id int64) error
}

// Compile-time check: if sqlc's generated queries stop satisfying the todo
// contract, this package stops building — drift is caught here, not in tests.
var _ TodoStore = (*db.Queries)(nil)

// Sentinel errors: services return these so callers (handlers, CLIs, other
// services) can decide what to do without string-matching messages.
var (
	ErrValidation = errors.New("todo: validation failed")
	ErrNotFound   = errors.New("todo: not found")
)

// Service holds todo rules and the TodoStore seam. The service returns domain
// values ([]db.Todo), never ViewModels; handlers shape display types.
type Service struct {
	store TodoStore
}

func NewService(store TodoStore) *Service {
	return &Service{store: store}
}

func ValidateTitle(title string) error {
	if strings.TrimSpace(title) == "" {
		return fmt.Errorf("%w: title cannot be empty", ErrValidation)
	}
	return nil
}

func (s *Service) List(ctx context.Context) ([]db.Todo, error) {
	todos, err := s.store.ListTodos(ctx)
	if err != nil {
		return nil, fmt.Errorf("list todos: %w", err)
	}
	return todos, nil
}

func (s *Service) Add(ctx context.Context, title string) ([]db.Todo, error) {
	if err := ValidateTitle(title); err != nil {
		return nil, err
	}
	if _, err := s.store.CreateTodo(ctx, strings.TrimSpace(title)); err != nil {
		return nil, fmt.Errorf("save todo: %w", err)
	}
	return s.List(ctx)
}

func (s *Service) Toggle(ctx context.Context, id int64) ([]db.Todo, error) {
	t, err := s.store.GetTodo(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("%w: id %d", ErrNotFound, id)
		}
		return nil, fmt.Errorf("get todo: %w", err)
	}
	t.Done = !t.Done
	if err := s.store.UpdateTodo(ctx, db.UpdateTodoParams{ID: t.ID, Title: t.Title, Done: t.Done}); err != nil {
		return nil, fmt.Errorf("update todo: %w", err)
	}
	return s.List(ctx)
}

func (s *Service) Delete(ctx context.Context, id int64) ([]db.Todo, error) {
	if err := s.store.DeleteTodo(ctx, id); err != nil {
		return nil, fmt.Errorf("delete todo: %w", err)
	}
	return s.List(ctx)
}

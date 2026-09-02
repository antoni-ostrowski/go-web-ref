package todo

import (
	"context"
	"fmt"
	"strings"

	db "go-htmx-todo/internal/db/sqlc"
)

// Service holds todo rules and generated sqlc queries. It is HTMX-oriented:
// methods return the ViewModel that the corresponding component renders.
type Service struct {
	queries *db.Queries
}

func NewService(queries *db.Queries) *Service {
	return &Service{queries: queries}
}

func ValidateTitle(title string) error {
	if strings.TrimSpace(title) == "" {
		return fmt.Errorf("title cannot be empty")
	}
	return nil
}

func (s *Service) List(ctx context.Context) (PageVM, error) {
	todos, err := s.queries.ListTodos(ctx)
	if err != nil {
		return PageVM{}, fmt.Errorf("list todos: %w", err)
	}
	return PageVM{Todos: todos}, nil
}

func (s *Service) Add(ctx context.Context, title string) (ListVM, error) {
	if err := ValidateTitle(title); err != nil {
		return ListVM{}, err
	}
	if _, err := s.queries.CreateTodo(ctx, strings.TrimSpace(title)); err != nil {
		return ListVM{}, fmt.Errorf("save todo: %w", err)
	}
	todos, err := s.queries.ListTodos(ctx)
	if err != nil {
		return ListVM{}, fmt.Errorf("list todos after add: %w", err)
	}
	return ListVM{Todos: todos}, nil
}

func (s *Service) Toggle(ctx context.Context, id int64) (ListVM, error) {
	t, err := s.queries.GetTodo(ctx, id)
	if err != nil {
		return ListVM{}, err
	}
	t.Done = !t.Done
	if err := s.queries.UpdateTodo(ctx, db.UpdateTodoParams{ID: t.ID, Title: t.Title, Done: t.Done}); err != nil {
		return ListVM{}, err
	}
	todos, err := s.queries.ListTodos(ctx)
	if err != nil {
		return ListVM{}, fmt.Errorf("list todos after toggle: %w", err)
	}
	return ListVM{Todos: todos}, nil
}

func (s *Service) Delete(ctx context.Context, id int64) (ListVM, error) {
	if err := s.queries.DeleteTodo(ctx, id); err != nil {
		return ListVM{}, err
	}
	todos, err := s.queries.ListTodos(ctx)
	if err != nil {
		return ListVM{}, fmt.Errorf("list todos after delete: %w", err)
	}
	return ListVM{Todos: todos}, nil
}

package db

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"go-htmx-todo/internal/todo"
)

// Repo is the GLOBAL single DB repo — pointer, shared across all handlers/domains.
// Holds all tables/slices. One instance for whole app, passed to every Service.
// For now only todos. Add users, projects etc as new fields + methods here.
// No per-request state — only pool/slices + mutex, so sharing *Repo is safe.
// Pointer receiver => only *Repo implements todo.Repository, not Repo value.
type Repo struct {
	mu     sync.Mutex
	todos  []todo.Todo
	nextID atomic.Int64
}

func New() *Repo { return &Repo{} }

// --- todo.Repository implementation ---

func (r *Repo) List(_ context.Context) ([]todo.Todo, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]todo.Todo, len(r.todos))
	copy(out, r.todos)
	return out, nil
}

func (r *Repo) Save(_ context.Context, t todo.Todo) (todo.Todo, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if t.ID == "" {
		t.ID = fmt.Sprintf("%d", r.nextID.Add(1))
	}
	r.todos = append(r.todos, t)
	return t, nil
}

func (r *Repo) Get(_ context.Context, id string) (todo.Todo, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, t := range r.todos {
		if t.ID == id {
			return t, nil
		}
	}
	return todo.Todo{}, fmt.Errorf("todo %s not found", id)
}

func (r *Repo) Update(_ context.Context, t todo.Todo) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.todos {
		if r.todos[i].ID == t.ID {
			r.todos[i] = t
			return nil
		}
	}
	return fmt.Errorf("todo %s not found", t.ID)
}

func (r *Repo) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, t := range r.todos {
		if t.ID == id {
			r.todos = append(r.todos[:i], r.todos[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("todo %s not found", id)
}

// WithTx placeholder for real DB — with in-memory it's just fn call.
// When you switch to pgx, this will do pool.Begin() / Commit / Rollback.
func (r *Repo) WithTx(ctx context.Context, fn func(*Repo) error) error {
	return fn(r)
}

// compile-time check: *Repo implements todo.Repository
var _ todo.Repository = (*Repo)(nil)

// Package todotest holds the shared test double for the todo domain.
// Import it from test files only — never from production code.
//
// It is a regular package (not a _test.go file) because Go test files
// cannot be imported: one fake here is shared by every package's tests —
// the todo service's, the actions', and any future cross-domain action —
// instead of being redeclared per test file. Consequence: tests that use
// it must be black-box (package todo_test), since a package's own test
// binary cannot import anything that imports the package under test.
package todotest

import (
	"context"
	"sort"
	"sync"
	"testing"

	db "go-htmx-todo/internal/db/sqlc"
	"go-htmx-todo/internal/todo"

	"github.com/jackc/pgx/v5"
)

// FakeTodos is an in-memory todo.TodoStore. It mirrors the observable
// behavior of sqlc's generated methods (CreateTodo returns a Todo,
// DeleteTodo on a missing row is a no-op, GetTodo on a missing row is
// pgx.ErrNoRows), so the interface the service depends on can be
// substituted in tests: no database, no Docker, no environment variables.
// The only thing it cannot prove is the SQL itself — the real-PostgreSQL
// integration test covers that.
type FakeTodos struct {
	mu        sync.Mutex
	next      int64
	todos     map[int64]db.Todo
	UpdateErr error // when set, UpdateTodo returns it (failure injection)
}

// New returns an empty FakeTodos.
func New() *FakeTodos {
	return &FakeTodos{todos: map[int64]db.Todo{}}
}

// Seed creates one todo per title.
func (f *FakeTodos) Seed(t *testing.T, titles ...string) {
	t.Helper()
	for _, title := range titles {
		if _, err := f.CreateTodo(context.Background(), title); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
}

// SetDone flips an existing todo's done flag (test convenience).
func (f *FakeTodos) SetDone(id int64, done bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.todos[id]
	if !ok {
		return
	}
	t.Done = done
	f.todos[id] = t
}

func (f *FakeTodos) ListTodos(context.Context) ([]db.Todo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]db.Todo, 0, len(f.todos))
	for _, t := range f.todos {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (f *FakeTodos) CreateTodo(_ context.Context, title string) (db.Todo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.next++
	t := db.Todo{ID: f.next, Title: title, Done: false}
	f.todos[t.ID] = t
	return t, nil
}

func (f *FakeTodos) GetTodo(_ context.Context, id int64) (db.Todo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.todos[id]
	if !ok {
		return db.Todo{}, pgx.ErrNoRows
	}
	return t, nil
}

func (f *FakeTodos) UpdateTodo(_ context.Context, arg db.UpdateTodoParams) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.UpdateErr != nil {
		return f.UpdateErr
	}
	t, ok := f.todos[arg.ID]
	if !ok {
		return pgx.ErrNoRows
	}
	t.Title, t.Done = arg.Title, arg.Done
	f.todos[t.ID] = t
	return nil
}

func (f *FakeTodos) DeleteTodo(_ context.Context, id int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.todos, id) // sqlc DeleteTodo is a no-op on missing rows; mirror that
	return nil
}

// Compile-time check: the fake stays in lockstep with the real seam.
var _ todo.TodoStore = (*FakeTodos)(nil)
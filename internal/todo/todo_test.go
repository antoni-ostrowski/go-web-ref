package todo

import (
	"context"
	"fmt"
	"sync"
	"testing"
)

// fakeRepo is in-memory fake for tests — implements Repository without importing db.
// Keeps tests fast and avoids import cycle (db imports todo).
type fakeRepo struct {
	mu    sync.Mutex
	todos []Todo
	next  int
}

func newFakeRepo() *fakeRepo { return &fakeRepo{} }

func (r *fakeRepo) List(_ context.Context) ([]Todo, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Todo, len(r.todos))
	copy(out, r.todos)
	return out, nil
}
func (r *fakeRepo) Save(_ context.Context, t Todo) (Todo, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if t.ID == "" {
		r.next++
		t.ID = fmt.Sprintf("%d", r.next)
	}
	r.todos = append(r.todos, t)
	return t, nil
}
func (r *fakeRepo) Get(_ context.Context, id string) (Todo, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, t := range r.todos {
		if t.ID == id {
			return t, nil
		}
	}
	return Todo{}, fmt.Errorf("todo %s not found", id)
}
func (r *fakeRepo) Update(_ context.Context, t Todo) error {
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
func (r *fakeRepo) Delete(_ context.Context, id string) error {
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

func TestValidateTitle(t *testing.T) {
	if err := ValidateTitle("  hello "); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := ValidateTitle("   "); err == nil {
		t.Fatal("expected error for empty title")
	}
}

func TestService_Add_List_Toggle_Delete(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)
	ctx := context.Background()

	todos, err := svc.Add(ctx, "buy milk")
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	if len(todos) != 1 || todos[0].Title != "buy milk" {
		t.Fatalf("unexpected todos: %+v", todos)
	}
	id := todos[0].ID

	if _, err := svc.Add(ctx, "   "); err == nil {
		t.Fatal("expected validation error")
	}

	todos, _ = svc.List(ctx)
	if len(todos) != 1 {
		t.Fatalf("expected 1, got %d", len(todos))
	}

	todos, err = svc.Toggle(ctx, id)
	if err != nil {
		t.Fatalf("Toggle failed: %v", err)
	}
	if !todos[0].Done {
		t.Fatal("expected done")
	}

	todos, err = svc.Delete(ctx, id)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	if len(todos) != 0 {
		t.Fatalf("expected 0, got %d", len(todos))
	}
}

func TestService_Toggle_NotFound(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)
	_, err := svc.Toggle(context.Background(), "999")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestHandlerBuildsVM(t *testing.T) {
	todos := []Todo{{ID: "1", Title: "a"}, {ID: "2", Title: "b", Done: true}}
	vm := ListVM{Todos: todos}
	if len(vm.Todos) != 2 {
		t.Fatal("VM should hold domain slice")
	}
	page := PageVM{Todos: todos}
	if len(page.Todos) != 2 {
		t.Fatal("PageVM wrong")
	}
}

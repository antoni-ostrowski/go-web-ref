// The fake TodoStore lives in internal/todo/todotest so every package's
// tests share one implementation; see that package's doc comment for why
// these tests are black-box (package todo_test) rather than white-box.
package todo_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	db "go-htmx-todo/internal/db/sqlc"
	"go-htmx-todo/internal/todo"
	"go-htmx-todo/internal/todo/todotest"
)

func TestValidateTitle(t *testing.T) {
	for _, tt := range []struct {
		in      string
		wantErr bool
	}{
		{"buy milk", false},
		{"", true},
		{"   ", true},
		{"\t\n", true},
	} {
		err := todo.ValidateTitle(tt.in)
		if tt.wantErr && !errors.Is(err, todo.ErrValidation) {
			t.Errorf("ValidateTitle(%q) err=%v, want ErrValidation", tt.in, err)
		}
		if !tt.wantErr && err != nil {
			t.Errorf("ValidateTitle(%q) err=%v, want nil", tt.in, err)
		}
	}
}

func TestListEmpty(t *testing.T) {
	todos, err := todo.NewService(todotest.New()).List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(todos) != 0 {
		t.Fatalf("want empty list, got %#v", todos)
	}
}

func TestAddTrimsAndCreates(t *testing.T) {
	svc := todo.NewService(todotest.New())
	todos, err := svc.Add(context.Background(), "  buy milk  ")
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if len(todos) != 1 || todos[0].Title != "buy milk" || todos[0].Done {
		t.Fatalf("Add returned %#v", todos)
	}
}

func TestAddRejectsBlankTitle(t *testing.T) {
	svc := todo.NewService(todotest.New())
	if _, err := svc.Add(context.Background(), "   "); !errors.Is(err, todo.ErrValidation) {
		t.Fatalf("want ErrValidation, got %v", err)
	}
}

func TestToggleFlipsDone(t *testing.T) {
	f := todotest.New()
	f.Seed(t, "a", "b")
	svc := todo.NewService(f)

	todos, err := svc.Toggle(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(todos) != 2 || todos[0].ID != 1 || !todos[0].Done || todos[1].Done {
		t.Fatalf("toggle result %#v", todos)
	}
}

func TestToggleNotFound(t *testing.T) {
	f := todotest.New()
	f.Seed(t, "a")
	svc := todo.NewService(f)

	if _, err := svc.Toggle(context.Background(), 99); !errors.Is(err, todo.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestDeleteRemovesTodo(t *testing.T) {
	f := todotest.New()
	f.Seed(t, "a", "b")
	svc := todo.NewService(f)

	todos, err := svc.Delete(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(todos) != 1 || todos[0].ID != 2 {
		t.Fatalf("delete result %#v", todos)
	}
}

// failStore proves the service wraps persistence errors without needing a DB.
type failStore struct {
	todo.TodoStore
}

func (failStore) ListTodos(context.Context) ([]db.Todo, error) {
	return nil, errors.New("db down")
}

func TestPersistenceErrorsAreWrapped(t *testing.T) {
	svc := todo.NewService(failStore{})
	if _, err := svc.List(context.Background()); err == nil || !strings.Contains(err.Error(), "list todos") {
		t.Fatalf("want wrapped error mentioning operation, got %v", err)
	}
}


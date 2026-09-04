package actions_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"go-htmx-todo/internal/actions"
	"go-htmx-todo/internal/testutil"
	"go-htmx-todo/internal/todo"
	"go-htmx-todo/internal/todo/todotest"
)

func TestCompleteAll(t *testing.T) {
	ctx := context.Background()

	t.Run("completes every open todo", func(t *testing.T) {
		f := todotest.New()
		f.Seed(t, "a", "b")
		f.SetDone(2, true) // b is already done

		out, err := actions.CompleteAll(ctx, todo.NewService(f))
		if err != nil {
			t.Fatal(err)
		}
		if len(out) != 2 || !out[0].Done || !out[1].Done {
			t.Fatalf("got %#v", out)
		}
	})

	t.Run("empty list is a no-op", func(t *testing.T) {
		out, err := actions.CompleteAll(ctx, todo.NewService(todotest.New()))
		if err != nil {
			t.Fatal(err)
		}
		if len(out) != 0 {
			t.Fatalf("got %#v", out)
		}
	})

	t.Run("store failure surfaces the failing todo id", func(t *testing.T) {
		f := todotest.New()
		f.Seed(t, "a")
		f.UpdateErr = errors.New("db down")

		_, err := actions.CompleteAll(ctx, todo.NewService(f))
		if err == nil || !errors.Is(err, f.UpdateErr) || !strings.Contains(err.Error(), "todo 1") {
			t.Fatalf("want wrapped db-down error naming todo 1, got %v", err)
		}
	})
}

func TestClearCompleted(t *testing.T) {
	ctx := context.Background()

	t.Run("deletes only done todos", func(t *testing.T) {
		f := todotest.New()
		f.Seed(t, "a", "b", "c")
		f.SetDone(1, true)
		f.SetDone(3, true)

		out, err := actions.ClearCompleted(ctx, todo.NewService(f))
		if err != nil {
			t.Fatal(err)
		}
		if len(out) != 1 || out[0].ID != 2 {
			t.Fatalf("got %#v", out)
		}
	})

	t.Run("empty list is a no-op", func(t *testing.T) {
		out, err := actions.ClearCompleted(ctx, todo.NewService(todotest.New()))
		if err != nil {
			t.Fatal(err)
		}
		if len(out) != 0 {
			t.Fatalf("got %#v", out)
		}
	})
}

// TestActionHandlers proves the action endpoints' bridge with the shared
// fake: routes registered, actions called, data reaches the rendered
// fragment. No database — the SQL is proven by the real-PG integration
// contract test, this only proves wiring and rendering.
func TestActionHandlers(t *testing.T) {
	t.Run("POST /todos/complete-all renders everything done", func(t *testing.T) {
		f := todotest.New()
		f.Seed(t, "a", "b")

		mux := http.NewServeMux()
		actions.Register(mux, todo.NewService(f))

		rec := testutil.Do(t, mux, http.MethodPost, "/todos/complete-all", nil)
		testutil.WantCode(t, rec, http.StatusOK)
		testutil.WantBody(t, rec, "a", "b", `class="done"`)
	})

	t.Run("POST /todos/clear-completed removes only done todos", func(t *testing.T) {
		f := todotest.New()
		f.Seed(t, "a", "b", "c")
		f.SetDone(2, true)

		mux := http.NewServeMux()
		actions.Register(mux, todo.NewService(f))

		rec := testutil.Do(t, mux, http.MethodPost, "/todos/clear-completed", nil)
		testutil.WantCode(t, rec, http.StatusOK)
		testutil.WantBody(t, rec, "a", "c")
		// b was done and is now gone from the fragment.
		if strings.Contains(rec.Body.String(), "b</button>") {
			t.Fatalf("done todo b still rendered: %q", rec.Body.String())
		}
	})
}

package handler

import (
	"context"
	"net/http"
	"os"
	"sync"
	"testing"

	"go-htmx-todo/internal/testutil"

	db "go-htmx-todo/internal/db/sqlc"
	"go-htmx-todo/internal/todo"

	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	poolOnce sync.Once
	tstPool  *pgxpool.Pool
	tstErr   error
)

// testPool connects to PostgreSQL once per package and applies schema.sql.
// Tests are skipped unless INTEGRATION_TESTS=1 (see `mise run test-integration`).
func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if os.Getenv("INTEGRATION_TESTS") != "1" {
		t.Skip("set INTEGRATION_TESTS=1 to run PostgreSQL integration tests")
	}

	ctx := context.Background()
	poolOnce.Do(func() {
		databaseURL := os.Getenv("DATABASE_URL")
		if databaseURL == "" {
			databaseURL = "postgres://postgres:postgres@localhost:5432/todos?sslmode=disable"
		}
		tstPool, tstErr = pgxpool.New(ctx, databaseURL)
		if tstErr != nil {
			return
		}
		if tstErr = tstPool.Ping(ctx); tstErr != nil {
			return
		}
		schema, err := os.ReadFile("../../db/schema.sql")
		if err != nil {
			tstErr = err
			return
		}
		if _, tstErr = tstPool.Exec(ctx, "DROP TABLE IF EXISTS todos;"+string(schema)); tstErr != nil {
			return
		}
	})
	if tstErr != nil {
		t.Fatalf("integration pool: %v", tstErr)
	}
	return tstPool
}

// truncate resets state so the contract test starts from a known, empty
// table (RESTART IDENTITY makes the first created todo's ID deterministic).
func truncate(t *testing.T, p *pgxpool.Pool) {
	t.Helper()
	if _, err := p.Exec(context.Background(), "TRUNCATE todos RESTART IDENTITY"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
}

// TestHandlerHTTPContract is the single integration test. It runs requests
// through the registered routes against real PostgreSQL and asserts:
//
//   - the right route matches and the form/body is parsed
//   - the handler calls the service and the service's data reaches the
//     response body (the new todo appears, the toggled one is marked done,
//     the empty state appears after deletion)
//   - sentinel errors map to statuses (422 validation, 404 missing)
//
// It does NOT assert the exact HTML the templates produce (no golden files)
// and it does NOT re-test service behavior (unit tests with the fake store
// already cover that). It only proves the bridge: HTTP in, rendered data out.
//
// The first four subtests form one sequential roundtrip — add, toggle,
// delete — each asserting the visible effect of the previous step.
func TestHandlerHTTPContract(t *testing.T) {
	p := testPool(t)
	truncate(t, p)

	mux := http.NewServeMux()
	Register(mux, todo.NewService(db.New(p)))

	t.Run("GET / renders the full page", func(t *testing.T) {
		rec := testutil.Do(t, mux, http.MethodGet, "/", nil)
		testutil.WantCode(t, rec, http.StatusOK)
		testutil.WantBody(t, rec, "<!doctype html>", `id="todo-list"`)
	})

	t.Run("POST /todos renders the fragment with the new todo", func(t *testing.T) {
		rec := testutil.Do(t, mux, http.MethodPost, "/todos", map[string][]string{"title": {"buy milk"}})
		testutil.WantCode(t, rec, http.StatusOK)
		testutil.WantBody(t, rec, "buy milk", `id="todo-list"`)
	})

	t.Run("POST /todos/{id}/toggle renders the todo as done", func(t *testing.T) {
		rec := testutil.Do(t, mux, http.MethodPost, "/todos/1/toggle", nil)
		testutil.WantCode(t, rec, http.StatusOK)
		testutil.WantBody(t, rec, `class="done"`, "buy milk")
	})

	t.Run("DELETE /todos/{id} renders the empty state", func(t *testing.T) {
		rec := testutil.Do(t, mux, http.MethodDelete, "/todos/1", nil)
		testutil.WantCode(t, rec, http.StatusOK)
		testutil.WantBody(t, rec, "no todos yet")
	})

	t.Run("POST /todos with blank title is 422", func(t *testing.T) {
		rec := testutil.Do(t, mux, http.MethodPost, "/todos", map[string][]string{"title": {"   "}})
		testutil.WantCode(t, rec, http.StatusUnprocessableEntity)
	})

	t.Run("toggle of missing todo is 404", func(t *testing.T) {
		rec := testutil.Do(t, mux, http.MethodPost, "/todos/999999/toggle", nil)
		testutil.WantCode(t, rec, http.StatusNotFound)
	})
}
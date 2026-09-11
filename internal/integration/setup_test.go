package integration_test

import (
	"context"
	"net/http"
	"testing"

	db "go-htmx-todo/internal/db/sqlc"
	"go-htmx-todo/internal/handlers"
	"go-htmx-todo/internal/handlers/auth"
	"go-htmx-todo/internal/handlers/static"
	"go-htmx-todo/internal/handlers/todo"
	"go-htmx-todo/internal/handlertest"

	"github.com/alexedwards/scs/pgxstore"
	"github.com/alexedwards/scs/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const schemaFile = "../db/schema.sql"

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	return handlertest.Pool(t, schemaFile)
}

// seedUser inserts a user with a real bcrypt hash for "password123", so the
// seeded user can actually sign in.
func seedUser(t *testing.T, p *pgxpool.Pool) uuid.UUID {
	t.Helper()
	hash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	id := uuid.New()
	if _, err := p.Exec(context.Background(),
		`INSERT INTO users (id, username, password_hash) VALUES ($1, $2, $3)`,
		id, "test-"+id.String(), hash); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return id
}

// setup builds the whole app, mirroring main.go. Register new domains here
// exactly as main.go does; the two must stay in sync.
func setup(t *testing.T) (app http.Handler, q *db.Queries, sessions *scs.SessionManager, user uuid.UUID) {
	t.Helper()
	p := testPool(t)
	handlertest.Truncate(t, p, "todos", "sessions", "users")
	user = seedUser(t, p)
	q = db.New(p)
	sessions = scs.New()
	sessions.Store = pgxstore.New(p)
	mux := http.NewServeMux()
	todo.Register(mux, handlers.Deps{Q: q, Sessions: sessions})
	auth.Register(mux, handlers.Deps{Q: q, Sessions: sessions})
	static.Register(mux, "../../static")
	return sessions.LoadAndSave(mux), q, sessions, user
}

// listDB reads the user's todos to prove stored state, not just HTML.
func listDB(t *testing.T, q *db.Queries, user uuid.UUID) []db.Todo {
	t.Helper()
	todos, err := q.ListTodos(context.Background(), user)
	if err != nil {
		t.Fatalf("ListTodos: %v", err)
	}
	return todos
}

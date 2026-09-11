# Go + HTMX + templ reference

Small server-rendered HTMX app using `net/http`, `templ`, `sqlc`, PostgreSQL,
`scs` sessions. Serves as a minimal template.

## Run

The app expects PostgreSQL. Default URL:
`postgres://postgres:postgres@localhost:5432/todos?sslmode=disable`

```bash
# 1. Generate Go code from SQL
mise run generate      # sqlc generate && templ generate

# 2. Apply desired schema (sqldef declarative, no migration files)
mise run db-apply      # psqldef --apply --file internal/db/schema.sql

# 3. Run
mise run dev           # go run .
```

`DATABASE_URL` is read from env, same default as `main.go`.

## Architecture

```text
main.go
  creates pool, Deps{Q, Sessions}
  each domain registers its own routes: todo.Register(mux, deps)
  wraps the mux with sessions.LoadAndSave

handlers
  root: Deps + ParseID + WriteError — shared wiring, no routes
  todo: the todo domain's routes and handlers (takes Deps as one arg)
  auth: sign-up/sign-in/sign-out + WithAuth/RequireAuth + hashing helpers
  static: serves ./static at /static (no listings, traversal rejected)

sqlc
  owns SQL, generated DB types, and generated query methods

PostgreSQL
```

There are no service structs, no store interfaces, no view-model types.
The generated `*db.Queries` travels inside one flat wiring bag, and
templates take `[]db.Todo` directly. Domains import the root for `Deps`
and helpers, never the reverse — so `main.go` is the only place that sees
every domain and no import cycle can form:

```go
type Deps struct {
	Q        *db.Queries
	Sessions *scs.SessionManager
	// Cross-cutting deps join here as the app grows (logger, mailer, ...),
	// each built once in main and owned by its own package.
}

// main.go composes domains; each domain owns its routes.
func Register(mux *http.ServeMux, d Deps) // in handlers/todo

func handleAdd(d Deps) http.HandlerFunc { // in handlers/todo
	return func(w http.ResponseWriter, r *http.Request) {
		title := strings.TrimSpace(r.FormValue("title"))
		if title == "" {
			handlers.WriteError(w, r, errors.New("title cannot be empty"), http.StatusUnprocessableEntity)
			return
		}
		// ... d.Q.CreateTodo, d.Q.ListTodos, templates.List(todos).Render
	}
}
```

Rules:

- **One `Deps` bag, not N parameters.** `Register` and every handler
  factory take `Deps` as a single argument, so a new shared dep means one
  field here — built once in `main`, closed over in `Register` — instead of
  a new parameter threaded through every factory.
- **Keep it dumb.** Nothing constructed inside the handlers package, and
  never request-scoped data: identity comes only from the auth middleware
  as `AuthData`, never from reading the session in a handler. Don't add
  fields speculatively — `slog.Default()` already covers logging with
  nothing passed at all.
- **Concrete by default, interfaces where swapping is real.** `*db.Queries`
  and `*scs.SessionManager` stay concrete because tests exercise the real
  thing — an interface there would add a seam nobody swaps. Reach for an
  interface when a second implementation genuinely exists: a payments client
  you must fake in tests, a mailer with sandbox/prod variants. Define it
  narrowly at the consumer, not as a mirror of the concrete struct:

```go
// in Deps: one method the handlers actually call, trivial to fake.
type Charger interface {
	Charge(ctx context.Context, amountCents int64, token string) error
}
```
- **No store interface.** Handlers call `d.Q.ListTodos`, `d.Q.CreateTodo`,
  etc. directly. If the SQL changes, the compiler points at the handler.
- **No action layer.** Validation is inline (`TrimSpace` + empty check);
  not-found is `errors.Is(err, pgx.ErrNoRows)` → 404. Status mapping lives
  next to the query that produces it.
- **No view models.** `templates.Page([]db.Todo)` and
  `templates.List([]db.Todo)` take domain rows directly.
- **Errors go through `handlers.WriteError`.** It slogs the error with
  method, path, and status, then renders: 4xx bodies carry the message,
  5xx bodies stay generic (`Internal Server Error`) with details in the log.

## HTTP — One Register Per Domain

`internal/handlers/todo/todo.go` holds the todo routes,
`internal/handlers/auth/auth.go` holds sign-up/sign-in/sign-out plus the
gates; the root `internal/handlers/handlers.go` holds only `Deps`,
`ParseID`, and `WriteError`:

```text
GET /{$}                    → full page
POST /todos                 → add            → list fragment
POST /todos/{id}/toggle     → toggle         → list fragment
DELETE /todos/{id}          → delete         → list fragment (idempotent)
POST /todos/complete-all    → complete all   → list fragment
POST /todos/clear-completed → clear completed → list fragment
GET+POST /signup            → sign-up page / create user → redirect /
GET+POST /signin            → sign-in page / log in     → redirect /
POST /signout               → log out                   → redirect /signin
```

`auth.WithAuth` is the one place that reads the session for identity: it
supplies `AuthData` (zero value when anonymous) to the page, while
`auth.RequireAuth` additionally redirects anonymous mutation requests —
303 to `/signin` for plain requests, 401 with `HX-Redirect: /signin` for
htmx ones — logging each redirect. `TestAnonymous` and
`TestRequireAuth_HtmxRedirect` lock both behaviors in.

## Auth

Username + password, bcrypt-hashed (`auth.HashPassword` /
`auth.CheckPassword`), no auth service — plain helpers. Sign-up and sign-in
`RenewToken` (no session fixation), then store `user_id` and `username` in
the session; sign-out destroys it. Failures re-render the form with a
message: blank/short credentials → 422, taken username → 409, bad
credentials → 401 (same message for unknown user and wrong password, so
usernames can't be probed).

## Sessions

Session management is `github.com/alexedwards/scs/v2` backed by
`scs/pgxstore` (the `sessions` table). Wiring lives in `main.go`:

```go
sessions := scs.New()
sessions.Store = pgxstore.New(pool)
sessions.Lifetime = 7 * 24 * 60 * time.Minute

mux := http.NewServeMux()
deps := handlers.Deps{Q: db.New(pool), Sessions: sessions}
todo.Register(mux, deps)

http.ListenAndServe(":8080", sessions.LoadAndSave(mux))
```

Tests build the same stack against the test database (`pgxstore` on the
test pool), so the session middleware under test is the real one.

## sqlc

Source files:

```text
sqlc.yaml
internal/db/schema.sql
internal/db/queries.sql
```

Generated files (`internal/db/sqlc/`) — never edit by hand; change SQL,
then run `sqlc generate`. `sqlc.yaml` maps UUID columns to
`github.com/google/uuid.UUID` so signatures stay friendly.

## sqldef (declarative schema)

No migration files. `internal/db/schema.sql` is the single source of truth
for both sqldef and sqlc:

```text
schema.sql ──► psqldef ──► PostgreSQL (diff + apply)
schema.sql ──► sqlc    ──► Go types/queries
```

Typical loop: edit `schema.sql` → `mise run generate` → `mise run db-plan`
→ `mise run db-apply` → `mise run check`.

## Testing — integration only

One tier: request the handlers over HTTP against real PostgreSQL, assert
the HTTP response **and** the database state. IDs stay deterministic
(`RESTART IDENTITY`: first todo per test is 1), assertions match body
substrings (never exact HTML), and `LoginAs` signs in through the real
session middleware, standing in for unimplemented sign-in.

```bash
go test ./...       # skips without INTEGRATION_TESTS=1
mise run test-integration  # disposable postgres:16-alpine on :5433, full suite
```

`internal/handlertest` holds the test-only helpers shared by the
integration suite (`Pool`, `Truncate`, `LoginAs`, `Do`, `WantCode`,
`WantBody`, `WantNoBody`). Production code must never import it.
`internal/integration/setup_test.go` composes the whole app — the same
wiring `main.go` performs — plus suite-specific seeding and state-reading:

- `testPool` — one-line wrapper fixing the schema path for `handlertest.Pool`.
- `seedUser` inserts a user with a real bcrypt hash (`password123`), so the
  seeded user can actually sign in.
- `setup` returns the real app (every domain's `Register` + `LoadAndSave`),
  the real `*db.Queries` for DB assertions, the session manager, and a
  fresh user. When a new domain lands, register it here exactly as `main.go`
  does — the two must stay in sync.
- `listDB` reads stored state for the "did it actually write?" assertion.

`internal/integration/todo_test.go` has one independent test per behavior
— each calls `setup(t)` for a fresh DB, so there is no shared state and no
ordering dependency:

- page renders full shell when empty; page shows only own todos
- add stores a trimmed row + renders the fragment; blank title is 422 and
  stores nothing
- toggle flips `done` in the DB both ways; missing → 404, malformed → 400,
  another user's row → 404 with both users' rows untouched
- delete removes only the targeted row; another user's/missing delete is 200 and
  changes nothing (idempotent, documented)
- complete-all marks all done (empty is a no-op); clear-completed keeps
  only open todos
- anonymous: page renders sign-in links, mutating redirects to sign-in
- auth: sign-up stores a hash (never plaintext) and logs in; duplicate is
  409; blank/short is 422; sign-in works and rejects bad credentials with
  401; sign-out destroys the session

The pattern per test is always the same — request, assert status + body
fragment, assert DB:

```go
rec := handlertest.Do(t, app, http.MethodPost, "/todos", url.Values{"title": {"buy milk"}}, cookie)
handlertest.WantCode(t, rec, http.StatusOK)
handlertest.WantBody(t, rec, "buy milk", `id="todo-list"`)
todos := listDB(t, q, user) // prove the row is really stored
```

## Folder Structure

```text
go-htmx-paradim-inspo/
├── main.go                       # composition root: pool, queries, sessions, mux
├── mise.toml
├── sqlc.yaml
├── internal/db/
│   ├── schema.sql                # users, sessions, todos (user_id FK)
│   ├── queries.sql               # user + user-scoped todo queries
│   └── sqlc/                     # generated code
├── internal/handlers/
│   ├── handlers.go               # Deps + ParseID + WriteError
│   ├── todo/
│   │   └── todo.go               # todo Register + handlers (takes Deps)
│   └── auth/
│       └── auth.go               # auth Register + handlers + hashing helpers
├── internal/handlertest/
│   └── handlertest.go            # shared test-only helpers (never imported by prod)
├── internal/integration/
│   ├── setup_test.go             # whole-app composition (mirror of main.go) + seeding
│   ├── todo_test.go              # todo tests: HTTP response + DB state
│   ├── auth_test.go              # auth tests: HTTP response + DB state
│   └── static_test.go            # static tests: content type, no listing/traversal
├── static/
│   └── js/htmx.min.js            # vendored htmx (no CDN dependency)
└── templates/
    ├── todos.templ               # Page(todos, username)/List(todos)/Item(t)
    ├── auth.templ                # Signup(errMsg)/Signin(errMsg)
    └── *_templ.go                # generated
```

A new domain is a new subpackage: `internal/handlers/users/` with its own
`Register(mux, handlers.Deps)` called from `main.go` and from the
integration `setup`, importing the root for `Deps` and helpers. Its tests
join the integration suite next to `todo_test.go`, reusing `handlertest`
and the same request-then-assert-DB pattern. `Truncate` takes the table
list from the caller, since the suite knows which tables exist.

## Mise Tasks

```bash
mise run db              # runs local pg via docker
mise run db-plan         # psqldef dry-run
mise run db-apply        # psqldef apply
mise run db-validate     # offline parse + idempotency check
mise run test            # go test ./...
mise run test-race       # go test -race ./...
mise run test-integration# disposable Postgres, runs integration tests, cleans up
mise run check           # generate + vet + test
mise run dev             # starts the dev db, then go run .
```

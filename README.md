# Go + HTMX + templ reference

Small server-rendered HTMX app using `net/http`, `templ`, `sqlc`, PostgreSQL,
and `log/slog`. Serves as reference.

## Run

The app expects PostgreSQL. Default URL:
`postgres://postgres:postgres@localhost:5432/todos?sslmode=disable`

```bash
# 1. Generate Go code from SQL
mise run generate      # sqlc generate && templ generate

# 2. Apply desired schema (Atlas declarative, no migration files)
mise run db-apply      # atlas schema apply --to file://internal/db/schema.sql
# or preview: mise run db-plan

# 3. Run
mise run dev           # go run .
```

`DATABASE_URL` is read from env, same default as `main.go`.

## Architecture

```text
main.go
  creates pool, generated *sqlc.Queries, and global slog logger
  builds each domain service ONCE and hands the ready services to every Register
  calls todo/handler.Register(mux, todos) and actions.Register(mux, todos)

todo/handler
  Register closes over the ready todo.Service and owns the domain routes
  handlers parse HTTP, call the service, and build VMs from []db.Todo

todo/Service
  owns todo rules; depends on TodoStore (interface), not concrete *db.Queries
  returns domain values ([]db.Todo), never ViewModels
  signals failure with sentinel errors: ErrValidation, ErrNotFound

todo/TodoStore
  the persistence seam; sqlc's generated *db.Queries satisfies it as-is
  tests substitute an in-memory fake here, so behavior tests need no database

actions
  cross-domain behaviors as plain functions over domain services
  Register owns the action routes; actions take only the services they need

httpx
  shared HTTP helpers; StatusFor maps every domain's sentinel errors to statuses

sqlc
  owns SQL, generated DB types, and generated query methods

PostgreSQL
```

The dependency flow is intentionally short:

```text
HTTP handler → todo.Service ──TodoStore──► *db.Queries → PostgreSQL
      ↑             │
      └ templ VM ←──┘   handler maps []db.Todo (domain) → VM

HTTP handler → action(s) ──► two+ domain services (composition only)
```

## sqlc

Source files:

```text
sqlc.yaml
internal/db/schema.sql
internal/db/queries.sql
```

Generated files:

```text
internal/db/sqlc/
  db.go
  models.go
  queries.sql.go
```

Never edit generated files. Change SQL, then run:

```bash
sqlc generate
```

## Atlas (declarative schema)

This repo uses Atlas declarative workflow — closest to Drizzle `push`. No
`internal/db/migrations/*.sql` files. `internal/db/schema.sql` is the single
source of truth for both Atlas and sqlc:

```text
schema.sql ──► Atlas ──► PostgreSQL (schema apply)
schema.sql ──► sqlc  ──► Go types/queries
```

`sqlc.yaml` already points at it:

```yaml
schema: internal/db/schema.sql
```

**Typical loop:**

```bash
# edit internal/db/schema.sql (add table/column)
# edit internal/db/queries.sql if needed
mise run generate      # sqlc + templ
mise run db-plan       # preview ALTER TABLE
mise run db-apply      # apply
mise run check         # vet + tests
```

**What Atlas does:**

```bash
atlas schema apply \
  --url "$DATABASE_URL" \
  --to "file://internal/db/schema.sql" \
  --dev-url "docker://postgres/16/dev" \
  --auto-approve       # skip prompt, plan still shown in --dry-run
```

*   compares current DB vs desired `schema.sql`
*   plans `CREATE/ALTER TABLE`
*   applies with `docker://postgres/16/dev` as temp dev DB for validation

If you later want versioned, reviewed SQL in git, switch Atlas to `migrate diff`:

```bash
atlas migrate diff add_users --to file://internal/db/schema.sql --dir file://internal/db/migrations --dev-url docker://postgres/16/dev
```

## Domain Service

`internal/todo/service.go` contains:

```go
type Service struct {
    store TodoStore
}

func NewService(store TodoStore) *Service
func (s *Service) List(ctx context.Context) ([]db.Todo, error)
func (s *Service) Add(ctx context.Context, title string) ([]db.Todo, error)
func (s *Service) Toggle(ctx context.Context, id int64) ([]db.Todo, error)
func (s *Service) Delete(ctx context.Context, id int64) ([]db.Todo, error)
```

Services hold the behavior worth naming: validation, error decisions, and
cross-query flows. A trivial operation can still live here because every
domain gets a service in this reference.

**Decision: the service returns domain values, not ViewModels.**

Service methods return `[]db.Todo` — the domain shape. Handlers build the
HTMX ViewModels from it:

```go
todos, err := svc.Add(r.Context(), r.FormValue("title"))
if err != nil {
    http.Error(w, err.Error(), statusFor(err))
    return
}
templates.List(todo.ListVM{Todos: todos}).Render(r.Context(), w)
```

Why: a ViewModel is a display concern. If the service returned `ListVM`
directly, every service test would depend on the HTML fragment shape, and a
UI change would break domain tests. Returning domain values keeps the service
UI-agnostic, so the same method can later feed JSON, a CLI, or another UI
without changing behavior.

Services signal *why* they failed with sentinel errors, so callers decide
what to do without string-matching messages:

```go
var ErrValidation = errors.New("todo: validation failed")
var ErrNotFound   = errors.New("todo: not found")
```

Pure rules stay beside the service:

```go
func ValidateTitle(title string) error
```

Do not extract every small check into another package.

## Persistence Seam

`internal/todo/store.go` defines what the todo domain needs from storage:

```go
type TodoStore interface {
    ListTodos(ctx context.Context) ([]db.Todo, error)
    CreateTodo(ctx context.Context, title string) (db.Todo, error)
    GetTodo(ctx context.Context, id int64) (db.Todo, error)
    UpdateTodo(ctx context.Context, arg db.UpdateTodoParams) error
    DeleteTodo(ctx context.Context, id int64) error
}

var _ TodoStore = (*db.Queries)(nil) // compile-time check
```

**Decision: depend on the interface, not on `*db.Queries`.**

- The interface is owned by the domain package and implemented for free by
  sqlc's generated `*db.Queries`. Production wiring stays `NewService(db.New(pool))`
  — there is no adapter package.
- The compile-time assertion fails the build if the generated queries ever
  drift from the interface, so drift is caught in the compiler, not in tests.
- Tests substitute an in-memory fake (`internal/todo/service_test.go`), so
  the whole behavior suite runs in milliseconds with no database.

Keep the interface narrow: only what this domain actually calls. A new domain
gets its own store interface rather than a growing global one.

## Authorization — Subject as a Parameter (pattern only, not implemented)

This reference has no users, so nothing here enforces auth. When you add
users to a real app built on this reference, use this pattern — it is the
difference between testable and untestable auth code.

**Protocol (avoid):** the service signature hides the actor:

```go
func (s *Service) Toggle(ctx context.Context, id int64) error // <- who is acting?
```

The implementation would have to read the "current user" from sessions or
middleware. Correctness now depends on invisible request state, so testing
each auth state means fabricating sessions, cookies, and middleware for
every test.

**Parameter (use):** the actor is an explicit input to the service, and the
policy is a pure function:

```go
type Subject struct {
    ID   int64
    Role Role // guest | member | admin
}

// The whole policy in one pure, total, table-testable function.
func can(u Subject, action string, ownerID int64) bool {
    switch u.Role {
    case RoleAdmin:  return true
    case RoleMember: return u.ID == ownerID
    default:         return false
    }
}

func (s *Service) Toggle(ctx context.Context, u Subject, id int64) ([]db.Todo, error) {
    t, err := s.store.GetTodo(ctx, id)
    if err != nil { /* map to ErrNotFound */ }
    if !can(u, "todos.toggle", t.OwnerID) {
        return nil, ErrForbidden
    }
    // ...
}
```

The session middleware only *parses* the cookie into a `Subject`; the service
calls the pure policy and makes the *decision*. Test the whole role matrix as
a table test against `can`, and test each auth state with a one-line
`Subject` literal — no sessions, no HTTP. When the matrix grows, move `can`
into its own `policy.go`.

## Handler Registration

`internal/todo/handler/handler.go` owns todo routes:

```go
func Register(mux *http.ServeMux, svc *todo.Service) {
    mux.HandleFunc("GET /{$}", handlePage(svc))
    mux.HandleFunc("POST /todos", handleAdd(svc))
    mux.HandleFunc("POST /todos/{id}/toggle", handleToggle(svc))
    mux.HandleFunc("DELETE /todos/{id}", handleDelete(svc))
}
```

**Decision: `main.go` is the composition root — it builds each service once,
every Register receives ready services.**

- Register functions construct nothing; they close over what they receive and
  own only their routes. One service instance per domain, built in one place.
- Any package taking a ready service is trivially testable: construct it over
  an in-memory fake, call it, assert — no wiring dance.
- Each domain's Register stays domain-owned: it decides its own routes. main
  decides nothing except which services exist.

Handlers do three jobs and nothing else: parse HTTP, call the service or
action, map sentinel errors to statuses via `httpx.StatusFor`, and shape
`[]db.Todo` into ViewModels before rendering. They never decide policy.

Each handler has one job:

```text
GET /                    → full Page VM → full document
POST /todos              → List VM → list fragment
POST /todos/{id}/toggle  → List VM → list fragment
DELETE /todos/{id}       → List VM → list fragment
```

## Actions — Cross-Domain Behavior as Plain Functions

`internal/actions/` holds behaviors that span two or more domains. An action
is a **plain function over the services it needs** — no registry struct, no
state, no storage of its own:

```go
func CompleteAll(ctx context.Context, todos *todo.Service) ([]db.Todo, error)
func ClearCompleted(ctx context.Context, todos *todo.Service) ([]db.Todo, error)
```

**Decision: actions take their dependencies as arguments, and that's the
whole abstraction.**

- The signature documents exactly which domains the behavior touches. When a
  second domain arrives, the action simply takes both services —
  `DeleteAccount(ctx, users, todos, userID)` — and stays equally testable.
- No service-locator struct holding pointers to every service: that would
  let any action reach everything, hide dependencies, and force tests to
  wire the world. Function arguments keep the dependency graph in the type
  system.
- Actions compose services; they never touch storage directly. If an action
  starts needing its own queries, the boundary was drawn wrong — move the
  behavior into the domain service that owns the data.

**When is something an action vs a service method?** A service method
belongs to one domain and owns its rules. An action is worth its own
existence only when it composes two or more domain services, or orchestrates
a workflow neither domain should own alone. Resist promoting single-domain
logic into actions — that is how pass-through layers are born.

`Register(mux, todos *todo.Service)` at the top of `actions.go` wires the
action routes (`POST /todos/complete-all`, `POST /todos/clear-completed`).
It takes the ready services main already built — actions are generic over
their arguments — and the thin HTTP handlers call the action functions and
render the fragment. `main.go` calls both `handler.Register` and
`actions.Register` with the same service instance.

**Decision: one shared `httpx.StatusFor`, not a switch per package.**

Sentinel errors are package-qualified values (`todo.ErrNotFound`), so one
total function over all of them — `internal/httpx.StatusFor` — is a single
table of "which failure maps to which status" instead of drifting copies.
The guardrails that keep it healthy: it stays one flat switch (every case
maps a sentinel, unknown → 500), it holds no per-domain logic, and a new
domain's sentinels each get one case. A forgotten case falls to 500 — and
the integration tests' error-path assertions (422, 404) catch exactly that.

**Testing an action is the cheapest test in the codebase:** the action is a
plain function, so its test constructs the shared fake
(`internal/todo/todotest`), calls the function, and asserts on domain
values — no database, no HTTP. The action's HTTP endpoints are additionally
covered by `TestActionHandlers` (same fake, `testutil.Do`), so wiring and
rendering are proven in milliseconds too; the SQL behind them runs in the
real-PG contract test like every other route.

## Transactions

sqlc generates `Queries.WithTx(tx)`. For a cross-domain operation, start one
transaction, create transaction-bound queries, and call all operations through
those queries:

```go
func withTx(ctx context.Context, pool *pgxpool.Pool, fn func(*db.Queries) error) error {
    tx, err := pool.Begin(ctx)
    if err != nil {
        return err
    }
    defer tx.Rollback(ctx)

    if err := fn(db.New(pool).WithTx(tx)); err != nil {
        return err
    }
    return tx.Commit(ctx)
}
```

Then a named cross-domain operation stays a plain function:

```go
func DeleteUser(ctx context.Context, pool *pgxpool.Pool, userID int64) error {
    return withTx(ctx, pool, func(q *db.Queries) error {
        if err := q.DeleteTodosByUser(ctx, userID); err != nil {
            return err
        }
        return q.DeleteUser(ctx, userID)
    })
}
```

## Testing

Testing is organized in two tiers. Tier 1 covers the behavior; Tier 2 proves http bridge and SQL.

### Tier 1 — unit tests (no database)

`internal/todo/service_test.go` runs under plain `go test ./...` with no
Docker. It substitutes an in-memory fake `TodoStore`, so validation, adding,
toggling, deletion, error wrapping, and empty states are all testable in
milliseconds:

```bash
go test ./...       # unit tests only — no Docker, no env vars
go test -race ./...
go vet ./...
```

**This is where behavioral coverage lives.** If a new rule belongs to the
service, it gets a test here — not in an HTTP test, not in a browser test.

**Decision: one fake per domain, in an `Xtest` package.**
`internal/todo/todotest.FakeTodos` is the single in-memory `TodoStore` used
by every package's tests — the todo service's, the actions', and any future
action that takes the service. It lives in a regular package instead of a
`_test.go` file because Go test files cannot be imported; without this,
every package's tests would redeclare the same fake. The knock-on rule:
tests using it are black-box (`package todo_test`), because a package's own
test binary cannot import anything that imports the package under test —
black-box tests have no such restriction. Each new domain gets the same
pair: `store.go` (seam) + `Xtest/` (shared fake), and the fakes carry their
own compile-time `var _ todo.TodoStore = (*FakeTodos)(nil)` check, mirroring
the real store's lockstep with the interface.

Tier 1 also covers the handler's one pure decision — the sentinel→status
mapping — through the integration test's 422/404 cases rather than a
dedicated table test; the mapping is a single `switch` in `statusFor`.

### Tier 2 — integration test (real PostgreSQL)

`TestHandlerHTTPContract` in `internal/todo/handler/handler_integration_test.go`
is skipped unless `INTEGRATION_TESTS=1`. Run it with a disposable container:

```bash
mise run test-integration  # starts postgres:16-alpine on :5432, runs tests, cleans up
```

There is exactly **one** integration test, and it exists to prove what the
in-memory fake cannot: that the bridge works against the real stack. A
subtest roundtrip (add → toggle → delete) sends real requests through the
registered routes and asserts the *visible effect of each step* — the new
todo appears in the fragment, the toggled one is rendered done, the empty
state appears after deletion — plus the error statuses (blank title → 422,
missing todo → 404).

What it deliberately does **not** do:

- **No golden files.** It never asserts the exact HTML templates produce —
  only that the service's data reaches the rendered body. Asserting exact
  markup makes every cosmetic template tweak break tests for no reason.
- **No replay of service behavior.** The service matrix lives in Tier 1;
  duplication between tiers means a test asserts something the layer below
  already proved.

Note that the SQL still runs here — every subtest executes real queries
through the real app path. If a generated query breaks, this test fails.
Per-query store tests are only worth adding when a query's correctness
depends on PostgreSQL semantics a fake cannot mirror: joins, upserts,
ordering guarantees, RLS, triggers, transactions.


## Folder Structure

```text
go-htmx-paradim-inspo/
├── main.go
├── mise.toml
├── sqlc.yaml
├── internal/db/
│   ├── schema.sql
│   ├── queries.sql
│   └── sqlc/                         # generated code
├── internal/testutil/
│   └── testutil.go                   # test-only helpers: Do, WantCode, WantBody
├── internal/httpx/
│   └── httpx.go                      # StatusFor: all domains' sentinels → HTTP statuses
├── internal/actions/
│   ├── actions.go                    # cross-domain actions + Register (action routes)
│   └── actions_test.go               # plain function tests + fake-store HTTP tests
├── internal/todo/
│   ├── service.go                    # service logic, returns domain []db.Todo
│   ├── todotest/
│   │   └── todotest.go               # FakeTodos: the shared in-memory TodoStore
│   ├── vm.go                         # HTMX ViewModels, built by handlers
│   ├── service_test.go               # unit tests (black-box) over the shared fake
│   └── handler/
│       ├── handler.go                # Register + handlers + error→status mapping
│       └── handler_integration_test.go  # single HTTP contract test (real PG)
└── templates/
    ├── todos.templ
    └── todos_templ.go                # generated
```

When adding another domain, add another vertical slice:

```text
internal/user/service.go
internal/user/store.go       # its own store interface for the persistence seam
internal/user/vm.go
internal/user/handler/handler.go
```

The user service can receive the same `*db.Queries` — it satisfies whichever
domain store interfaces exist. SQL remains centralized in `internal/db`,
while domain routes and service behavior stay in their own slice. Test
helpers (`Do`, `WantCode`, `WantBody`) live in `internal/testutil` and are
shared across domains. Behaviors spanning the user and todo domains become
plain functions in `internal/actions` — `DeleteAccount(ctx, users, todos, id)`
— whose tests are plain function tests over fakes.

## Mise Tasks

`mise.toml` pins Go, sqlc, templ, and Atlas versions:

```bash
mise run generate        # sqlc generate && templ generate
muse run db              # runs local pg via docker
mise run db-plan         # atlas dry-run
mise run db-apply        # atlas schema apply
mise run db-validate     # atlas schema validate
mise run test            # go test ./...
mise run test-race       # go test -race ./...
mise run test-integration# spins up disposable Postgres, runs integration tests, cleans up
mise run check           # generate + vet + test
mise run dev             # go run .
```

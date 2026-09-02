# Go + HTMX + templ reference

Small server-rendered HTMX app using `net/http`, `templ`, `sqlc`, PostgreSQL,
and `log/slog`. No handwritten repository wrapper and no per-domain query
interfaces. `sqlc` is the database layer.

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
  calls todo/handler.Register(mux, queries)

todo/handler
  Register creates todo.Service from *sqlc.Queries
  handlers parse HTTP and render service-returned VMs

todo/Service
  owns todo rules and calls generated sqlc methods
  returns HTMX-oriented ViewModels

sqlc
  owns SQL, generated DB types, and generated query methods

PostgreSQL
```

The dependency flow is intentionally short:

```text
HTTP handler → todo.Service → sqlc.Queries → PostgreSQL
      ↑              ↓
      └──── templ VM ┘
```

## Why No Repository Wrapper?

sqlc already generates typed methods:

```go
queries.GetTodo(ctx, id)
queries.CreateTodo(ctx, title)
queries.UpdateTodo(ctx, params)
queries.DeleteTodo(ctx, id)
```

A handwritten method that only calls the generated method adds a redundant
layer. Services use concrete `*db.Queries` directly. If another domain is
added, it can receive the same shared `*db.Queries` instance.

This reference also does not maintain local query interfaces. That is a
deliberate choice: the important behavior is tested through real PostgreSQL.
If a future piece of logic becomes complex enough to need isolated testing,
add a small interface or pure function then, not before.

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

`db.Todo` is used directly in this small app. No second persistence model and
no JSON or DB tags are needed for HTMX. Introduce separate domain types only
when DB shape and domain shape genuinely differ.

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

No need for 12 files for 6 tables — one `schema.sql` holds all `CREATE TABLE` statements. Atlas diffs it.

If you later want versioned, reviewed SQL in git, switch Atlas to `migrate diff`:

```bash
atlas migrate diff add_users --to file://internal/db/schema.sql --dir file://internal/db/migrations --dev-url docker://postgres/16/dev
```

This repo stays declarative by default. Add `down` migrations only if you need explicit rollback; Atlas declarative does not require them.

## Domain Service

`internal/todo/service.go` contains:

```go
type Service struct {
    queries *db.Queries
}

func NewService(queries *db.Queries) *Service
func (s *Service) List(ctx context.Context) (PageVM, error)
func (s *Service) Add(ctx context.Context, title string) (ListVM, error)
```

Services hold business behavior that is worth naming: validation, ownership,
authorization, multiple queries, and error decisions. A trivial operation can
still live there because every domain gets a service in this reference.

Service methods return VMs directly because this project targets HTMX. That
keeps handlers short:

```go
vm, err := svc.Add(r.Context(), r.FormValue("title"))
if err != nil {
    http.Error(w, err.Error(), http.StatusUnprocessableEntity)
    return
}
templates.List(vm).Render(r.Context(), w)
```

If the same operation later serves JSON, a CLI, or another UI, split the
reusable result from the HTMX VM at that point.

Pure rules can remain beside the service:

```go
func ValidateTitle(title string) error
```

Do not extract every small check into another package.

## Handler Registration

`internal/todo/handler/handler.go` owns todo routes:

```go
func Register(mux *http.ServeMux, queries *db.Queries) {
    svc := todo.NewService(queries)
    mux.HandleFunc("GET /{$}", handlePage(svc))
    mux.HandleFunc("POST /todos", handleAdd(svc))
    mux.HandleFunc("POST /todos/{id}/toggle", handleToggle(svc))
    mux.HandleFunc("DELETE /todos/{id}", handleDelete(svc))
}
```

`main.go` creates one `*sqlc.Queries` and passes it to each domain's
`Register`. Each domain creates its own service and passes that service into
its handler closures. Handlers do not receive a logger; startup installs the
global default `slog` logger.

Each handler has one job:

```text
GET /                    → full Page VM → full document
POST /todos              → List VM → list fragment
POST /todos/{id}/toggle  → List VM → list fragment
DELETE /todos/{id}       → List VM → list fragment
```

There is no `HX-Request` branching. Page and fragment endpoints are separate.

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

No `Factory`, `UnitOfWork`, or universal application orchestrator is required.

## Testing

The main reference test is an HTTP integration test against ephemeral real
PostgreSQL. It uses `httptest`, the real registered handlers, real sqlc
queries, and real database state. It does not use Playwright.

The test is skipped by default so ordinary `go test ./...` needs no Docker:

```bash
go test ./...
go test -race ./...
go vet ./...
```

Run the PostgreSQL integration test with Docker:

```bash
INTEGRATION_TESTS=1 go test ./internal/todo/handler -run TestHandlersWithPostgres -v
```

The integration test verifies:

- route registration and HTTP request parsing
- service and sqlc wiring
- real inserts, updates, deletes, and selects
- PostgreSQL schema compatibility
- full-page versus fragment responses

Add pure service tests only when a rule becomes sufficiently complex to
deserve isolation. Add Playwright for browser-specific behavior, such as
confirming that HTMX performs the expected DOM swap.

Do not use SQLite as a fake PostgreSQL. SQL syntax, constraints, types,
locking, and transaction behavior differ.

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
├── internal/todo/
│   ├── service.go                    # service + direct *sqlc.Queries
│   ├── vm.go                         # HTMX VMs
│   └── handler/
│       ├── handler.go                # Register + handlers
│       └── handler_integration_test.go
└── templates/
    ├── todos.templ
    └── todos_templ.go                # generated
```

When adding another domain, add another vertical slice:

```text
internal/user/service.go
internal/user/vm.go
internal/user/handler/handler.go
```

The user service can receive the same `*db.Queries`. SQL remains centralized
in `internal/db`, while domain routes and service behavior stay in their own
slice.

## Mise Tasks

`mise.toml` pins Go, sqlc, templ, and Atlas versions:

```bash
mise run generate        # sqlc generate && templ generate
mise run db-plan         # atlas dry-run
mise run db-apply        # atlas schema apply
mise run db-validate     # atlas schema validate
mise run test            # go test ./...
mise run test-race       # go test -race ./...
mise run test-integration# integration with real Postgres
mise run check           # generate + vet + test
mise run dev             # go run .
```

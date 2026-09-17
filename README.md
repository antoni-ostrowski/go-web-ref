# Go + HTMX + templ reference

Small server-rendered HTMX app using `net/http`, `templ`, `sqlc`, PostgreSQL,
`scs` sessions, and OpenTelemetry. Serves as a minimal template.

## Run

The app expects PostgreSQL. Default URL:
`postgres://postgres:postgres@localhost:5432/todos?sslmode=disable`

```bash
# 1. Generate Go code from SQL
mise run generate      # sqlc generate && templ generate

# 2. Apply desired schema (sqldef declarative, no migration files)
mise run db-apply      # psqldef --apply --file internal/db/schema.sql

# 3. Run
mise run dev           # Air hot reload (starts dev db, applies schema)
```

`DATABASE_URL` is read from env, same default as `server.Run`.

## Architecture

```text
main.go      env + signals, then server.Run
server       Config + Run + NewServer + NewHandler (shared by main + tests)
handlers     domains (todo, auth, static) + shared root (Deps, helpers)
sqlc         SQL, generated DB types and query methods
```

One shared bag (`handlers.Deps`) carries process-wide deps to every
domain; each domain registers its own routes and imports the root, never
the reverse — so `server` is the only place that sees every domain and no
import cycle can form. Handlers call generated `*db.Queries` directly, and
templates take domain rows directly. Errors go through `handlers.WriteError`
(4xx message to the client, generic 5xx, details in the log, span marked
red on 5xx).

## HTTP — One Register Per Domain

`internal/handlers/todo/todo.go` holds the todo routes,
`internal/handlers/auth/auth.go` holds sign-up/sign-in/sign-out plus the
gates; the root `internal/handlers/handlers.go` holds the shared pieces
(`Deps`, `ParseID`, `WriteError`, `Route`):

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
usernames can't be probed). Sign-up, sign-in, and sign-out responses carry
`Clear-Site-Data: "cache"` so browsers prune origin cache at auth
transitions instead of showing stale signed-in/out pages on back-button;
page shells additionally reload on bfcache restore (`pageshow`).

## Sessions

`scs` with a Postgres store (the `sessions` table), built once via
`auth.NewSessionManager` and shared through `Deps`. Tests use the same
manager against the test database, with a discard logger and noop
telemetry, so the session middleware under test is the real one.

## Observability (`internal/obs`)

OpenTelemetry traces + logs via OTLP/HTTP, with stderr JSON always on.
Example usage (spans, contextual logs, error marking) lives in the todo
handlers (`internal/handlers/todo/todo.go`); shared wiring is
`handlers.Deps{Logger, Tel}`.

```bash
OTEL_EXPORTER_OTLP_ENDPOINT=          # fallback for all signals
OTEL_EXPORTER_OTLP_TRACES_ENDPOINT=   # per-signal overrides
OTEL_EXPORTER_OTLP_METRICS_ENDPOINT=
OTEL_EXPORTER_OTLP_LOGS_ENDPOINT=
APP_ENV=production                    # OTel exports INFO+; otherwise DEBUG+
```

No endpoint set → telemetry stays noop (one `otel disabled` warning) and
the app keeps running on stderr logs. Tests skip the SDK the same way.

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

## Styling — Tailwind via standalone CLI

No Node, no `package.json`: the Tailwind CLI runs as a pinned standalone
binary (`github:tailwindlabs/tailwindcss` in `mise.toml`). It scans
`templates/` for classes (`@source` in `input.css`) and emits plain CSS
served by the static handler. Two flavors:

```bash
mise run dev    # Air hot reload: unminified CSS rebuilt on template change
mise run build  # prod binary + minified CSS via `mise run css`
```

`static/css/app.css` is gitignored build output. Utility classes appear in
`.templ` sources as plain strings (including inside `templ.KV`), so the
scanner picks them up with no config beyond `@source`.

## Testing — integration only

One tier: request the handlers over HTTP against real PostgreSQL, assert
the HTTP response **and** the database state. `login` signs in through the
real `/signin` form, so auth changes break todo tests too. One smoke test
boots the real `server.Run` on a live port to prove main's wiring serves.

```bash
go test ./...       # skips without INTEGRATION_TESTS=1
mise run test-integration  # disposable postgres:16-alpine on :5433, full suite
```

## Folder Structure

```text
go-htmx-paradim-inspo/
├── main.go                       # env + signals, then server.Run
├── internal/server/
│   └── server.go                   # Config + Run + NewServer + NewHandler
├── mise.toml
├── sqlc.yaml
├── internal/db/
│   ├── schema.sql                # users, sessions, todos (user_id FK)
│   ├── queries.sql               # user + user-scoped todo queries
│   └── sqlc/                     # generated code
├── internal/handlers/
│   ├── handlers.go               # Deps + ParseID + WriteError + Route
│   ├── todo/
│   │   └── todo.go               # todo Register + handlers (takes Deps)
│   └── auth/
│       └── auth.go               # auth Register + handlers + hashing helpers
├── internal/obs/
│   ├── otel.go                   # SetupOTelSDK: OTLP providers from env
│   └── logs.go                   # NewLogHandler fanout + MinLevel gate
├── internal/integration/
│   ├── setup_test.go             # test deps + seeding (serves via server.NewHandler)
│   ├── helpers_test.go           # shared helpers: Pool, Do/DoHtmx, WantCode/Body/NoBody
│   ├── smoke_test.go             # boots real server.Run on a live port
│   ├── todo_test.go              # todo tests: HTTP response + DB state
│   ├── auth_test.go              # auth tests: HTTP response + DB state
│   └── static_test.go            # static tests: content type, no listing/traversal
├── static/
│   ├── css/input.css            # Tailwind entry (@import + @source)
│   ├── css/app.css              # built output (gitignored, `mise run css`)
│   └── js/htmx.min.js            # vendored htmx (no CDN dependency)
└── templates/
    ├── layout.templ              # Layout(title): single page shell
    ├── todos.templ               # Page(todos, username)/List(todos)/Item(t)
    ├── auth.templ                # Signup(errMsg)/Signin(errMsg)
    └── *_templ.go                # generated
```

A new domain is a new subpackage: `internal/handlers/users/` with its own
`Register(mux, handlers.Deps)`, called once from `server.NewHandler`.
Its tests join the integration suite next to `todo_test.go`, reusing the
shared helpers and the same request-then-assert-DB pattern.

## Mise Tasks

```bash
mise run generate        # sqlc + templ codegen
mise run css             # Tailwind build, minified (prod)
mise run dev             # Air hot reload: templ + unminified CSS + Go rebuild
mise run build           # generate + minified CSS + prod binary in tmp/
mise run db              # runs local pg via docker
mise run db-plan         # psqldef dry-run
mise run db-apply        # psqldef apply
mise run db-validate     # offline parse + idempotency check
mise run test            # go test ./...
mise run test-race       # go test -race ./...
mise run test-integration# disposable Postgres, runs integration tests, cleans up
mise run check           # generate + vet + test
```

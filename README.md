# Go + HTMX + Templ Web app structure reference

Bare-minimum reference for building Go web apps with HTMX **fully decoupled from rendering** — app is self-contained and testable without HTTP or HTML.

Stack: `net/http` + `templ` + `htmx`. No chi, no ORM, no JS build. In-memory store (swap for Postgres without touching handlers).

Run: `templ generate && go run .` → http://localhost:8080

---

## Big Idea

> **Repo (DB) → Service (rules) → Handler (builds VM) → Renderer (templ) → HTML**

```
Browser --HTTP--> Handler --call--> Service --call--> Repo --data--> DB
                    | builds VM from domain      | pure validation
                    v                            v
                 templ.Page/List(VM)         ValidateTitle()
```


This is **Hexagonal / Ports & Adapters** stripped to vertical slices — `Service + Repo + VM + Renderer`. Stateless Elm-ish.

---

## Layers — what each does, how it connects

### 1. Model — `internal/todo/service.go:8`

```go
type Todo struct { ID, Title string; Done bool }
```

What a todo *is*. No `json:`, no `htmx`. Everyone imports it, it imports nothing.

### 2. Repo — `internal/db/repo.go:12` (GLOBAL single DB repo, pointer)

```go
// db.Repo is ONE struct for whole app, shared across all domains
type Repo struct { mu sync.Mutex; todos []todo.Todo; nextID atomic.Int64 }
func New() *Repo { return &Repo{} }
func (r *Repo) List(ctx context.Context) ([]todo.Todo, error)
func (r *Repo) Save(ctx context.Context, t todo.Todo) (todo.Todo, error)
func (r *Repo) Get(ctx context.Context, id string) (todo.Todo, error)
// add more domains here: SaveUser, ListProjects... still one Repo
```

**Purpose:** Single place for all DB operations. One `*Repo` instance for whole app, passed to every `Service`. All methods have `*Repo` pointer receiver — only `*Repo` implements interfaces.

*   **Global & shared:** `main.go` does `repo := db.New()` once, every handler/service shares `*repo`. Safe because `Repo` holds only `pool/mutex` (read-only after `New`), no `currentUserID` field — pass per-request data via `ctx`/args. `*sql.DB`/`*pgxpool.Pool` inside is concurrency-safe.
*   **Implements domain PORTs:** `todo` defines `type Repository interface{ List/Save/Get... }`, `db.Repo` implements it (`var _ todo.Repository = (*db.Repo)(nil)`). So `db` imports `todo`, `todo` NEVER imports `db` — no cycle. Add `users` later: define `user.Repository` in `internal/user`, same `db.Repo` implements it.
*   For now in-memory. Swap to Postgres: keep same methods, change bodies to `tx.Query`. No handler/service change.

**Connects to:** Called only by `Service`.

### 3. Service — `internal/todo/service.go:16` (core, pointer)

```go
// PORT defined by domain, implemented by global db.Repo
type Repository interface {
  List(ctx context.Context) ([]Todo, error)
  Save(ctx context.Context, t Todo) (Todo, error)
  Get(ctx context.Context, id string) (Todo, error)
}
type Service struct{ repo Repository } // holds interface, not *db.Repo
func NewService(repo Repository) *Service { return &Service{repo: repo} }

func ValidateTitle(title string) error // PURE, no repo
```

**Purpose:** Business rules. Validates via pure `ValidateTitle`, creates domain `Todo`, delegates to `Repository`, returns domain `[]Todo` (not VM).

*   Imports nothing from `net/http`, `templ`, or `db`.
*   Holds `Repository` interface — test with `fakeRepo` in `todo_test.go` (avoids import cycle `todo -> db -> todo`), prod with `*db.Repo`.
*   Pure funcs live same file, they contain pure logic that can be tested separetly without side effects. 

**Connects to:** Called by `Handler`. Returns `[]Todo` to `Handler`.

**Tested by:** `internal/todo/todo_test.go:8` — uses local `fakeRepo` (no `db` import): `repo:=newFakeRepo(); svc:=NewService(repo); svc.Add(ctx,"buy milk")`.

### 4. Handler + ViewModel — `internal/todo/vm.go` and `internal/todo/handler/handler.go`

```go
// vm.go — UI-shaped, one per renderer most of the time
type ListVM struct{ Todos []Todo }
type PageVM struct{ Todos []Todo }

// handler.go — domain owns its routes
func Register(mux *http.ServeMux, svc *Service) {
  mux.HandleFunc("GET /{$}", handlePage(svc))
  mux.HandleFunc("POST /todos", handleAdd(svc))
  mux.HandleFunc("POST /todos/{id}/toggle", handleToggle(svc))
  mux.HandleFunc("DELETE /todos/{id}", handleDelete(svc))
}
func handleAdd(svc *Service) http.HandlerFunc {
  return func(w http.ResponseWriter, r *http.Request){
    todos, err := svc.Add(r.Context(), r.FormValue("title"))
    if err != nil { http.Error(w,err.Error(),422); return }
    vm := ListVM{Todos: todos} // handler builds VM
    // passes VM for rendering
    templates.List(vm).Render(r.Context(), w)
  }
}
```

**Purpose:** Thin glue. handler = wires up service, VM's and rendering.

*   `Register` keeps `main.go` as composition root only — `main.go:12` just `repo:=db.New(); svc:=todo.NewService(repo); handler.Register(mux, svc)` (one global `*db.Repo` shared).
*   Handlers are `func(*Service) http.HandlerFunc` closures capturing deps — no `struct{svc *Service}` needed. Testable via `httptest.NewRequest` + `FakeRepo` if you want, but most bugs already covered by `Service` tests.
*   `Handler` **builds VM** from domain slice. Why not `Service`? VM is UI-shaped (`CanEdit`, formatted date). Keeping it in handler keeps `Service` reusable for JSON API vs HTMX (different VMs). If VM is just `Todos`, either place works — we choose handler for flexibility.

**Connects to:** `net/http` in, `Service` middle, `templates` out.

### 5. Renderer — `templates/todos.templ`

```go
templ Page(vm todo.PageVM) // full <!DOCTYPE html> + @List(todo.ListVM{Todos: vm.Todos})
templ List(vm todo.ListVM) // fragment <ul id="todo-list">
templ Item(t todo.Todo)
```

Only layer that imports `templ`. Takes VM, renders HTML. No logic.
`Todo` stays tag-free in `todo` (no `json`/`db` tags) — `db.Repo` holds `[]todo.Todo` directly, no redeclaration. Add tags later only if you add JSON API, not needed for HTMX.

**Why handler/register split avoids cycle:** `todo` (service/vm) imports nothing; `db` imports `todo` to implement `todo.Repository`; `templates` imports `todo` for `Todo`/`VM`; `handler` (`internal/todo/handler`) imports `todo`+`templates`. No `todo -> db -> todo` cycle in prod, and tests use local `fakeRepo` to avoid `todo_test -> db -> todo` cycle.

---

## Request Flows

**GET / — full page**

```
GET / → handler.Register → handlePage → svc.List() → repo.List() → []Todo
→ handler builds PageVM{Todos: todos} → templates.Page(vm).Render → HTML doc
```

**POST /todos — HTMX fragment**

```
POST /todos title=buy+milk → handleAdd → svc.Add() → ValidateTitle → repo.Save → repo.List
→ handler builds ListVM{Todos: todos} → templates.List(vm).Render → <ul id="todo-list">...
→ htmx swaps innerHTML of #todo-list (form has hx-target="#todo-list" hx-swap="innerHTML")
```

No `HX-Request` check. Form declares fragment via `hx-target`, server has dedicated fragment endpoint.

---

## Why VM per renderer?

Not one god `ViewModel`. `ListVM` for `List`, `PageVM` for `Page`, `ItemVM{T Todo; CanEdit bool}` for per-row swap. Handler builds small VM from domain. Today `PageVM{Todos []Todo}` looks redundant vs `[]Todo`, but when you need `Total`, `DoneCount`, `Flash`, VM is the seam without leaking rendering into `Service`.

---

## Testing

| Layer | How | Speed |
|-------|-----|-------|
| **Repo** | In-memory, no DB | 0ms |
| **Service + pure funcs** | `go test ./internal/todo -run TestService` | 0ms |
| **Handler (optional)** | `httptest.NewRequest` + `handler.Register` + fake `Repo` | 1ms |
| **Renderer** | Golden files or eyeball | 1ms |

Run: `go test ./... -v` — proves app without HTTP/HTML.

---

## Folder Structure

```
root/
├── go.mod
├── main.go                          // wiring only: db.New() → todo.NewService → handler.Register
├── internal/db/
│   └── repo.go                      // GLOBAL *Repo pointer, mutex, in-memory, implements todo.Repository (+ future domains)
├── internal/todo/
│   ├── service.go                   // Todo model (no tags) + Repository PORT + Service{repo Repository} + ValidateTitle pure
│   ├── vm.go                        // ListVM, PageVM
│   ├── handler/handler.go           // Register + 4 handlers building VMs
│   └── todo_test.go                 // fakeRepo (no db import) + pure Service + VM tests
└── templates/
    ├── todos.templ                  // Page(PageVM), List(ListVM), Item(Todo)
    └── todos_templ.go               // generated
```

`internal/` enforces decoupling. Global `db.Repo` is one `*Repo` shared; handler subpackage breaks cycle.

---

## Transactions

With GLOBAL `*db.Repo`, add one helper:

```go
// internal/db/repo.go (when using real DB)
func (r *Repo) WithTx(ctx context.Context, fn func(*Repo) error) error {
  tx, _ := r.pool.Begin(ctx)
  txRepo := &Repo{pool: tx} // new per-request instance, not shared
  defer tx.Rollback(ctx)
  if err := fn(txRepo); err != nil { return err }
  return tx.Commit(ctx)
}
```

Cross-domain with Tx (when you add `users` to same global Repo):

```go
repo.WithTx(ctx, func(tx *db.Repo) error {
  if err := tx.DeleteUser(ctx, id); err != nil { return err }
  return tx.DeleteTodosByUser(ctx, id)
})
```


---


## Commands

```bash
templ generate
go run .                # http://localhost:8080
go test ./... -v
go vet ./...            # 0
```


// Package actions composes domain services into named cross-domain
// behaviors. An action is a plain function over the seams it needs —
// no registry struct, no state of its own — so its whole decision matrix
// is testable as plain function tests with in-memory fakes, and the only
// thing the real stack still has to prove is the SQL itself.
//
// The rule of thumb that keeps this from bloat: an action earns its own
// existence by touching two or more domains. One domain → it belongs in
// that domain's service. Two+ → it goes here, as a bare function whose
// arguments are the concrete services it actually calls.
//
// Handlers stay thin: they parse HTTP and call one service or one action.
// Actions never parse HTTP and never touch storage directly; if one
// starts to need its own queries, that is the signal the boundary was
// drawn wrong — move it into a domain service instead.
package actions

import (
	"context"
	"fmt"
	"net/http"

	db "go-htmx-todo/internal/db/sqlc"
	"go-htmx-todo/internal/httpx"
	"go-htmx-todo/internal/todo"
	"go-htmx-todo/templates"
)

// Register wires the action endpoints onto the mux. Call from main.go.
//
// Takes the ready services main already built — actions are generic over
// their arguments, and the composition root is the only place that
// constructs anything.
func Register(mux *http.ServeMux, todos *todo.Service) {
	mux.HandleFunc("POST /todos/complete-all", handleCompleteAll(todos))
	mux.HandleFunc("POST /todos/clear-completed", handleClearCompleted(todos))
}

// CompleteAll marks every open todo done and returns the resulting list.
//
// The sample single-domain action: it takes only the todo service and
// composes its existing calls (List → Toggle → List) into one named
// behavior. Its whole decision matrix — nothing open, one open, many
// open, store failure mid-way — is testable as plain function tests over
// the shared fake in internal/todo/todotest: no database, no HTTP.
func CompleteAll(ctx context.Context, todos *todo.Service) ([]db.Todo, error) {
	list, err := todos.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("complete all: %w", err)
	}
	for _, t := range list {
		if t.Done {
			continue
		}
		if _, err := todos.Toggle(ctx, t.ID); err != nil {
			return nil, fmt.Errorf("complete todo %d: %w", t.ID, err)
		}
	}
	return todos.List(ctx)
}

// ClearCompleted deletes every done todo and returns the remaining list.
//
// Same pattern, second sample: compose the service, name the behavior,
// return the domain value the caller renders. When a second domain shows
// up (users), an action like this simply takes both services as arguments —
// DeleteAccount(ctx, users, todos, userID) — and the signature itself
// documents exactly which domains the behavior touches.
func ClearCompleted(ctx context.Context, todos *todo.Service) ([]db.Todo, error) {
	list, err := todos.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("clear completed: %w", err)
	}
	for _, t := range list {
		if !t.Done {
			continue
		}
		if _, err := todos.Delete(ctx, t.ID); err != nil {
			return nil, fmt.Errorf("delete todo %d: %w", t.ID, err)
		}
	}
	return todos.List(ctx)
}

// --- HTTP: parse request, call the action, render the fragment. ---

func handleCompleteAll(todos *todo.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		out, err := CompleteAll(r.Context(), todos)
		if err != nil {
			http.Error(w, err.Error(), httpx.StatusFor(err))
			return
		}
		if err := templates.List(todo.ListVM{Todos: out}).Render(r.Context(), w); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}

func handleClearCompleted(todos *todo.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		out, err := ClearCompleted(r.Context(), todos)
		if err != nil {
			http.Error(w, err.Error(), httpx.StatusFor(err))
			return
		}
		if err := templates.List(todo.ListVM{Todos: out}).Render(r.Context(), w); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}

// Package todo holds the todo domain's routes and handlers.
package todo

import (
	"errors"
	"net/http"
	"strings"

	db "go-htmx-todo/internal/db/sqlc"
	"go-htmx-todo/internal/handlers"
	"go-htmx-todo/templates"

	"github.com/jackc/pgx/v5"
)

// Register wires the todo routes onto mux.
func Register(mux *http.ServeMux, d handlers.Deps) {
	mux.HandleFunc("GET /{$}", handlePage(d))
	mux.HandleFunc("POST /todos", handleAdd(d))
	mux.HandleFunc("POST /todos/{id}/toggle", handleToggle(d))
	mux.HandleFunc("DELETE /todos/{id}", handleDelete(d))
	mux.HandleFunc("POST /todos/complete-all", handleCompleteAll(d))
	mux.HandleFunc("POST /todos/clear-completed", handleClearCompleted(d))
}

func handlePage(d handlers.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		todos, err := d.Q.ListTodos(r.Context(), handlers.CurrentUserID(r, d))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := templates.Page(todos).Render(r.Context(), w); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}

func handleAdd(d handlers.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		title := strings.TrimSpace(r.FormValue("title"))
		if title == "" {
			http.Error(w, "title cannot be empty", http.StatusUnprocessableEntity)
			return
		}
		ctx := r.Context()
		userID := handlers.CurrentUserID(r, d)
		if _, err := d.Q.CreateTodo(ctx, db.CreateTodoParams{UserID: userID, Title: title}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		todos, err := d.Q.ListTodos(ctx, userID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := templates.List(todos).Render(ctx, w); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}

func handleToggle(d handlers.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := handlers.ParseID(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		ctx := r.Context()
		userID := handlers.CurrentUserID(r, d)
		t, err := d.Q.GetTodo(ctx, db.GetTodoParams{ID: id, UserID: userID})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "todo not found", http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := d.Q.UpdateTodo(ctx, db.UpdateTodoParams{
			ID: t.ID, UserID: t.UserID, Title: t.Title, Done: !t.Done,
		}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		todos, err := d.Q.ListTodos(ctx, userID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := templates.List(todos).Render(ctx, w); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}

func handleDelete(d handlers.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := handlers.ParseID(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		ctx := r.Context()
		userID := handlers.CurrentUserID(r, d)
		if err := d.Q.DeleteTodo(ctx, db.DeleteTodoParams{ID: id, UserID: userID}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		todos, err := d.Q.ListTodos(ctx, userID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := templates.List(todos).Render(ctx, w); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}

func handleCompleteAll(d handlers.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		userID := handlers.CurrentUserID(r, d)
		todos, err := d.Q.ListTodos(ctx, userID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		for _, t := range todos {
			if t.Done {
				continue
			}
			if err := d.Q.UpdateTodo(ctx, db.UpdateTodoParams{
				ID: t.ID, UserID: t.UserID, Title: t.Title, Done: true,
			}); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
		todos, err = d.Q.ListTodos(ctx, userID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := templates.List(todos).Render(ctx, w); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}

func handleClearCompleted(d handlers.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		userID := handlers.CurrentUserID(r, d)
		todos, err := d.Q.ListTodos(ctx, userID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		for _, t := range todos {
			if !t.Done {
				continue
			}
			if err := d.Q.DeleteTodo(ctx, db.DeleteTodoParams{ID: t.ID, UserID: userID}); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
		todos, err = d.Q.ListTodos(ctx, userID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := templates.List(todos).Render(ctx, w); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}

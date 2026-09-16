// Package todo holds the todo domain's routes and handlers.
package todo

import (
	"errors"
	"net/http"
	"strings"

	db "go-htmx-todo/internal/db/sqlc"
	"go-htmx-todo/internal/handlers"
	"go-htmx-todo/internal/handlers/auth"
	"go-htmx-todo/templates"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel/attribute"
)

// Register wires the todo routes onto mux. Mutations require identity;
// the page is public and renders sign-in links when anonymous.
func Register(mux *http.ServeMux, d handlers.Deps) {
	handlers.Route(mux, "GET /{$}", auth.WithAuth(handlePage(d), d))
	handlers.Route(mux, "POST /todos", auth.RequireAuth(handleAdd(d), d))
	handlers.Route(mux, "POST /todos/{id}/toggle", auth.RequireAuth(handleToggle(d), d))
	handlers.Route(mux, "DELETE /todos/{id}", auth.RequireAuth(handleDelete(d), d))
	handlers.Route(mux, "POST /todos/complete-all", auth.RequireAuth(handleCompleteAll(d), d))
	handlers.Route(mux, "POST /todos/clear-completed", auth.RequireAuth(handleClearCompleted(d), d))
}

func handlePage(d handlers.Deps) auth.AuthedHandler {
	return func(w http.ResponseWriter, r *http.Request, a auth.AuthData) {
		ctx, span := d.Tel.Tracer.Start(r.Context(), "todo.list")
		defer span.End()
		todos, err := d.Queries.ListTodos(ctx, a.UserID)
		if err != nil {
			handlers.WriteError(w, r, d.Logger, err, http.StatusInternalServerError)
			return
		}
		if err := templates.Page(todos, a.Username).Render(r.Context(), w); err != nil {
			handlers.WriteError(w, r, d.Logger, err, http.StatusInternalServerError)
		}
		span.AddEvent("template.rendered")
	}
}

func handleAdd(d handlers.Deps) auth.AuthedHandler {
	return func(w http.ResponseWriter, r *http.Request, a auth.AuthData) {
		title := strings.TrimSpace(r.FormValue("title"))
		if title == "" {
			handlers.WriteError(w, r, d.Logger, errors.New("title cannot be empty"), http.StatusUnprocessableEntity)
			return
		}
		ctx := r.Context()
		if _, err := d.Queries.CreateTodo(ctx, db.CreateTodoParams{UserID: a.UserID, Title: title}); err != nil {
			handlers.WriteError(w, r, d.Logger, err, http.StatusInternalServerError)
			return
		}
		todos, err := d.Queries.ListTodos(ctx, a.UserID)
		if err != nil {
			handlers.WriteError(w, r, d.Logger, err, http.StatusInternalServerError)
			return
		}
		if err := templates.List(todos).Render(ctx, w); err != nil {
			handlers.WriteError(w, r, d.Logger, err, http.StatusInternalServerError)
		}
	}
}

func handleToggle(d handlers.Deps) auth.AuthedHandler {
	return func(w http.ResponseWriter, r *http.Request, a auth.AuthData) {
		id, err := handlers.ParseID(r)
		if err != nil {
			handlers.WriteError(w, r, d.Logger, err, http.StatusBadRequest)
			return
		}
		ctx := r.Context()
		t, err := d.Queries.GetTodo(ctx, db.GetTodoParams{ID: id, UserID: a.UserID})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				handlers.WriteError(w, r, d.Logger, errors.New("todo not found"), http.StatusNotFound)
				return
			}
			handlers.WriteError(w, r, d.Logger, err, http.StatusInternalServerError)
			return
		}
		if err := d.Queries.UpdateTodo(ctx, db.UpdateTodoParams{
			ID: t.ID, UserID: t.UserID, Title: t.Title, Done: !t.Done,
		}); err != nil {
			handlers.WriteError(w, r, d.Logger, err, http.StatusInternalServerError)
			return
		}
		todos, err := d.Queries.ListTodos(ctx, a.UserID)
		if err != nil {
			handlers.WriteError(w, r, d.Logger, err, http.StatusInternalServerError)
			return
		}
		if err := templates.List(todos).Render(ctx, w); err != nil {
			handlers.WriteError(w, r, d.Logger, err, http.StatusInternalServerError)
		}
	}
}

func handleDelete(d handlers.Deps) auth.AuthedHandler {
	return func(w http.ResponseWriter, r *http.Request, a auth.AuthData) {
		id, err := handlers.ParseID(r)
		if err != nil {
			handlers.WriteError(w, r, d.Logger, err, http.StatusBadRequest)
			return
		}
		ctx, span := d.Tel.Tracer.Start(r.Context(), "db.delete")
		defer span.End()
		span.SetAttributes(attribute.Int64("id", id))

		if err := d.Queries.DeleteTodo(ctx, db.DeleteTodoParams{ID: id, UserID: a.UserID}); err != nil {
			span.End()
			handlers.WriteError(w, r, d.Logger, err, http.StatusInternalServerError)
			return
		}

		ctx, span = d.Tel.Tracer.Start(r.Context(), "db.list")
		defer span.End()
		todos, err := d.Queries.ListTodos(ctx, a.UserID)
		if err != nil {
			handlers.WriteError(w, r, d.Logger, err, http.StatusInternalServerError)
			return
		}
		if err := templates.List(todos).Render(ctx, w); err != nil {
			handlers.WriteError(w, r, d.Logger, err, http.StatusInternalServerError)
		}
	}
}

func handleCompleteAll(d handlers.Deps) auth.AuthedHandler {
	return func(w http.ResponseWriter, r *http.Request, a auth.AuthData) {
		ctx := r.Context()
		todos, err := d.Queries.ListTodos(ctx, a.UserID)
		if err != nil {
			handlers.WriteError(w, r, d.Logger, err, http.StatusInternalServerError)
			return
		}
		for _, t := range todos {
			if t.Done {
				continue
			}
			if err := d.Queries.UpdateTodo(ctx, db.UpdateTodoParams{
				ID: t.ID, UserID: t.UserID, Title: t.Title, Done: true,
			}); err != nil {
				handlers.WriteError(w, r, d.Logger, err, http.StatusInternalServerError)
				return
			}
		}
		todos, err = d.Queries.ListTodos(ctx, a.UserID)
		if err != nil {
			handlers.WriteError(w, r, d.Logger, err, http.StatusInternalServerError)
			return
		}
		if err := templates.List(todos).Render(ctx, w); err != nil {
			handlers.WriteError(w, r, d.Logger, err, http.StatusInternalServerError)
		}
	}
}

func handleClearCompleted(d handlers.Deps) auth.AuthedHandler {
	return func(w http.ResponseWriter, r *http.Request, a auth.AuthData) {
		ctx := r.Context()
		todos, err := d.Queries.ListTodos(ctx, a.UserID)
		if err != nil {
			handlers.WriteError(w, r, d.Logger, err, http.StatusInternalServerError)
			return
		}
		for _, t := range todos {
			if !t.Done {
				continue
			}
			if err := d.Queries.DeleteTodo(ctx, db.DeleteTodoParams{ID: t.ID, UserID: a.UserID}); err != nil {
				handlers.WriteError(w, r, d.Logger, err, http.StatusInternalServerError)
				return
			}
		}
		todos, err = d.Queries.ListTodos(ctx, a.UserID)
		if err != nil {
			handlers.WriteError(w, r, d.Logger, err, http.StatusInternalServerError)
			return
		}
		if err := templates.List(todos).Render(ctx, w); err != nil {
			handlers.WriteError(w, r, d.Logger, err, http.StatusInternalServerError)
		}
	}
}

package handler

import (
	"fmt"
	"net/http"
	"strconv"

	"go-htmx-todo/internal/httpx"
	"go-htmx-todo/internal/todo"
	"go-htmx-todo/templates"
)

// Register wires all todo routes. Call from main.go.
// main builds the service; this package only closes over it and owns its routes.
func Register(mux *http.ServeMux, svc *todo.Service) {
	mux.HandleFunc("GET /{$}", handlePage(svc))
	mux.HandleFunc("POST /todos", handleAdd(svc))
	mux.HandleFunc("POST /todos/{id}/toggle", handleToggle(svc))
	mux.HandleFunc("DELETE /todos/{id}", handleDelete(svc))
}

func handlePage(svc *todo.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		todos, err := svc.List(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := templates.Page(todo.PageVM{Todos: todos}).Render(r.Context(), w); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}

func handleAdd(svc *todo.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		todos, err := svc.Add(r.Context(), r.FormValue("title"))
		if err != nil {
			http.Error(w, err.Error(), httpx.StatusFor(err))
			return
		}
		if err := templates.List(todo.ListVM{Todos: todos}).Render(r.Context(), w); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}

func handleToggle(svc *todo.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseID(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		todos, err := svc.Toggle(r.Context(), id)
		if err != nil {
			http.Error(w, err.Error(), httpx.StatusFor(err))
			return
		}
		if err := templates.List(todo.ListVM{Todos: todos}).Render(r.Context(), w); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}

func handleDelete(svc *todo.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseID(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		todos, err := svc.Delete(r.Context(), id)
		if err != nil {
			http.Error(w, err.Error(), httpx.StatusFor(err))
			return
		}
		if err := templates.List(todo.ListVM{Todos: todos}).Render(r.Context(), w); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}

func parseID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		return 0, fmt.Errorf("invalid todo id")
	}
	return id, nil
}
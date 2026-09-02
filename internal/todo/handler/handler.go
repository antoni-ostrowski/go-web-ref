package handler

import (
	"fmt"
	"net/http"
	"strconv"

	db "go-htmx-todo/internal/db/sqlc"
	"go-htmx-todo/internal/todo"
	"go-htmx-todo/templates"
)

// Register wires all todo routes. Call from main.go.
// Domain owns its routes and creates its service; main stays composition root.
func Register(mux *http.ServeMux, queries *db.Queries) {
	svc := todo.NewService(queries)
	mux.HandleFunc("GET /{$}", handlePage(svc))
	mux.HandleFunc("POST /todos", handleAdd(svc))
	mux.HandleFunc("POST /todos/{id}/toggle", handleToggle(svc))
	mux.HandleFunc("DELETE /todos/{id}", handleDelete(svc))
}

func handlePage(svc *todo.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vm, err := svc.List(r.Context())
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		if err := templates.Page(vm).Render(r.Context(), w); err != nil {
			http.Error(w, err.Error(), 500)
		}
	}
}

func handleAdd(svc *todo.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		title := r.FormValue("title")
		vm, err := svc.Add(r.Context(), title)
		if err != nil {
			http.Error(w, err.Error(), 422)
			return
		}
		if err := templates.List(vm).Render(r.Context(), w); err != nil {
			http.Error(w, err.Error(), 500)
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
		vm, err := svc.Toggle(r.Context(), id)
		if err != nil {
			http.Error(w, err.Error(), 404)
			return
		}
		if err := templates.List(vm).Render(r.Context(), w); err != nil {
			http.Error(w, err.Error(), 500)
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
		vm, err := svc.Delete(r.Context(), id)
		if err != nil {
			http.Error(w, err.Error(), 404)
			return
		}
		if err := templates.List(vm).Render(r.Context(), w); err != nil {
			http.Error(w, err.Error(), 500)
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

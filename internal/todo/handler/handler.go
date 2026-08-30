package handler

import (
	"net/http"

	"go-htmx-todo/internal/todo"
	"go-htmx-todo/templates"
)

// Register wires all todo routes. Call from main.go.
// Domain owns its routes, main stays composition root.
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
			http.Error(w, err.Error(), 500)
			return
		}
		vm := todo.PageVM{Todos: todos}
		if err := templates.Page(vm).Render(r.Context(), w); err != nil {
			http.Error(w, err.Error(), 500)
		}
	}
}

func handleAdd(svc *todo.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		title := r.FormValue("title")
		todos, err := svc.Add(r.Context(), title)
		if err != nil {
			http.Error(w, err.Error(), 422)
			return
		}
		vm := todo.ListVM{Todos: todos}
		if err := templates.List(vm).Render(r.Context(), w); err != nil {
			http.Error(w, err.Error(), 500)
		}
	}
}

func handleToggle(svc *todo.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		todos, err := svc.Toggle(r.Context(), id)
		if err != nil {
			http.Error(w, err.Error(), 404)
			return
		}
		vm := todo.ListVM{Todos: todos}
		if err := templates.List(vm).Render(r.Context(), w); err != nil {
			http.Error(w, err.Error(), 500)
		}
	}
}

func handleDelete(svc *todo.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		todos, err := svc.Delete(r.Context(), id)
		if err != nil {
			http.Error(w, err.Error(), 404)
			return
		}
		vm := todo.ListVM{Todos: todos}
		if err := templates.List(vm).Render(r.Context(), w); err != nil {
			http.Error(w, err.Error(), 500)
		}
	}
}

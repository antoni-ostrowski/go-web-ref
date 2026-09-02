package todo

import db "go-htmx-todo/internal/db/sqlc"

// ViewModels — UI-shaped. One per renderer. Handler builds them.
// Service never returns VM, only domain []Todo.
type ListVM struct {
	Todos []db.Todo
}

type PageVM struct {
	Todos []db.Todo
}

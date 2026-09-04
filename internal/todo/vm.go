package todo

import db "go-htmx-todo/internal/db/sqlc"

// ViewModels — UI-shaped, one per renderer. Handlers build these from the
// domain values the service returns. The service never returns VMs; it
// returns []db.Todo so domain logic and its tests stay UI-agnostic.
type ListVM struct {
	Todos []db.Todo
}

type PageVM struct {
	Todos []db.Todo
}
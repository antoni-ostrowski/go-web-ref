package todo

// ViewModels — UI-shaped. One per renderer. Handler builds them.
// Service never returns VM, only domain []Todo.
type ListVM struct {
	Todos []Todo
}

type PageVM struct {
	Todos []Todo
}

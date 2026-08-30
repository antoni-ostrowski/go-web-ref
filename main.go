package main

import (
	"log"
	"net/http"

	"go-htmx-todo/internal/db"
	"go-htmx-todo/internal/todo"
	"go-htmx-todo/internal/todo/handler"
)

func main() {
	repo := db.New() // GLOBAL single DB repo *db.Repo — shared across all domains
	svc := todo.NewService(repo)

	mux := http.NewServeMux()
	handler.Register(mux, svc) // domain owns routes

	log.Println("listening on http://localhost:8080")
	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatal(err)
	}
}

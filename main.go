package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"

	db "go-htmx-todo/internal/db/sqlc"
	"go-htmx-todo/internal/todo/handler"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, nil)))

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		databaseURL = "postgres://postgres:postgres@localhost:5432/todos?sslmode=disable"
	}

	pool, err := pgxpool.New(context.Background(), databaseURL)
	if err != nil {
		slog.Error("create database pool", "error", err)
		return
	}
	defer pool.Close()

	queries := db.New(pool)

	mux := http.NewServeMux()
	handler.Register(mux, queries) // domain creates service and owns routes

	slog.Info("listening", "address", "http://localhost:8080")
	if err := http.ListenAndServe(":8080", mux); err != nil {
		slog.Error("server stopped", "error", err)
	}
}

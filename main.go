package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"time"

	"go-htmx-todo/internal/actions"
	db "go-htmx-todo/internal/db/sqlc"
	"go-htmx-todo/internal/todo"
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
	todos := todo.NewService(queries) // built once, shared by every Register

	mux := http.NewServeMux()
	handler.Register(mux, todos) // domain routes
	actions.Register(mux, todos) // cross-domain action routes

	srv := http.Server{
		Addr:              ":3000",
		ReadHeaderTimeout: 10 * time.Second,
		Handler:           mux,
	}
	slog.Info("listening", "port", srv.Addr)
	if err := srv.ListenAndServe(); err != nil {
		slog.Error("server stopped", "error", err)
	}
}

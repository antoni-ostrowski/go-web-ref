package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"time"

	"go-htmx-todo/internal/handlers"
	"go-htmx-todo/internal/handlers/todo"
	db "go-htmx-todo/internal/db/sqlc"

	"github.com/alexedwards/scs/pgxstore"
	"github.com/alexedwards/scs/v2"
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

	// Composition root: one Deps, built once, handed to every domain.
	sessions := scs.New()
	sessions.Store = pgxstore.New(pool)
	sessions.Lifetime = 7 * 24 * 60 * time.Minute

	deps := handlers.Deps{Q: db.New(pool), Sessions: sessions}

	mux := http.NewServeMux()
	todo.Register(mux, deps)

	slog.Info("listening", "address", "http://localhost:8080")
	if err := http.ListenAndServe(":8080", sessions.LoadAndSave(mux)); err != nil {
		slog.Error("server stopped", "error", err)
	}
}

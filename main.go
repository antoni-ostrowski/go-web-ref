package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	db "go-htmx-todo/internal/db/sqlc"
	"go-htmx-todo/internal/handlers"
	"go-htmx-todo/internal/handlers/auth"
	"go-htmx-todo/internal/handlers/static"
	"go-htmx-todo/internal/handlers/todo"

	"github.com/alexedwards/scs/pgxstore"
	"github.com/alexedwards/scs/v2"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))

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
	auth.Register(mux, deps)
	static.Register(mux, "static")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)

	srv := &http.Server{
		Addr:              ":8080",
		Handler:           sessions.LoadAndSave(mux),
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      10 * time.Second,
	}

	srvErr := make(chan error, 1)
	go func() {
		slog.Info("listening", "address", "http://localhost:8080")
		srvErr <- srv.ListenAndServe()
	}()

	select {
	case err := <-srvErr:
		// Startup failed: nothing to drain, and the pool never served
		// traffic, so exiting directly is safe.
		stop()
		if !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server error", "err", err)
			os.Exit(1)
		}
	case <-ctx.Done():
		// First signal: stop listening for more, so a second Ctrl+C
		// kills immediately. Drain in-flight work within budget.
		stop()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdown); err != nil {
			slog.Error("shutdown error", "err", err)
		}
		slog.Info("stopped")
	}
}

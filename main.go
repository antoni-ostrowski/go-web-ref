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
	"go-htmx-todo/internal/obs"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
)

const APP_NAME = "go-htmx-template"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)

	otelShutdown, err := obs.SetupOTelSDK(ctx, APP_NAME)
	if err != nil && !errors.Is(err, obs.ErrNoEndpoint) {
		stop()
		slog.Error("setup otel SDK", "error", err)
		return
	}

	logger := slog.New(obs.NewLogHandler(APP_NAME))
	slog.SetDefault(logger)
	if err != nil {
		logger.Warn("otel disabled", "reason", "OTEL_EXPORTER_OTLP_ENDPOINT not set")
	}
	defer func() {
		if otelShutdown == nil {
			return
		}
		if err := otelShutdown(context.Background()); err != nil {
			logger.Error("shutdown otel SDK", "error", err)
		}
	}()

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		databaseURL = "postgres://postgres:postgres@localhost:5432/todos?sslmode=disable"
	}

	pool, err := pgxpool.New(context.Background(), databaseURL)
	if err != nil {
		logger.Error("create database pool", "error", err)
		return
	}
	defer pool.Close()

	sessions := auth.NewSessionManager(pool)

	deps := handlers.Deps{
		Queries:  db.New(pool),
		Sessions: sessions, Logger: logger,
		Tel: &handlers.Telemetry{Tracer: otel.Tracer(APP_NAME), Meter: otel.Meter(APP_NAME)},
	}

	mux := http.NewServeMux()
	todo.Register(mux, deps)
	auth.Register(mux, deps)
	static.Register(mux, "static")

	srv := &http.Server{
		Addr:              ":8080",
		Handler:           otelhttp.NewHandler(sessions.LoadAndSave(mux), "server"),
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      10 * time.Second,
	}

	srvErr := make(chan error, 1)
	go func() {
		logger.Info("listening", "address", "http://localhost:8080")
		srvErr <- srv.ListenAndServe()
	}()

	select {
	case err := <-srvErr:
		// Startup failed: nothing to drain, and the pool never served
		// traffic, so exiting directly is safe.
		stop()
		if !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server error", "err", err)
			os.Exit(1)
		}
	case <-ctx.Done():
		// First signal: stop listening for more, so a second Ctrl+C
		// kills immediately. Drain in-flight work within budget.
		stop()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdown); err != nil {
			logger.Error("shutdown error", "err", err)
		}
		logger.Info("stopped")
	}
}

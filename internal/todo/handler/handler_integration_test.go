package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	db "go-htmx-todo/internal/db/sqlc"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

func TestHandlersWithPostgres(t *testing.T) {
	if os.Getenv("INTEGRATION_TESTS") != "1" {
		t.Skip("set INTEGRATION_TESTS=1 to run PostgreSQL integration tests")
	}

	ctx := context.Background()

	container, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("todos"),
		postgres.WithUsername("postgres"),
		postgres.WithPassword("postgres"),
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		t.Fatalf("start PostgreSQL container: %v", err)
	}
	t.Cleanup(func() { _ = container.Terminate(ctx) })

	connectionString, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("get connection string: %v", err)
	}
	pool, err := pgxpool.New(ctx, connectionString)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping PostgreSQL: %v", err)
	}

	schema, err := os.ReadFile("../../db/schema.sql")
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	if _, err := pool.Exec(ctx, string(schema)); err != nil {
		t.Fatalf("apply schema: %v", err)
	}

	mux := http.NewServeMux()
	Register(mux, db.New(pool))

	addBody := url.Values{"title": {"buy milk"}}.Encode()
	addRequest := httptest.NewRequest(http.MethodPost, "/todos", strings.NewReader(addBody))
	addRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	addResponse := httptest.NewRecorder()
	mux.ServeHTTP(addResponse, addRequest)
	if addResponse.Code != http.StatusOK || !strings.Contains(addResponse.Body.String(), "buy milk") {
		t.Fatalf("add response = %d %q", addResponse.Code, addResponse.Body.String())
	}

	pageResponse := httptest.NewRecorder()
	mux.ServeHTTP(pageResponse, httptest.NewRequest(http.MethodGet, "/", nil))
	if pageResponse.Code != http.StatusOK || !strings.Contains(pageResponse.Body.String(), "buy milk") {
		t.Fatalf("page response = %d %q", pageResponse.Code, pageResponse.Body.String())
	}

	toggleResponse := httptest.NewRecorder()
	mux.ServeHTTP(toggleResponse, httptest.NewRequest(http.MethodPost, "/todos/1/toggle", nil))
	if toggleResponse.Code != http.StatusOK || !strings.Contains(toggleResponse.Body.String(), `class="done"`) {
		t.Fatalf("toggle response = %d %q", toggleResponse.Code, toggleResponse.Body.String())
	}

	deleteResponse := httptest.NewRecorder()
	mux.ServeHTTP(deleteResponse, httptest.NewRequest(http.MethodDelete, "/todos/1", nil))
	if deleteResponse.Code != http.StatusOK || !strings.Contains(deleteResponse.Body.String(), "no todos yet") {
		t.Fatalf("delete response = %d %q", deleteResponse.Code, deleteResponse.Body.String())
	}
}

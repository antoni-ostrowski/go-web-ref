// Package handlers holds shared HTTP wiring: the Deps bag plus helpers
// every domain uses. Domains live in subpackages and import this package,
// never the reverse.
package handlers

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	db "go-htmx-todo/internal/db/sqlc"

	"github.com/alexedwards/scs/v2"
)

// Deps carries process-wide dependencies, built once by the caller.
// New shared deps join here as fields; never request-scoped data.
type Deps struct {
	Q        *db.Queries
	Sessions *scs.SessionManager
}

// WriteError logs err with request context and renders it. 4xx bodies carry
// the message (client-caused, safe); 5xx bodies stay generic while the real
// error goes to the log.
func WriteError(w http.ResponseWriter, r *http.Request, err error, code int) {
	slog.Error("handler error",
		"method", r.Method, "path", r.URL.Path, "status", code, "err", err)
	if code >= 500 {
		http.Error(w, http.StatusText(code), code)
		return
	}
	http.Error(w, err.Error(), code)
}

// ParseID reads the {id} path value as a positive integer.
func ParseID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		return 0, errors.New("invalid id")
	}
	return id, nil
}

// Package handlers holds shared HTTP wiring: the Deps bag plus helpers
// every domain uses. Domains live in subpackages and import this package,
// never the reverse.
package handlers

import (
	"errors"
	"net/http"
	"strconv"

	db "go-htmx-todo/internal/db/sqlc"

	"github.com/alexedwards/scs/v2"
	"github.com/google/uuid"
)

// Deps carries process-wide dependencies, built once by the caller.
// New shared deps join here as fields; never request-scoped data.
type Deps struct {
	Q        *db.Queries
	Sessions *scs.SessionManager
}

// CurrentUserID returns the session user, or uuid.Nil when signed out.
func CurrentUserID(r *http.Request, d Deps) uuid.UUID {
	id, err := uuid.Parse(d.Sessions.GetString(r.Context(), "user_id"))
	if err != nil {
		return uuid.Nil
	}
	return id
}

// ParseID reads the {id} path value as a positive integer.
func ParseID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		return 0, errors.New("invalid id")
	}
	return id, nil
}

// Package httpx holds small HTTP-layer helpers shared by every package
// that registers routes.
//
// It is the one place allowed to know every domain's sentinel errors —
// that is its whole job: mapping "why did this fail" (domain decision)
// onto "which HTTP status" (transport encoding).
package httpx

import (
	"errors"
	"net/http"

	"go-htmx-todo/internal/todo"
)

// StatusFor maps the domain packages' sentinel errors to HTTP statuses.
//
// One total function over all sentinels, in one place: every endpoint in
// the app maps errors identically, and a new domain's sentinels get one
// switch case here. Unknown errors (DB down, bugs) fall to 500. The only
// failure mode is a forgotten case when a domain adds a sentinel — the
// integration tests' error-path assertions (422, 404) catch that.
func StatusFor(err error) int {
	switch {
	case errors.Is(err, todo.ErrValidation):
		return http.StatusUnprocessableEntity
	case errors.Is(err, todo.ErrNotFound):
		return http.StatusNotFound
	default:
		return http.StatusInternalServerError
	}
}
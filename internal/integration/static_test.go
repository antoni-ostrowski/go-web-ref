package integration_test

import (
	"net/http"
	"strings"
	"testing"

	"go-htmx-todo/internal/handlertest"
)

// Static files serve with a JS content type and a body.
func TestStatic_ServesVendoredJS(t *testing.T) {
	app, _, _, _ := setup(t)

	rec := handlertest.Do(t, app, http.MethodGet, "/static/js/htmx.min.js", nil, nil)
	handlertest.WantCode(t, rec, http.StatusOK)
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "javascript") {
		t.Fatalf("Content-Type = %q, want javascript", ct)
	}
	if rec.Body.Len() == 0 {
		t.Fatal("empty body")
	}
}

// Directories don't list; traversal outside the root fails.
func TestStatic_NoListingNoTraversal(t *testing.T) {
	app, _, _, _ := setup(t)

	rec := handlertest.Do(t, app, http.MethodGet, "/static/js/", nil, nil)
	handlertest.WantCode(t, rec, http.StatusNotFound)

	rec = handlertest.Do(t, app, http.MethodGet, "/static/../go.mod", nil, nil)
	if rec.Code == http.StatusOK {
		t.Fatalf("status = 200 serving outside root, body starts %q", rec.Body.String()[:32])
	}
}

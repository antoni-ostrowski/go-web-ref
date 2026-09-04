// Package testutil holds small shared helpers for tests. Everything here is
// test-only: import it from _test.go files, never from production code.
package testutil

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// Do sends one in-memory request through mux and returns the recorded
// response. If form is non-nil it is encoded and sent as
// application/x-www-form-urlencoded (what HTMX form posts look like).
func Do(t *testing.T, mux *http.ServeMux, method, target string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req := httptest.NewRequest(method, target, body)
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

// WantCode asserts the response status. On mismatch the body is printed so
// the failure is debuggable without re-running anything.
func WantCode(t *testing.T, rec *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rec.Code != want {
		t.Fatalf("status = %d, want %d, body: %q", rec.Code, want, rec.Body.String())
	}
}

// WantBody asserts the response body contains each given substring — the
// data the service produced, a fragment marker, an htmx attribute. It does
// not assert exact HTML; see the README's "no golden files" decision.
func WantBody(t *testing.T, rec *httptest.ResponseRecorder, want ...string) {
	t.Helper()
	for _, s := range want {
		if !strings.Contains(rec.Body.String(), s) {
			t.Fatalf("body does not contain %q\nbody: %q", s, rec.Body.String())
		}
	}
}
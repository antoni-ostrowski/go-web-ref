package integration_test

import (
	"context"
	"net/http"
	"net/url"
	"testing"

	"go-htmx-todo/internal/handlers/auth"
	"go-htmx-todo/internal/handlertest"

	"github.com/google/uuid"
)

// seedNamedUser inserts a user with a known username and password
// ("password123"), for tests that sign in through the real form.
func seedNamedUser(t *testing.T, username string) uuid.UUID {
	t.Helper()
	hash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	id := uuid.New()
	if _, err := testPool(t).Exec(context.Background(),
		`INSERT INTO users (id, username, password_hash) VALUES ($1, $2, $3)`,
		id, username, hash); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return id
}

// doSignup posts the signup form, returning the response and the session
// cookie when one was set.
func doSignup(t *testing.T, app http.Handler, username, password string) (*http.Cookie, int, string) {
	t.Helper()
	rec := handlertest.Do(t, app, http.MethodPost, "/signup",
		url.Values{"username": {username}, "password": {password}}, nil)
	var cookie *http.Cookie
	if cookies := rec.Result().Cookies(); len(cookies) > 0 {
		cookie = cookies[0]
	}
	return cookie, rec.Code, rec.Header().Get("Location")
}

func userCount(t *testing.T) int {
	t.Helper()
	var n int
	if err := testPool(t).QueryRow(context.Background(), `SELECT count(*) FROM users`).Scan(&n); err != nil {
		t.Fatalf("count users: %v", err)
	}
	return n
}

// POST /signup stores a hashed password, logs the user in, and the session
// immediately works for todo mutations.
func TestSignup_CreatesUserAndLogsIn(t *testing.T) {
	app, q, _, _ := setup(t)

	cookie, code, location := doSignup(t, app, "alice", "password123")
	if code != http.StatusSeeOther || location != "/" {
		t.Fatalf("status = %d, location = %q, want 303 to /", code, location)
	}
	if cookie == nil {
		t.Fatal("no session cookie set on signup")
	}

	user, err := q.GetUserByUsername(context.Background(), "alice")
	if err != nil {
		t.Fatalf("GetUserByUsername: %v", err)
	}
	if user.PasswordHash == "password123" {
		t.Error("password stored in plaintext")
	}
	if !auth.CheckPassword(user.PasswordHash, "password123") {
		t.Error("stored hash does not verify")
	}

	rec := handlertest.Do(t, app, http.MethodPost, "/todos", url.Values{"title": {"first"}}, cookie)
	handlertest.WantCode(t, rec, http.StatusOK)
	handlertest.WantBody(t, rec, "first")

	rec = handlertest.Do(t, app, http.MethodGet, "/", nil, cookie)
	handlertest.WantCode(t, rec, http.StatusOK)
	handlertest.WantBody(t, rec, "alice")
}

// GET /signup and /signin render their forms.
func TestAuth_Pages(t *testing.T) {
	app, _, _, _ := setup(t)

	rec := handlertest.Do(t, app, http.MethodGet, "/signup", nil, nil)
	handlertest.WantCode(t, rec, http.StatusOK)
	handlertest.WantBody(t, rec, `action="/signup"`, `name="password"`)

	rec = handlertest.Do(t, app, http.MethodGet, "/signin", nil, nil)
	handlertest.WantCode(t, rec, http.StatusOK)
	handlertest.WantBody(t, rec, `action="/signin"`, `name="password"`)
}

// A taken username is 409 and stores nothing new.
func TestSignup_Duplicate(t *testing.T) {
	app, _, _, _ := setup(t)

	if _, code, _ := doSignup(t, app, "alice", "password123"); code != http.StatusSeeOther {
		t.Fatalf("first signup status = %d, want 303", code)
	}
	rec := handlertest.Do(t, app, http.MethodPost, "/signup",
		url.Values{"username": {"alice"}, "password": {"password123"}}, nil)
	handlertest.WantCode(t, rec, http.StatusConflict)
	handlertest.WantBody(t, rec, "taken")
	if n := userCount(t); n != 2 { // setup user + alice
		t.Fatalf("users = %d, want 2", n)
	}
}

// Blank username and short passwords are 422 and store nothing.
func TestSignup_Validation(t *testing.T) {
	app, _, _, _ := setup(t)

	for _, tt := range []struct{ username, password string }{
		{"", "password123"},
		{"   ", "password123"},
		{"bob", ""},
		{"bob", "short"},
	} {
		rec := handlertest.Do(t, app, http.MethodPost, "/signup",
			url.Values{"username": {tt.username}, "password": {tt.password}}, nil)
		handlertest.WantCode(t, rec, http.StatusUnprocessableEntity)
	}
	if n := userCount(t); n != 1 { // only the setup user
		t.Fatalf("users = %d, want 1", n)
	}
}

// Usernames are trimmed, so "  bob  " signs up (and in) as "bob".
func TestSignup_TrimsUsername(t *testing.T) {
	app, q, _, _ := setup(t)

	if _, code, _ := doSignup(t, app, "  bob  ", "password123"); code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", code)
	}
	if _, err := q.GetUserByUsername(context.Background(), "bob"); err != nil {
		t.Fatalf("GetUserByUsername(bob): %v", err)
	}
}

// POST /signin with correct credentials logs in through the real form.
func TestSignin(t *testing.T) {
	app, _, _, _ := setup(t)
	seedNamedUser(t, "carol")

	rec := handlertest.Do(t, app, http.MethodPost, "/signin",
		url.Values{"username": {"carol"}, "password": {"password123"}}, nil)
	handlertest.WantCode(t, rec, http.StatusSeeOther)
	if loc := rec.Header().Get("Location"); loc != "/" {
		t.Fatalf("location = %q, want /", loc)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("no session cookie set on signin")
	}

	mut := handlertest.Do(t, app, http.MethodPost, "/todos", url.Values{"title": {"via signin"}}, cookies[0])
	handlertest.WantCode(t, mut, http.StatusOK)
	handlertest.WantBody(t, mut, "via signin")
}

// Wrong password and unknown users are 401 without revealing which failed.
func TestSignin_RejectsBadCredentials(t *testing.T) {
	app, _, _, _ := setup(t)
	seedNamedUser(t, "carol")

	for _, tt := range []struct{ username, password string }{
		{"carol", "wrongpassword"},
		{"nobody", "password123"},
		{"", ""},
	} {
		rec := handlertest.Do(t, app, http.MethodPost, "/signin",
			url.Values{"username": {tt.username}, "password": {tt.password}}, nil)
		handlertest.WantCode(t, rec, http.StatusUnauthorized)
		handlertest.WantBody(t, rec, "invalid username or password")
	}
}

// POST /signout destroys the session; the old cookie stops working.
func TestSignout(t *testing.T) {
	app, _, _, _ := setup(t)

	cookie, code, _ := doSignup(t, app, "dave", "password123")
	if code != http.StatusSeeOther || cookie == nil {
		t.Fatalf("signup failed: status = %d, cookie = %v", code, cookie)
	}

	rec := handlertest.Do(t, app, http.MethodPost, "/signout", nil, cookie)
	handlertest.WantCode(t, rec, http.StatusSeeOther)
	if loc := rec.Header().Get("Location"); loc != "/signin" {
		t.Fatalf("location = %q, want /signin", loc)
	}

	rec = handlertest.Do(t, app, http.MethodPost, "/todos", url.Values{"title": {"x"}}, cookie)
	handlertest.WantCode(t, rec, http.StatusSeeOther)
	if loc := rec.Header().Get("Location"); loc != "/signin" {
		t.Fatalf("location = %q, want /signin", loc)
	}
}

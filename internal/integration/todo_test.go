package integration_test

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"testing"

	db "go-htmx-todo/internal/db/sqlc"
	"go-htmx-todo/internal/handlertest"

	"github.com/google/uuid"
)

// seedSecondUser inserts a second user for isolation tests.
func seedSecondUser(t *testing.T) uuid.UUID {
	t.Helper()
	return seedUser(t, testPool(t))
}

func mkTodo(user uuid.UUID, title string) db.CreateTodoParams {
	return db.CreateTodoParams{UserID: user, Title: title}
}

func itoa(id int64) string { return strconv.FormatInt(id, 10) }

// GET / with no todos renders the full page shell plus the empty state.
func TestPage_Empty(t *testing.T) {
	app, q, sessions, user := setup(t)
	cookie := handlertest.LoginAs(t, sessions, user)

	rec := handlertest.Do(t, app, http.MethodGet, "/", nil, cookie)
	handlertest.WantCode(t, rec, http.StatusOK)
	handlertest.WantBody(t, rec, "<!doctype html>", `id="todo-list"`, "no todos yet")

	if todos := listDB(t, q, user); len(todos) != 0 {
		t.Fatalf("db = %#v, want empty", todos)
	}
}

// GET / shows only the acting user's todos.
func TestPage_Isolation(t *testing.T) {
	app, q, sessions, user := setup(t)
	cookie := handlertest.LoginAs(t, sessions, user)

	other := seedSecondUser(t)
	if _, err := q.CreateTodo(context.Background(), mkTodo(other, "their-todo-xyz")); err != nil {
		t.Fatal(err)
	}
	if _, err := q.CreateTodo(context.Background(), mkTodo(user, "my-todo-xyz")); err != nil {
		t.Fatal(err)
	}

	rec := handlertest.Do(t, app, http.MethodGet, "/", nil, cookie)
	handlertest.WantCode(t, rec, http.StatusOK)
	handlertest.WantBody(t, rec, "my-todo-xyz")
	handlertest.WantNoBody(t, rec, "their-todo-xyz")
}

// POST /todos stores a trimmed row and renders the fragment with it.
func TestAdd_CreatesTodo(t *testing.T) {
	app, q, sessions, user := setup(t)
	cookie := handlertest.LoginAs(t, sessions, user)

	rec := handlertest.Do(t, app, http.MethodPost, "/todos", url.Values{"title": {"  buy milk  "}}, cookie)
	handlertest.WantCode(t, rec, http.StatusOK)
	handlertest.WantBody(t, rec, "buy milk", `id="todo-list"`)
	handlertest.WantNoBody(t, rec, "<!doctype html>") // fragment, not the full page

	todos := listDB(t, q, user)
	if len(todos) != 1 {
		t.Fatalf("db has %d todos, want 1: %#v", len(todos), todos)
	}
	if todos[0].Title != "buy milk" {
		t.Errorf("db title = %q, want trimmed %q", todos[0].Title, "buy milk")
	}
	if todos[0].Done {
		t.Error("db done = true, want false")
	}
	if todos[0].UserID != user {
		t.Errorf("db user = %v, want %v", todos[0].UserID, user)
	}
}

// POST /todos with a blank title is 422 and stores nothing.
func TestAdd_RejectsBlankTitle(t *testing.T) {
	app, q, sessions, user := setup(t)
	cookie := handlertest.LoginAs(t, sessions, user)

	for _, title := range []string{"", "   ", "\t\n "} {
		rec := handlertest.Do(t, app, http.MethodPost, "/todos", url.Values{"title": {title}}, cookie)
		handlertest.WantCode(t, rec, http.StatusUnprocessableEntity)
	}
	if todos := listDB(t, q, user); len(todos) != 0 {
		t.Fatalf("db = %#v, want empty after rejected adds", todos)
	}
}

// POST /todos/{id}/toggle flips done in the DB and renders the new state.
func TestToggle_FlipsDone(t *testing.T) {
	app, q, sessions, user := setup(t)
	cookie := handlertest.LoginAs(t, sessions, user)

	handlertest.Do(t, app, http.MethodPost, "/todos", url.Values{"title": {"alpha"}}, cookie)
	handlertest.Do(t, app, http.MethodPost, "/todos", url.Values{"title": {"bravo"}}, cookie)

	rec := handlertest.Do(t, app, http.MethodPost, "/todos/1/toggle", nil, cookie)
	handlertest.WantCode(t, rec, http.StatusOK)
	handlertest.WantBody(t, rec, `line-through`, "alpha", "bravo")

	todos := listDB(t, q, user)
	if len(todos) != 2 || !todos[0].Done || todos[1].Done {
		t.Fatalf("after toggle db = %#v", todos)
	}

	rec = handlertest.Do(t, app, http.MethodPost, "/todos/1/toggle", nil, cookie)
	handlertest.WantCode(t, rec, http.StatusOK)
	if todos := listDB(t, q, user); todos[0].Done {
		t.Fatalf("after second toggle db = %#v, want not done", todos)
	}
}

// Missing id is 404, malformed id is 400, another user's row is 404 with
// both users' rows untouched.
func TestToggle_Errors(t *testing.T) {
	app, q, sessions, user := setup(t)
	cookie := handlertest.LoginAs(t, sessions, user)

	handlertest.Do(t, app, http.MethodPost, "/todos", url.Values{"title": {"a"}}, cookie)

	rec := handlertest.Do(t, app, http.MethodPost, "/todos/999999/toggle", nil, cookie)
	handlertest.WantCode(t, rec, http.StatusNotFound)

	for _, target := range []string{"/todos/abc/toggle", "/todos/0/toggle", "/todos/-1/toggle"} {
		rec := handlertest.Do(t, app, http.MethodPost, target, nil, cookie)
		handlertest.WantCode(t, rec, http.StatusBadRequest)
	}

	other := seedSecondUser(t)
	otherTodo, err := q.CreateTodo(context.Background(), mkTodo(other, "not mine"))
	if err != nil {
		t.Fatal(err)
	}
	rec = handlertest.Do(t, app, http.MethodPost, "/todos/"+itoa(otherTodo.ID)+"/toggle", nil, cookie)
	handlertest.WantCode(t, rec, http.StatusNotFound)

	mine := listDB(t, q, user)
	if len(mine) != 1 || mine[0].Done {
		t.Fatalf("my db = %#v, want one open todo", mine)
	}
	theirs := listDB(t, q, other)
	if len(theirs) != 1 || theirs[0].Done {
		t.Fatalf("their db = %#v, want one open todo", theirs)
	}
}

// DELETE /todos/{id} removes only the targeted row.
func TestDelete_RemovesTargetedTodo(t *testing.T) {
	app, q, sessions, user := setup(t)
	cookie := handlertest.LoginAs(t, sessions, user)

	handlertest.Do(t, app, http.MethodPost, "/todos", url.Values{"title": {"alpha"}}, cookie)
	handlertest.Do(t, app, http.MethodPost, "/todos", url.Values{"title": {"bravo"}}, cookie)

	rec := handlertest.Do(t, app, http.MethodDelete, "/todos/1", nil, cookie)
	handlertest.WantCode(t, rec, http.StatusOK)
	handlertest.WantBody(t, rec, "bravo")
	handlertest.WantNoBody(t, rec, "alpha")

	todos := listDB(t, q, user)
	if len(todos) != 1 || todos[0].Title != "bravo" {
		t.Fatalf("db = %#v, want only bravo", todos)
	}
}

// DELETE is user-scoped and idempotent: foreign/missing rows render 200,
// changing nothing.
func TestDelete_IsolationAndIdempotent(t *testing.T) {
	app, q, sessions, user := setup(t)
	cookie := handlertest.LoginAs(t, sessions, user)

	other := seedSecondUser(t)
	otherTodo, err := q.CreateTodo(context.Background(), mkTodo(other, "not mine"))
	if err != nil {
		t.Fatal(err)
	}
	handlertest.Do(t, app, http.MethodPost, "/todos", url.Values{"title": {"mine"}}, cookie)

	rec := handlertest.Do(t, app, http.MethodDelete, "/todos/"+itoa(otherTodo.ID), nil, cookie)
	handlertest.WantCode(t, rec, http.StatusOK)
	if theirs := listDB(t, q, other); len(theirs) != 1 {
		t.Fatalf("their db = %#v, want untouched", theirs)
	}
	if mine := listDB(t, q, user); len(mine) != 1 {
		t.Fatalf("my db = %#v, want untouched", mine)
	}

	rec = handlertest.Do(t, app, http.MethodDelete, "/todos/999999", nil, cookie)
	handlertest.WantCode(t, rec, http.StatusOK)
	if mine := listDB(t, q, user); len(mine) != 1 {
		t.Fatalf("my db = %#v, want untouched after missing delete", mine)
	}
}

// POST /todos/complete-all marks every open todo done.
func TestCompleteAll(t *testing.T) {
	app, q, sessions, user := setup(t)
	cookie := handlertest.LoginAs(t, sessions, user)

	// Empty list is a no-op, not an error.
	rec := handlertest.Do(t, app, http.MethodPost, "/todos/complete-all", nil, cookie)
	handlertest.WantCode(t, rec, http.StatusOK)
	handlertest.WantBody(t, rec, "no todos yet")

	handlertest.Do(t, app, http.MethodPost, "/todos", url.Values{"title": {"a"}}, cookie)
	handlertest.Do(t, app, http.MethodPost, "/todos", url.Values{"title": {"b"}}, cookie)
	handlertest.Do(t, app, http.MethodPost, "/todos/2/toggle", nil, cookie) // b already done

	rec = handlertest.Do(t, app, http.MethodPost, "/todos/complete-all", nil, cookie)
	handlertest.WantCode(t, rec, http.StatusOK)
	handlertest.WantBody(t, rec, `line-through`)

	todos := listDB(t, q, user)
	if len(todos) != 2 || !todos[0].Done || !todos[1].Done {
		t.Fatalf("db = %#v, want all done", todos)
	}
}

// POST /todos/clear-completed deletes done todos and keeps open ones.
func TestClearCompleted(t *testing.T) {
	app, q, sessions, user := setup(t)
	cookie := handlertest.LoginAs(t, sessions, user)

	// Empty list is a no-op, not an error.
	rec := handlertest.Do(t, app, http.MethodPost, "/todos/clear-completed", nil, cookie)
	handlertest.WantCode(t, rec, http.StatusOK)

	handlertest.Do(t, app, http.MethodPost, "/todos", url.Values{"title": {"alpha"}}, cookie)
	handlertest.Do(t, app, http.MethodPost, "/todos", url.Values{"title": {"bravo"}}, cookie)
	handlertest.Do(t, app, http.MethodPost, "/todos", url.Values{"title": {"charlie"}}, cookie)
	handlertest.Do(t, app, http.MethodPost, "/todos/1/toggle", nil, cookie)
	handlertest.Do(t, app, http.MethodPost, "/todos/3/toggle", nil, cookie)

	rec = handlertest.Do(t, app, http.MethodPost, "/todos/clear-completed", nil, cookie)
	handlertest.WantCode(t, rec, http.StatusOK)
	handlertest.WantBody(t, rec, "bravo")
	handlertest.WantNoBody(t, rec, "alpha", "charlie")

	todos := listDB(t, q, user)
	if len(todos) != 1 || todos[0].Title != "bravo" {
		t.Fatalf("db = %#v, want only bravo", todos)
	}
}

// Anonymous htmx requests get 401 + HX-Redirect so the client navigates.
func TestRequireAuth_HtmxRedirect(t *testing.T) {
	app, _, _, _ := setup(t)

	rec := handlertest.DoHtmx(t, app, http.MethodPost, "/todos", url.Values{"title": {"x"}}, nil)
	handlertest.WantCode(t, rec, http.StatusUnauthorized)
	if h := rec.Header().Get("HX-Redirect"); h != "/signin" {
		t.Fatalf("HX-Redirect = %q, want /signin", h)
	}
}

// Authed htmx requests pass through to the handler.
func TestRequireAuth_HtmxAuthed(t *testing.T) {
	app, _, sessions, user := setup(t)
	cookie := handlertest.LoginAs(t, sessions, user)

	rec := handlertest.DoHtmx(t, app, http.MethodPost, "/todos", url.Values{"title": {"via htmx"}}, cookie)
	handlertest.WantCode(t, rec, http.StatusOK)
	handlertest.WantBody(t, rec, "via htmx")
}

// Anonymous requests see sign-in links; mutations redirect to sign-in.
func TestAnonymous(t *testing.T) {
	app, q, _, _ := setup(t)

	rec := handlertest.Do(t, app, http.MethodGet, "/", nil, nil)
	handlertest.WantCode(t, rec, http.StatusOK)
	handlertest.WantBody(t, rec, "no todos yet", "/signin")
	handlertest.WantNoBody(t, rec, `hx-post="/todos"`)

	rec = handlertest.Do(t, app, http.MethodPost, "/todos", url.Values{"title": {"x"}}, nil)
	handlertest.WantCode(t, rec, http.StatusSeeOther)
	if loc := rec.Header().Get("Location"); loc != "/signin" {
		t.Fatalf("location = %q, want /signin", loc)
	}

	if todos := listDB(t, q, uuid.Nil); len(todos) != 0 {
		t.Fatalf("db = %#v, want empty", todos)
	}
}

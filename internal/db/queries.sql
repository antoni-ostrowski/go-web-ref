-- name: ListTodos :many
SELECT id, title, done
FROM todos
ORDER BY id;

-- name: CreateTodo :one
INSERT INTO todos (title)
VALUES ($1)
RETURNING id, title, done;

-- name: GetTodo :one
SELECT id, title, done
FROM todos
WHERE id = $1;

-- name: UpdateTodo :exec
UPDATE todos
SET title = $2, done = $3
WHERE id = $1;

-- name: DeleteTodo :exec
DELETE FROM todos
WHERE id = $1;

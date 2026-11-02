-- name: CreateWorkspace :one
INSERT INTO workspaces (name, slug) VALUES ($1, $2) RETURNING *;

-- name: GetWorkspace :one
SELECT * FROM workspaces WHERE id = $1;

-- name: CountWorkspaces :one
SELECT count(*) FROM workspaces;

-- name: CreateUser :one
INSERT INTO users (workspace_id, name, email, initials, role_title, avatar_color, password_hash)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetUserByEmail :one
SELECT * FROM users WHERE lower(email) = lower($1);

-- name: ListUsers :many
SELECT * FROM users WHERE workspace_id = $1 ORDER BY name;

-- name: GetLocalUser :one
-- The user every request acts as while there is no login: the first one created.
SELECT * FROM users ORDER BY created_at, id LIMIT 1;

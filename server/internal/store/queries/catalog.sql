-- name: CreateProject :one
INSERT INTO projects (workspace_id, key, name, color, repo_url)
VALUES ($1, $2, $3, $4, $5) RETURNING *;

-- name: ListProjects :many
SELECT * FROM projects WHERE workspace_id = $1 ORDER BY created_at;

-- name: GetProject :one
SELECT * FROM projects WHERE id = $1 AND workspace_id = $2;

-- name: GetProjectByKey :one
SELECT * FROM projects WHERE workspace_id = $1 AND key = $2;

-- name: AllocateTicketNumber :one
-- Row lock on the project serializes concurrent creates.
UPDATE projects SET next_ticket_seq = next_ticket_seq + 1
WHERE id = $1 RETURNING next_ticket_seq - 1 AS number;

-- name: SetNextTicketSeq :exec
-- Seeding only: lets demo tickets keep the numbers from the mockups.
UPDATE projects SET next_ticket_seq = $2 WHERE id = $1;

-- name: CreateAgent :one
INSERT INTO agents (workspace_id, name, handle, color_index, role, specialty, model, status)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING *;

-- name: ListAgents :many
SELECT * FROM agents WHERE workspace_id = $1 ORDER BY color_index, name;

-- name: GetAgent :one
SELECT * FROM agents WHERE id = $1 AND workspace_id = $2;

-- name: CreateEpic :one
INSERT INTO epics (workspace_id, project_id, name, objective, owner_user_id, start_date, target_date, health)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING *;

-- name: ListEpics :many
SELECT * FROM epics WHERE workspace_id = $1 ORDER BY name;

-- name: GetEpic :one
SELECT * FROM epics WHERE id = $1 AND workspace_id = $2;

-- name: CreateLabel :one
INSERT INTO labels (workspace_id, name, color) VALUES ($1, $2, $3) RETURNING *;

-- name: ListLabels :many
SELECT * FROM labels WHERE workspace_id = $1 ORDER BY name;

-- name: GetUser :one
SELECT * FROM users WHERE id = $1 AND workspace_id = $2;

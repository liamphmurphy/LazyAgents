-- name: GetAgentByHandle :one
SELECT * FROM agents WHERE workspace_id = $1 AND handle = $2;

-- name: LockAgentByHandle :one
SELECT * FROM agents WHERE workspace_id = $1 AND handle = $2 FOR UPDATE;

-- name: CreateDefinitionVersion :one
INSERT INTO agent_definition_versions (agent_id, version, spec, note, created_by_user_id, created_at)
VALUES ($1, (SELECT coalesce(max(version), 0) + 1 FROM agent_definition_versions WHERE agent_id = $1), $2, $3, $4,
        coalesce(sqlc.narg(created_at)::timestamptz, now()))
RETURNING *;

-- name: GetDefinitionVersion :one
SELECT * FROM agent_definition_versions WHERE id = $1;

-- name: ListDefinitionVersions :many
SELECT * FROM agent_definition_versions WHERE agent_id = $1 ORDER BY version DESC;

-- name: UpdateAgentFromDefinition :one
-- Identity lives on the agent row; model and concurrency are denormalized
-- from the current definition for lists and the scheduler.
UPDATE agents SET name = $2, role = $3, specialty = $4, color_index = $5, model = $6,
  max_concurrency = $7, current_definition_version_id = $8, kind = coalesce(sqlc.narg(kind), kind)
WHERE id = $1 RETURNING *;

-- name: UpsertOverride :one
INSERT INTO agent_project_overrides (agent_id, project_id, spec, updated_by_user_id)
VALUES ($1, $2, $3, $4)
ON CONFLICT (agent_id, project_id) DO UPDATE
SET spec = EXCLUDED.spec, updated_by_user_id = EXCLUDED.updated_by_user_id,
    version = agent_project_overrides.version + 1
RETURNING *;

-- name: DeleteOverride :execrows
DELETE FROM agent_project_overrides WHERE agent_id = $1 AND project_id = $2;

-- name: GetOverride :one
SELECT * FROM agent_project_overrides WHERE agent_id = $1 AND project_id = $2;

-- name: ListOverrides :many
SELECT o.*, p.key AS project_key, p.name AS project_name, p.color AS project_color
FROM agent_project_overrides o JOIN projects p ON p.id = o.project_id
WHERE o.agent_id = $1
ORDER BY p.created_at;

-- name: CreateAgentToken :one
INSERT INTO agent_tokens (agent_id, token_hash, prefix, kind, label, run_id, created_by_user_id, expires_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING *;

-- name: ListAgentTokens :many
SELECT * FROM agent_tokens WHERE agent_id = $1 AND kind = 'unmanaged' ORDER BY created_at DESC;

-- name: ResolveAgentToken :one
-- A usable token: not revoked, not expired.
SELECT sqlc.embed(agents), agent_tokens.id AS token_id, agent_tokens.kind AS token_kind,
       agent_tokens.run_id AS token_run_id, agent_tokens.last_used_at AS token_last_used_at
FROM agent_tokens JOIN agents ON agents.id = agent_tokens.agent_id
WHERE agent_tokens.token_hash = $1
  AND agents.archived_at IS NULL
  AND agent_tokens.revoked_at IS NULL
  AND (agent_tokens.expires_at IS NULL OR agent_tokens.expires_at > now());

-- name: TouchAgentToken :exec
UPDATE agent_tokens SET last_used_at = now() WHERE id = $1;

-- name: RevokeAgentToken :execrows
UPDATE agent_tokens SET revoked_at = now()
WHERE id = $1 AND agent_id = $2 AND revoked_at IS NULL;

-- name: UpsertSecret :one
INSERT INTO secrets (id, workspace_id, project_id, name, grants_scope, nonce, ciphertext, created_by_user_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (workspace_id, coalesce(project_id, '00000000-0000-0000-0000-000000000000'::uuid), name) DO UPDATE
SET grants_scope = EXCLUDED.grants_scope, nonce = EXCLUDED.nonce, ciphertext = EXCLUDED.ciphertext,
    created_by_user_id = EXCLUDED.created_by_user_id
RETURNING *;

-- name: FindSecret :one
SELECT * FROM secrets
WHERE workspace_id = $1 AND name = $2
  AND (project_id IS NULL OR project_id = sqlc.narg(project_id))
-- A project's own secret beats the workspace one.
ORDER BY project_id IS NULL
LIMIT 1;

-- name: ListSecrets :many
SELECT s.id, s.project_id, s.name, s.grants_scope, s.created_at, s.updated_at, s.last_used_at,
       p.key AS project_key, p.name AS project_name, p.color AS project_color
FROM secrets s LEFT JOIN projects p ON p.id = s.project_id
WHERE s.workspace_id = $1
ORDER BY s.project_id NULLS FIRST, s.name;

-- name: DeleteSecret :execrows
DELETE FROM secrets WHERE id = $1 AND workspace_id = $2;

-- name: TouchSecret :exec
UPDATE secrets SET last_used_at = now() WHERE id = $1;

-- name: SecretExists :one
-- Metadata only: whether any secret of this name is stored; never reads the value.
SELECT EXISTS (SELECT 1 FROM secrets WHERE workspace_id = $1 AND name = $2)::bool;

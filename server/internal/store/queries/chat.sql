-- name: CreateChatSession :one
INSERT INTO chat_sessions (workspace_id, project_id, title, created_by, agent_id, harness, model)
VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING *;

-- name: ListChatSessions :many
SELECT * FROM chat_sessions
WHERE workspace_id = $1 AND project_id = $2 AND created_by = $3 AND NOT archived
ORDER BY COALESCE(last_message_at, created_at) DESC, id DESC
LIMIT $4;

-- name: GetChatSession :one
SELECT * FROM chat_sessions WHERE id = $1 AND workspace_id = $2;

-- name: RenameChatSession :one
-- Returns no row when the revision is stale.
UPDATE chat_sessions SET title = $3, revision = revision + 1, updated_at = now()
WHERE id = $1 AND workspace_id = $2 AND revision = $4 RETURNING *;

-- name: ArchiveChatSession :one
UPDATE chat_sessions SET archived = true, revision = revision + 1, updated_at = now()
WHERE id = $1 AND workspace_id = $2 AND revision = $3 RETURNING *;

-- name: AppendChatMessage :one
-- seq is allocated in the statement; concurrent appenders to one session collide on
-- UNIQUE (session_id, seq) and the loser must retry.
WITH next AS (
  SELECT COALESCE(MAX(m.seq), 0) + 1 AS seq FROM chat_messages m WHERE m.session_id = sqlc.arg(session_id)
), ins AS (
  INSERT INTO chat_messages (session_id, seq, role, body_md, harness, model, run_id, run_turn_id, context_refs, citations, usage)
  SELECT sqlc.arg(session_id), next.seq, sqlc.arg(role), sqlc.arg(body_md), sqlc.narg(harness), sqlc.narg(model), sqlc.narg(run_id), sqlc.narg(run_turn_id),
         sqlc.arg(context_refs), sqlc.arg(citations), sqlc.narg(usage)
  FROM next RETURNING *
), touch AS (
  UPDATE chat_sessions SET last_message_at = (SELECT created_at FROM ins), updated_at = now()
  WHERE id = sqlc.arg(session_id)
)
SELECT * FROM ins;

-- name: ListChatMessages :many
SELECT * FROM chat_messages WHERE session_id = $1 AND seq > $2 ORDER BY seq LIMIT $3;

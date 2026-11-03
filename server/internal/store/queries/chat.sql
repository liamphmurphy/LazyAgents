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

-- name: LockChatSession :one
SELECT * FROM chat_sessions WHERE id = $1 AND workspace_id = $2 FOR UPDATE;

-- name: ChatSessionForRun :one
SELECT * FROM chat_sessions WHERE current_run_id = $1;

-- name: SetChatRunState :one
UPDATE chat_sessions SET current_run_id = sqlc.narg(current_run_id), state = sqlc.arg(state),
 revision = revision + 1, updated_at = now()
WHERE id = $1 RETURNING *;

-- name: HasRunningChatTurn :one
SELECT EXISTS (SELECT 1 FROM run_turns WHERE run_id = $1 AND status = 'running')::bool;

-- name: FinishUnfinishedChatTurns :exec
UPDATE run_turns SET status = 'done', ended_at = coalesce(ended_at, now()), exit_code = 1
WHERE run_id = $1 AND status IN ('pending', 'running');

-- name: RecoverChatCancellations :exec
-- Cancel may have marked the run paused just before its runner stopped.
-- RequeueInterrupted has already made its interrupted turn pending.
WITH locked AS MATERIALIZED (
 SELECT c.id, c.current_run_id FROM chat_sessions c JOIN runs r ON r.id = c.current_run_id
 WHERE r.purpose = 'chat' AND r.state = 'paused' AND (r.runner_id = $1 OR r.runner_id IS NULL)
 ORDER BY c.id FOR UPDATE OF c
), cancelled AS (
 UPDATE runs SET state = 'waiting', wait_reason = 'human_reply'
 WHERE id IN (SELECT current_run_id FROM locked) AND state = 'paused'
 RETURNING id
), turns AS (
 UPDATE run_turns SET status = 'done', ended_at = coalesce(ended_at, now()), exit_code = 130
 WHERE run_id IN (SELECT id FROM cancelled) AND status = 'pending'
)
UPDATE chat_sessions SET state = 'idle', revision = revision + 1, updated_at = now()
WHERE current_run_id IN (SELECT id FROM cancelled);

-- name: ListChatsForUser :many
-- One page of a person's chats: archived or not, optionally one project.
SELECT c.* FROM chat_sessions c JOIN projects p ON p.id = c.project_id
WHERE c.workspace_id = $1 AND c.created_by = $2 AND c.archived = sqlc.arg(archived)
  AND (sqlc.narg(project_key)::text IS NULL OR p.key = sqlc.narg(project_key)::text)
ORDER BY COALESCE(c.last_message_at, c.created_at) DESC, c.id DESC
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: ChatMessageCounts :many
SELECT session_id, count(*)::int AS n FROM chat_messages WHERE session_id = ANY(sqlc.arg(ids)::uuid[]) GROUP BY session_id;

-- name: ChatLastErrors :many
-- The newest system message of each chat; the API shows it as `error` while the chat is failed.
SELECT DISTINCT ON (session_id) session_id, body_md FROM chat_messages
WHERE session_id = ANY(sqlc.arg(ids)::uuid[]) AND role = 'system'
ORDER BY session_id, seq DESC;

-- name: PageChatMessages :many
-- Messages with after < seq < before (either bound optional). newest_first pages
-- back through history; the caller reverses the rows to oldest first.
SELECT * FROM chat_messages
WHERE session_id = sqlc.arg(session_id)
  AND (sqlc.narg(after_seq)::bigint IS NULL OR seq > sqlc.narg(after_seq)::bigint)
  AND (sqlc.narg(before_seq)::bigint IS NULL OR seq < sqlc.narg(before_seq)::bigint)
ORDER BY CASE WHEN sqlc.arg(newest_first)::bool THEN -seq ELSE seq END
LIMIT sqlc.arg(row_limit);

-- name: ChatMessageForTurn :one
SELECT * FROM chat_messages WHERE run_turn_id = $1 AND role = 'user';

-- name: SetChatSelection :one
UPDATE chat_sessions SET harness = $2, model = $3, revision = revision + 1, updated_at = now()
WHERE id = $1 RETURNING *;

-- name: UpdateChatTitleArchived :one
UPDATE chat_sessions SET title = $2, archived = $3, revision = revision + 1, updated_at = now()
WHERE id = $1 RETURNING *;

-- name: DeleteChatSession :exec
DELETE FROM chat_sessions WHERE id = $1;

-- name: DefaultChatAgent :one
-- The default agent for a chat that names none: the oldest main (non-sub) agent,
-- ties broken by name then id so the choice is deterministic.
SELECT * FROM agents WHERE workspace_id = $1 AND kind <> 'sub' ORDER BY created_at, name, id LIMIT 1;

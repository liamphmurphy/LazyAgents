-- name: CreateChatImageJob :one
INSERT INTO chat_image_jobs (workspace_id, run_id, turn_id, chat_id, prompt, model)
VALUES ($1, $2, $3, $4, $5, $6) RETURNING *;

-- name: CountChatImageJobsForTurn :one
SELECT count(*)::int FROM chat_image_jobs WHERE run_id = $1 AND turn_id = $2;

-- name: QueuedChatImageJobs :many
SELECT * FROM chat_image_jobs WHERE run_id = $1 AND turn_id = $2 AND state = 'queued' ORDER BY id;

-- name: ChatImageJobsForTurn :many
SELECT * FROM chat_image_jobs WHERE run_id = $1 AND turn_id = $2 ORDER BY id;

-- name: StartChatImageJob :exec
UPDATE chat_image_jobs SET state = 'running' WHERE id = $1;

-- name: SetChatImageJobSession :exec
UPDATE chat_image_jobs SET codex_session_id = $2 WHERE id = $1;

-- name: FinishChatImageJob :exec
UPDATE chat_image_jobs SET state = sqlc.arg(state), error = sqlc.narg(error), finished_at = now() WHERE id = sqlc.arg(id);

-- name: RunningChatImageJobs :many
-- This run's image jobs that are making an image now.
SELECT * FROM chat_image_jobs WHERE run_id = $1 AND state = 'running';

-- name: RunningChatTurn :one
SELECT * FROM run_turns WHERE run_id = $1 AND status = 'running' ORDER BY seq DESC LIMIT 1;

-- name: FailStaleChatImageJobs :exec
-- A runner that died mid-step leaves its job running; end it so it stops
-- granting the step's tool allowances.
UPDATE chat_image_jobs SET state = 'failed', error = 'interrupted', finished_at = now()
WHERE run_id = $1 AND state = 'running';

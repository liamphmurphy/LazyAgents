-- name: CreateChatImageJob :one
INSERT INTO chat_image_jobs (workspace_id, run_id, turn_id, chat_id, prompt, model, harness, reference_roles)
VALUES ($1, $2, $3, $4, $5, $6, $7, coalesce(sqlc.narg(reference_roles)::text[], '{}')) RETURNING *;

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
-- failure_kind is set exactly when the job failed ('moderation' or 'error').
UPDATE chat_image_jobs SET state = sqlc.arg(state), error = sqlc.narg(error), failure_kind = sqlc.narg(failure_kind),
  outbox_files = coalesce(sqlc.narg(outbox_files)::text[], '{}'), finished_at = now()
WHERE id = sqlc.arg(id);

-- name: LinkChatImageJobAttachment :exec
-- The chat attachment a job's image became when its turn's reply was stored.
UPDATE chat_image_jobs SET attachment_id = $2 WHERE id = $1;

-- name: SetChatImageJobsFollowup :exec
-- The finished jobs of a turn, reported to the assistant by its image_followup turn.
UPDATE chat_image_jobs SET followup_turn_id = sqlc.arg(followup_turn_id)
WHERE run_id = sqlc.arg(run_id) AND turn_id = sqlc.arg(turn_id) AND state IN ('done', 'failed');

-- name: ChatImageJobsForFollowup :many
-- What an image_followup turn reports: each job with its image, if it got one.
SELECT j.id, j.prompt, j.state, j.failure_kind, j.error, t.seq AS turn_seq,
  a.filename AS attachment_filename, a.content_type AS attachment_content_type, a.blob_key AS attachment_blob_key
FROM chat_image_jobs j
JOIN run_turns t ON t.id = j.turn_id
LEFT JOIN chat_attachments a ON a.id = j.attachment_id
WHERE j.followup_turn_id = $1
ORDER BY j.id;

-- name: RunningChatImageJobs :many
-- This run's image jobs that are making an image now.
SELECT * FROM chat_image_jobs WHERE run_id = $1 AND state = 'running';

-- name: RunningChatTurn :one
SELECT * FROM run_turns WHERE run_id = $1 AND status = 'running' ORDER BY seq DESC LIMIT 1;

-- name: FailStaleChatImageJobs :exec
-- A runner that died mid-step leaves its job running; end it so it stops
-- granting the step's tool allowances.
UPDATE chat_image_jobs SET state = 'failed', error = 'interrupted', failure_kind = 'error', finished_at = now()
WHERE run_id = $1 AND state = 'running';

-- name: FailQueuedChatImageJobs :many
-- A turn that ends without running its image jobs (its reply failed, or the
-- run was stopped) fails them, so no request is left queued with no result.
UPDATE chat_image_jobs SET state = 'failed', error = sqlc.arg(error)::text, failure_kind = 'error', finished_at = now()
WHERE run_id = sqlc.arg(run_id) AND turn_id = sqlc.arg(turn_id) AND state = 'queued'
RETURNING *;

-- name: RecentChatImagePrompts :many
-- The prompts behind a chat's newest generated images, newest first, so a
-- fresh segment can revise an image it made in an earlier one.
SELECT prompt, created_at FROM chat_image_jobs
WHERE chat_id = $1 AND state = 'done'
ORDER BY created_at DESC, id DESC LIMIT sqlc.arg(row_limit);

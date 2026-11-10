-- name: CreateChatVideoJob :one
INSERT INTO chat_video_jobs (workspace_id, user_id, chat_id, run_id, turn_id, persona_id, title, width, height, fps, duration_ms, script_path)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12) RETURNING *;

-- name: CountChatVideoJobsForTurn :one
SELECT count(*)::int FROM chat_video_jobs WHERE run_id = $1 AND turn_id = $2;

-- name: LockChatVideoJob :one
-- One of a run's jobs, locked for a stage change.
SELECT * FROM chat_video_jobs WHERE id = $1 AND run_id = $2 FOR UPDATE;

-- name: GetChatVideoJob :one
SELECT * FROM chat_video_jobs WHERE id = $1 AND workspace_id = $2;

-- name: SetChatVideoStage :one
-- The agent moved the job on. render_started_at is set the first time it
-- reaches render; script_path changes only when one is given.
UPDATE chat_video_jobs SET stage = sqlc.arg(stage),
  checks = coalesce(sqlc.narg(checks)::jsonb, checks),
  script_path = coalesce(sqlc.narg(script_path)::text, script_path),
  render_started_at = CASE WHEN sqlc.arg(stage) IN ('render', 'encode', 'save') THEN coalesce(render_started_at, now()) ELSE render_started_at END,
  updated_at = now()
WHERE id = sqlc.arg(id) AND state = 'running'
RETURNING *;

-- name: FailChatVideoJob :one
-- Ends a running job as failed at the stage it reached.
UPDATE chat_video_jobs SET state = 'failed', error = sqlc.arg(error), failure_kind = sqlc.arg(failure_kind), error_log = sqlc.arg(error_log), updated_at = now(), finished_at = now()
WHERE id = sqlc.arg(id) AND state = 'running'
RETURNING *;

-- name: RunningChatVideoJobs :many
-- The jobs a run is working on, oldest first.
SELECT * FROM chat_video_jobs WHERE run_id = $1 AND state = 'running' ORDER BY created_at, id;

-- name: SetChatVideoProgress :exec
-- What the runner read from the job's directory (the caller skips it when
-- nothing changed: every statement notifies). A renderer only moves a job
-- from render to encode; a stage the agent has already moved past is kept.
UPDATE chat_video_jobs SET
  frames_done = sqlc.arg(frames_done), frames_total = sqlc.arg(frames_total), stage_note = sqlc.arg(stage_note),
  script_lines = sqlc.arg(script_lines), script_tail = sqlc.arg(script_tail),
  stage = CASE WHEN stage = 'render' AND sqlc.arg(encoding)::boolean THEN 'encode' ELSE stage END,
  updated_at = now()
WHERE id = sqlc.arg(id) AND state = 'running';

-- name: FinishChatVideoJob :one
-- The runner stored the finished video.
UPDATE chat_video_jobs SET state = 'done', output_blob_key = $2, output_filename = $3, output_content_type = $4, output_size_bytes = $5,
  output_codec = $6, output_duration_ms = $7, library_item_id = $8, updated_at = now(), finished_at = now()
WHERE id = $1 AND state = 'running'
RETURNING *;

-- name: ChatVideoFrames :many
SELECT * FROM chat_video_frames WHERE job_id = ANY(sqlc.arg(job_ids)::uuid[]) ORDER BY job_id, kind, idx;

-- name: UpsertChatVideoFrame :one
-- Stores a frame, returning the object it replaced (to delete), or '' for a new one.
WITH old AS (SELECT blob_key FROM chat_video_frames WHERE job_id = $1 AND kind = $2 AND idx = $3)
INSERT INTO chat_video_frames (job_id, kind, idx, blob_key, content_type, size_bytes, source_stamp)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (job_id, kind, idx) DO UPDATE SET blob_key = EXCLUDED.blob_key, content_type = EXCLUDED.content_type,
  size_bytes = EXCLUDED.size_bytes, source_stamp = EXCLUDED.source_stamp, updated_at = now()
RETURNING coalesce((SELECT blob_key FROM old), '')::text AS replaced;

-- name: GetChatVideoFrame :one
SELECT f.* FROM chat_video_frames f JOIN chat_video_jobs j ON j.id = f.job_id
WHERE f.job_id = $1 AND f.kind = $2 AND f.idx = $3 AND j.workspace_id = $4;

-- name: ListChatVideoJobs :many
-- A chat's videos, oldest first.
SELECT * FROM chat_video_jobs WHERE chat_id = $1 ORDER BY created_at, id;

-- name: LinkChatVideoJobsToMessage :exec
-- Puts a turn's videos, and the Library copies of the finished ones, on the
-- reply that turn stored.
WITH j AS (
  UPDATE chat_video_jobs SET message_id = sqlc.arg(message_id), updated_at = now()
  WHERE run_id = sqlc.arg(run_id) AND turn_id = sqlc.arg(turn_id) AND message_id IS NULL
  RETURNING library_item_id
)
UPDATE library_items SET message_id = sqlc.arg(message_id) WHERE id IN (SELECT library_item_id FROM j WHERE library_item_id IS NOT NULL);

-- name: DeleteDetachedChatVideoJobs :many
-- Removes up to row_limit jobs whose chat is gone, returning the objects only
-- they held: their frames, and their video unless the Library keeps it.
WITH j AS (
  SELECT id FROM chat_video_jobs WHERE chat_id IS NULL AND state <> 'running'
  ORDER BY created_at LIMIT sqlc.arg(row_limit) FOR UPDATE SKIP LOCKED
), f AS (
  DELETE FROM chat_video_frames WHERE job_id IN (SELECT id FROM j) RETURNING blob_key
), d AS (
  DELETE FROM chat_video_jobs WHERE id IN (SELECT id FROM j) RETURNING output_blob_key
)
SELECT f.blob_key::text FROM f
UNION ALL
SELECT d.output_blob_key::text FROM d WHERE d.output_blob_key IS NOT NULL
  AND NOT EXISTS (SELECT 1 FROM library_items l WHERE l.blob_key = d.output_blob_key);

-- name: CreateLibraryVideo :one
INSERT INTO library_items (workspace_id, user_id, kind, title, filename, content_type, size_bytes, blob_key, chat_id, message_id, persona_id, duration_ms, width, height, poster_key)
VALUES ($1, $2, 'video', $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14) RETURNING *;

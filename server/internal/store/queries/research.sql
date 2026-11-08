-- name: CreateResearchJob :one
INSERT INTO research_jobs (workspace_id, chat_id, user_id, message_id, title, max_sources, max_minutes, max_cost_micros)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING *;

-- name: CreateResearchTask :one
INSERT INTO research_tasks (job_id, run_id, seq, label, question) VALUES ($1, $2, $3, $4, $5) RETURNING *;

-- name: ResearchJobForMessage :one
SELECT * FROM research_jobs WHERE message_id = $1;

-- name: GetResearchJob :one
SELECT * FROM research_jobs WHERE id = $1 AND chat_id = $2;

-- name: LockResearchJob :one
SELECT * FROM research_jobs WHERE id = $1 FOR UPDATE;

-- name: ListResearchJobs :many
SELECT * FROM research_jobs WHERE chat_id = $1 ORDER BY created_at;

-- name: ActiveResearchJobs :many
SELECT id, chat_id FROM research_jobs WHERE state IN ('running', 'finishing', 'ready', 'writing') ORDER BY created_at LIMIT 50;

-- name: LiveResearchJobsForChat :many
SELECT * FROM research_jobs WHERE chat_id = $1 AND state IN ('running', 'finishing', 'ready', 'writing');

-- name: WritingResearchJobForChat :one
SELECT * FROM research_jobs WHERE chat_id = $1 AND state = 'writing' ORDER BY created_at DESC LIMIT 1 FOR UPDATE;

-- name: SetResearchJobState :one
UPDATE research_jobs SET state = $2, stop_reason = $3, error = $4,
  finished_at = CASE WHEN $2 IN ('done', 'stopped', 'failed') THEN now() ELSE finished_at END
WHERE id = $1 RETURNING *;

-- name: SetResearchJobReport :one
UPDATE research_jobs SET state = 'done', report = $2, report_artifact_id = $3, finished_at = now() WHERE id = $1 RETURNING *;

-- name: ResearchJobForRun :one
SELECT j.* FROM research_jobs j JOIN research_tasks t ON t.job_id = j.id WHERE t.run_id = $1;

-- name: ResearchTaskForRun :one
SELECT * FROM research_tasks WHERE run_id = $1 FOR UPDATE;

-- name: SetResearchTaskFindings :exec
UPDATE research_tasks SET findings = $2 WHERE id = $1;

-- name: ListResearchTasks :many
-- Progress per researcher: what its run is doing, read from the run's events.
SELECT t.*, r.state AS run_state, r.cost_usd_micros,
  (SELECT count(*) FROM run_events e WHERE e.run_id = r.id AND e.type = 'tool_call' AND e.payload->>'tool' IN ('WebFetch', 'web_search'))::int AS sources_read,
  coalesce((SELECT e.payload->'input'->>'url' FROM run_events e WHERE e.run_id = r.id AND e.type = 'tool_call' AND e.payload->>'tool' = 'WebFetch' ORDER BY e.seq DESC LIMIT 1), '')::text AS reading
FROM research_tasks t JOIN runs r ON r.id = t.run_id
WHERE t.job_id = $1 ORDER BY t.seq;

-- name: ResearchJobFetches :one
-- Pages the job's researchers have opened: checked against the source limit.
SELECT count(*)::int FROM run_events e JOIN research_tasks t ON t.run_id = e.run_id
WHERE t.job_id = $1 AND e.type = 'tool_call' AND e.payload->>'tool' = 'WebFetch';

-- name: OwnsResearchRun :one
SELECT EXISTS (SELECT 1 FROM research_jobs j JOIN research_tasks t ON t.job_id = j.id WHERE t.run_id = $1 AND j.user_id = $2)::boolean;

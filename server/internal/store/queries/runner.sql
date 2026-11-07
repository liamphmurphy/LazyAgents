-- The runner (PLAN.md §6.4–6.6): queue, turns, heartbeats, PR sync.

-- name: ClaimQueuedRun :one
-- The oldest queued run whose agent has a free slot (max_concurrency from
-- the frozen definition; waiting runs don't hold a slot). A ticket run whose
-- blocker tickets aren't Done stays queued.
UPDATE runs SET state = 'starting', runner_id = sqlc.arg(runner_id)
WHERE id = (
  SELECT r.id FROM runs r
  WHERE r.state = 'queued'
    AND NOT EXISTS (SELECT 1 FROM run_turns b WHERE b.run_id = r.parent_run_id AND b.status = 'running')
    AND (r.parent_run_id IS NOT NULL OR NOT EXISTS (
      SELECT 1 FROM ticket_dependencies d JOIN tickets bt ON bt.id = d.blocker_id
      WHERE d.ticket_id = r.ticket_id AND bt.status <> 'done'))
    AND (r.purpose = 'chat' OR (SELECT count(*) FROM runs o WHERE o.agent_id = r.agent_id AND o.purpose <> 'chat' AND o.state IN ('starting', 'running'))
        < greatest(coalesce((r.resolved_spec->>'max_concurrency')::int, 1), 1))
  ORDER BY r.started_at
  FOR UPDATE SKIP LOCKED
  LIMIT 1
)
RETURNING *;

-- name: ClaimPendingTurn :one
-- The next turn to execute: a pending turn of a running managed run whose
-- previous turn has exited (a reply can arrive while the agent is still
-- ending its turn; one process per workspace at a time).
UPDATE run_turns SET status = 'running', process_started_at = now()
WHERE id = (
  SELECT t.id FROM run_turns t JOIN runs r ON r.id = t.run_id
  WHERE t.status = 'pending' AND r.mode = 'managed' AND r.state = 'running'
    AND NOT EXISTS (SELECT 1 FROM run_turns b WHERE b.run_id = t.run_id AND b.status = 'running')
    -- A delegated run shares its parent's checkout: wait for the parent's process to exit.
    AND NOT EXISTS (SELECT 1 FROM run_turns b WHERE b.run_id = r.parent_run_id AND b.status = 'running')
    AND NOT EXISTS (
      SELECT 1 FROM run_turns b JOIN runs c ON c.id = b.run_id
      WHERE c.parent_run_id = r.id AND b.status = 'running')
  ORDER BY t.seq
  FOR UPDATE OF t SKIP LOCKED
  LIMIT 1
)
RETURNING *;

-- name: FinishTurn :exec
UPDATE run_turns SET status = 'done', ended_at = coalesce(ended_at, now()), exit_code = coalesce(exit_code, sqlc.narg(exit_code)),
  log_key = sqlc.narg(log_key), usage = sqlc.arg(usage)
WHERE id = $1;

-- name: DropPendingTurns :exec
-- A run that ended can't execute queued turns.
UPDATE run_turns SET status = 'done', ended_at = coalesce(ended_at, now()) WHERE run_id = $1 AND status = 'pending';

-- name: GetTurn :one
SELECT * FROM run_turns WHERE id = $1;

-- name: SetRunSession :exec
UPDATE runs SET harness_session_id = $2 WHERE id = $1;

-- name: SetRunWorkspace :exec
UPDATE runs SET workspace_volume = $2 WHERE id = $1;

-- name: SetIdleTurns :exec
UPDATE runs SET idle_turns = $2 WHERE id = $1;

-- name: SetRunCost :exec
-- Harness-reported cost replaces computed or self-reported cost.
UPDATE runs SET cost_source = $2 WHERE id = $1;

-- name: RunActiveSeconds :one
-- Time the harness actually ran, across turns (waits don't count).
SELECT count(*)::int AS turns,
  coalesce(sum(extract(epoch FROM (coalesce(ended_at, now()) - process_started_at))), 0)::float8 AS seconds
FROM run_turns WHERE run_id = $1 AND process_started_at IS NOT NULL;

-- name: RequeueInterrupted :exec
-- On runner start: turns that were running when it died run again (the
-- harness resumes the session), and half-started runs go back to the queue.
-- The runner id is host-pid, so a restarted runner has a new one: also reclaim
-- turns of any runner that has stopped heartbeating (LiveRunners' window).
UPDATE run_turns SET status = 'pending', process_started_at = NULL, ended_at = NULL, exit_code = NULL
WHERE status = 'running' AND run_id IN (
  SELECT id FROM runs WHERE runner_id = sqlc.arg(runner_id) OR runner_id IS NULL
    OR runner_id NOT IN (SELECT id FROM runners WHERE heartbeat_at > now() - interval '45 seconds'));

-- name: RequeueStarting :exec
UPDATE runs SET state = 'queued' WHERE state = 'starting';

-- name: Heartbeat :exec
INSERT INTO runners (id, backend, max_concurrent, running, version)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (id) DO UPDATE SET backend = EXCLUDED.backend, max_concurrent = EXCLUDED.max_concurrent,
  running = EXCLUDED.running, version = EXCLUDED.version, heartbeat_at = now();

-- name: DeleteRunner :exec
DELETE FROM runners WHERE id = $1;

-- name: LiveRunners :many
SELECT * FROM runners WHERE heartbeat_at > now() - interval '45 seconds' ORDER BY started_at;

-- name: QueuedRunsCount :one
SELECT count(*)::int FROM runs WHERE state = 'queued';

-- name: SecretsForRun :many
-- Workspace secrets plus the project's own; a project secret shadows the
-- workspace one with the same name.
SELECT DISTINCT ON (name) id, name, grants_scope, project_id FROM secrets
WHERE workspace_id = $1 AND (project_id IS NULL OR project_id = sqlc.narg(project_id))
ORDER BY name, project_id NULLS LAST;

-- name: TicketsWithOpenPRs :many
SELECT t.id, t.workspace_id, t.key, t.epic_id, t.pr_url, t.pr_number, t.pr_state, t.status, t.project_id
FROM tickets t
WHERE t.pr_url IS NOT NULL AND coalesce(t.pr_state, 'open') IN ('draft', 'open')
  AND (t.pr_synced_at IS NULL OR t.pr_synced_at < now() - make_interval(secs => sqlc.arg(min_age_s)::int))
ORDER BY t.pr_synced_at NULLS FIRST
LIMIT 50;

-- name: UpdatePRSync :exec
UPDATE tickets SET pr_state = $2, pr_additions = $3, pr_deletions = $4, pr_synced_at = now(),
  pr_closed_unmerged = $5
WHERE id = $1;

-- name: TouchPRSync :exec
UPDATE tickets SET pr_synced_at = now() WHERE id = $1;

-- name: SetRunDispatcher :exec
UPDATE runs SET dispatched_by_user_id = $2 WHERE id = $1;

-- name: LockRunNoWait :one
-- Plain read used by the runner (it locks inside its own transactions).
SELECT * FROM runs WHERE id = $1;

-- name: HasPendingTurn :one
SELECT EXISTS (SELECT 1 FROM run_turns WHERE run_id = $1 AND status = 'pending')::bool;

-- name: EndedRunsWithWorkspace :many
-- Workspaces of runs that completed while no turn was running; the runner
-- removes them. Failed and cancelled runs keep theirs (unpushed work).
SELECT id FROM runs r
WHERE r.workspace_volume IS NOT NULL AND r.state = 'completed'
  AND NOT EXISTS (SELECT 1 FROM run_turns t WHERE t.run_id = r.id AND t.status = 'running')
LIMIT 20;

-- name: RunnerEnvironments :many
-- Latest observed setup per runner and project, or the exact run requested.
SELECT * FROM (
  SELECT DISTINCT ON (coalesce(nullif(e.payload->>'runner_id', ''), r.runner_id, ''), t.project_id)
    r.id AS run_id, coalesce(nullif(e.payload->>'runner_id', ''), r.runner_id, '')::text AS runner_id,
    e.at AS recorded_at, e.payload,
    t.key AS ticket_key, p.key AS project_key, p.name AS project_name
  FROM run_events e
  JOIN runs r ON r.id = e.run_id
  JOIN tickets t ON t.id = r.ticket_id
  JOIN projects p ON p.id = t.project_id
  WHERE r.workspace_id = sqlc.arg(workspace_id)
    AND e.type = 'lifecycle' AND e.payload->>'state' = 'environment_snapshot'
    AND (sqlc.narg(run_id)::uuid IS NULL OR r.id = sqlc.narg(run_id))
  ORDER BY coalesce(nullif(e.payload->>'runner_id', ''), r.runner_id, ''), t.project_id, e.at DESC, e.seq DESC
) AS snapshots
ORDER BY recorded_at DESC
LIMIT 50;

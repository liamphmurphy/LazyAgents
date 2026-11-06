-- name: CreateRun :one
INSERT INTO runs (workspace_id, ticket_id, agent_id, mode, state, resolved_spec, definition_version_id, harness, model, parent_run_id, purpose)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, sqlc.narg(parent_run_id), sqlc.arg(purpose)) RETURNING *;

-- name: GetRun :one
SELECT * FROM runs WHERE id = $1 AND workspace_id = $2;

-- name: LockRun :one
SELECT * FROM runs WHERE id = $1 FOR UPDATE;

-- name: ActiveRunForTicket :one
-- The ticket's top-level run; delegated runs belong to it.
SELECT * FROM runs WHERE ticket_id = $1 AND parent_run_id IS NULL AND state NOT IN ('completed', 'failed', 'cancelled');

-- name: ActiveChildRun :one
SELECT * FROM runs WHERE parent_run_id = $1 AND state NOT IN ('completed', 'failed', 'cancelled') ORDER BY started_at DESC LIMIT 1;

-- name: HasLiveChild :one
SELECT EXISTS(SELECT 1 FROM runs WHERE parent_run_id = $1 AND state NOT IN ('completed', 'failed', 'cancelled'));

-- name: CountChildRuns :one
SELECT count(*) FROM runs WHERE parent_run_id = $1;

-- name: ChildRuns :many
SELECT * FROM runs WHERE parent_run_id = $1 ORDER BY started_at;

-- name: LatestRunForTicket :one
SELECT * FROM runs WHERE ticket_id = $1 AND parent_run_id IS NULL ORDER BY started_at DESC LIMIT 1;

-- name: ActiveRunsForAgent :many
SELECT r.*, coalesce(t.key, 'Planning')::text AS ticket_key FROM runs r LEFT JOIN tickets t ON t.id = r.ticket_id
WHERE r.agent_id = $1 AND r.state NOT IN ('completed', 'failed', 'cancelled')
ORDER BY r.started_at;

-- name: SetRunState :one
UPDATE runs SET state = $2, wait_reason = $3,
  ended_at = CASE WHEN $2 IN ('completed', 'failed', 'cancelled') THEN now() ELSE ended_at END,
  outcome = coalesce(sqlc.narg(outcome), outcome)
WHERE id = $1 RETURNING *;

-- name: AddRunUsage :one
UPDATE runs SET tool_calls = tool_calls + $2, input_tokens = input_tokens + $3, output_tokens = output_tokens + $4,
  cost_usd_micros = cost_usd_micros + sqlc.arg(cost_usd_micros),
  cost_source = CASE WHEN cost_source IN ('unknown','partial') OR sqlc.narg(cost_source)::text = 'unknown' THEN
    CASE WHEN cost_usd_micros + sqlc.arg(cost_usd_micros) > 0 THEN 'partial' ELSE 'unknown' END
    ELSE coalesce(cost_source, sqlc.narg(cost_source)) END
WHERE id = $1 RETURNING *;

-- name: StartTurn :one
-- Managed turns wait for the runner (pending); unmanaged turns are records.
INSERT INTO run_turns (run_id, seq, trigger, input_message, status)
VALUES ($1, (SELECT coalesce(max(seq), 0) + 1 FROM run_turns WHERE run_id = $1), $2, $3,
  CASE WHEN (SELECT mode FROM runs WHERE id = $1) = 'managed' THEN 'pending' ELSE 'done' END)
RETURNING *;

-- name: EndOpenTurn :exec
UPDATE run_turns SET ended_at = now(), exit_code = $2 WHERE run_id = $1 AND ended_at IS NULL;

-- name: ListTurns :many
SELECT * FROM run_turns WHERE run_id = $1 ORDER BY seq;

-- name: AddRunEvent :exec
INSERT INTO run_events (run_id, type, payload) VALUES ($1, $2, $3);

-- name: ListRunEvents :many
SELECT * FROM run_events WHERE run_id = $1 AND seq > $2 ORDER BY seq LIMIT 1000;

-- name: DeletePlan :exec
DELETE FROM plan_steps WHERE run_id = $1;

-- name: InsertPlanStep :exec
INSERT INTO plan_steps (run_id, position, text, state) VALUES ($1, $2, $3, $4);

-- name: ListPlan :many
SELECT * FROM plan_steps WHERE run_id = $1 ORDER BY position;

-- name: SetStepState :execrows
UPDATE plan_steps SET state = $3 WHERE run_id = $1 AND position = $2;

-- name: ClearCurrentStep :exec
UPDATE plan_steps SET state = 'pending' WHERE run_id = $1 AND state = 'current' AND position <> $2;

-- name: SetAgentStatus :exec
UPDATE agents SET status = $2 WHERE id = $1;

-- name: CreateApproval :one
INSERT INTO approval_requests (workspace_id, run_id, ticket_id, agent_id, scope, action_summary, action_detail, risk_summary)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING *;

-- name: GetApproval :one
SELECT * FROM approval_requests WHERE id = $1 AND workspace_id = $2;

-- name: LockApproval :one
SELECT * FROM approval_requests WHERE id = $1 AND workspace_id = $2 FOR UPDATE;

-- name: DecideApproval :one
UPDATE approval_requests SET state = $2, decided_by_user_id = $3, decided_at = now(), decision_note = $4
WHERE id = $1 AND state = 'pending' RETURNING *;

-- name: MarkApprovalExecuted :one
UPDATE approval_requests SET executed_at = now(), result = $2
WHERE id = $1 AND state = 'approved' AND executed_at IS NULL RETURNING *;

-- name: ExpireRunApprovals :exec
UPDATE approval_requests SET state = 'expired' WHERE run_id = $1 AND state = 'pending';

-- name: PendingApprovalsForTicket :many
SELECT * FROM approval_requests WHERE ticket_id = $1 AND state = 'pending' ORDER BY created_at;

-- name: CountPendingApprovals :one
SELECT count(*)::int FROM approval_requests WHERE workspace_id = $1 AND state = 'pending';

-- name: SetTicketBranch :exec
UPDATE tickets SET branch = $2 WHERE id = $1;

-- name: SetTicketPR :exec
UPDATE tickets SET pr_number = $2, pr_url = $3, pr_state = coalesce($4, pr_state, 'open') WHERE id = $1;

-- name: GetCriterionByPosition :one
SELECT * FROM acceptance_criteria WHERE ticket_id = $1 ORDER BY position, created_at OFFSET $2 LIMIT 1;

-- name: ClaimApprovalExecution :one
-- Marks an approved request as executing exactly once (concurrent callers lose).
UPDATE approval_requests SET executed_at = now()
WHERE id = $1 AND run_id = $2 AND state = 'approved' AND executed_at IS NULL
RETURNING *;

-- name: SetApprovalResult :exec
UPDATE approval_requests SET result = $2 WHERE id = $1;

-- name: DecidedApprovalsSince :many
SELECT * FROM approval_requests WHERE run_id = $1 AND decided_at > $2 ORDER BY decided_at;

-- name: ListApprovalsForRun :many
SELECT * FROM approval_requests WHERE run_id = $1 ORDER BY created_at;

-- name: ReleaseApprovalExecution :exec
-- Nothing ran (e.g. the secret was missing): the approval can be used again.
UPDATE approval_requests SET executed_at = NULL WHERE id = $1 AND result IS NULL;


-- name: SetRunModel :exec
-- Same-harness model switch: keeps the session id and workspace.
UPDATE runs SET model = sqlc.arg(model)::text,
  resolved_spec = jsonb_set(resolved_spec, '{model}', to_jsonb(sqlc.arg(model)::text))
WHERE id = sqlc.arg(id);

-- name: RunVerdict :one
-- The review verdict ("" if none) a delegated run reported when it completed.
SELECT coalesce(payload->>'verdict', '')::text AS verdict FROM run_events WHERE run_id = $1 AND type = 'lifecycle' AND payload->>'state' = 'completed' ORDER BY seq DESC LIMIT 1;

-- name: SetRunContract :one
UPDATE runs SET contract = $2 WHERE id = $1 RETURNING *;

-- name: SetRunChecks :exec
UPDATE runs SET checks = $2 WHERE id = $1;

-- name: AddRunTiming :exec
-- Adds wall-clock milliseconds to a run's phase timings (NULL counts as 0).
UPDATE runs SET setup_ms = coalesce(setup_ms, 0) + sqlc.arg(setup_ms)::bigint,
  harness_ms = coalesce(harness_ms, 0) + sqlc.arg(harness_ms)::bigint,
  checks_ms = coalesce(checks_ms, 0) + sqlc.arg(checks_ms)::bigint
WHERE id = $1;

-- name: DelegationEfficiency :many
-- Per sub agent: what delegating to it costs and how often its first attempt
-- passes the runner's checks. A repair loop is a later delegation to the same
-- agent under the same parent run.
-- checked counts delegations the runner checked (checks is not null; older runs
-- were never checked); first-pass rate is first_pass / checked.
-- parent_cost_usd_micros attributes each parent run's cost evenly across its
-- child runs (parent cost / number of child runs of that parent, finished or not), summed over this agent's children, so a parent that
-- delegated to several agents is not counted in full in each row.
-- avg_*_seconds average the phase timings over child runs that have them.
WITH kids AS (
  SELECT r.*, lead(r.id) OVER (PARTITION BY r.parent_run_id, r.agent_id ORDER BY r.started_at) AS repaired_by,
    (SELECT count(*) FROM runs c WHERE c.parent_run_id = r.parent_run_id) AS siblings
  FROM runs r
  WHERE r.workspace_id = $1 AND r.parent_run_id IS NOT NULL AND r.state IN ('completed', 'failed', 'cancelled')
)
SELECT k.agent_id,
  count(*)::bigint AS delegations,
  count(*) FILTER (WHERE k.checks IS NOT NULL)::bigint AS checked,
  count(*) FILTER (WHERE k.checks->>'passed' = 'true' AND k.repaired_by IS NULL)::bigint AS first_pass,
  count(*) FILTER (WHERE k.repaired_by IS NOT NULL)::bigint AS repair_loops,
  coalesce(sum(k.cost_usd_micros), 0)::bigint AS child_cost_usd_micros,
  floor(coalesce(sum(p.cost_usd_micros::float8 / k.siblings), 0))::bigint AS parent_cost_usd_micros,
  coalesce(sum(extract(epoch FROM (k.ended_at - k.started_at))), 0)::float8 AS child_seconds,
  (coalesce(avg(k.setup_ms), 0) / 1000)::float8 AS avg_setup_seconds,
  (coalesce(avg(k.harness_ms), 0) / 1000)::float8 AS avg_harness_seconds,
  (coalesce(avg(k.checks_ms), 0) / 1000)::float8 AS avg_checks_seconds
FROM kids k LEFT JOIN runs p ON p.id = k.parent_run_id GROUP BY k.agent_id;

-- name: ListMockupWaitingRuns :many
-- Top-level runs parked on a mockup review, with their ticket.
SELECT r.* FROM runs r
WHERE r.workspace_id = $1 AND r.parent_run_id IS NULL AND r.state = 'waiting' AND r.wait_reason = 'mockup_review';

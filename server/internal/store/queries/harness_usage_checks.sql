-- name: RequestUsageCheck :exec
-- Queues a check unless one is in flight or backing off. A changed login
-- always queues: the person just signed in and expects to see it.
INSERT INTO harness_usage_checks (workspace_id, harness, login_version, status)
VALUES (@workspace_id, @harness, @login_version, 'pending')
ON CONFLICT (workspace_id, harness) DO UPDATE
SET login_version = EXCLUDED.login_version, status = 'pending', requested_at = now(),
    claimed_at = NULL, finished_at = NULL, runner_id = NULL, claim_id = NULL, retry_after = now()
WHERE harness_usage_checks.login_version <> EXCLUDED.login_version
   OR (harness_usage_checks.status = 'done' AND harness_usage_checks.retry_after <= now());

-- name: UsageChecksInFlight :many
-- Harnesses with a check queued or running, recently enough that a runner
-- should still answer (the orb shows them as checking).
SELECT harness FROM harness_usage_checks
WHERE workspace_id = @workspace_id AND status IN ('pending', 'running')
  AND requested_at > now() - interval '2 minutes';

-- name: ClaimUsageCheck :one
-- The oldest pending request, or one a runner claimed and then went quiet on.
UPDATE harness_usage_checks SET status = 'running', runner_id = @runner_id, claimed_at = now(), claim_id = uuid_v7()
WHERE (workspace_id, harness) = (
  SELECT c.workspace_id, c.harness FROM harness_usage_checks c
  WHERE c.status = 'pending' OR (c.status = 'running' AND c.claimed_at < now() - interval '2 minutes')
  ORDER BY c.requested_at LIMIT 1 FOR UPDATE SKIP LOCKED)
RETURNING *;

-- name: FinishUsageCheck :execrows
-- Only the holder of the current claim may finish it.
UPDATE harness_usage_checks SET status = 'done', finished_at = now(), retry_after = @retry_after
WHERE claim_id = @claim_id AND status = 'running';

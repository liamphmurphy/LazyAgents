-- name: GetModelListing :one
SELECT * FROM harness_model_listings WHERE workspace_id = $1 AND harness = $2;

-- name: RequestModelListing :exec
-- Queues a listing for the current login; the last good models stay readable.
-- A request already in flight for the same login is left alone.
INSERT INTO harness_model_listings (workspace_id, harness, login_version, status)
VALUES ($1, $2, $3, 'pending')
ON CONFLICT (workspace_id, harness) DO UPDATE
SET login_version = EXCLUDED.login_version, status = 'pending', error = '',
    requested_at = now(), claimed_at = NULL, finished_at = NULL, runner_id = NULL, claim_id = NULL
WHERE harness_model_listings.login_version <> EXCLUDED.login_version
   OR harness_model_listings.status IN ('done', 'failed');

-- name: ClaimModelListing :one
-- The oldest pending request, or one a runner claimed and then went quiet on.
UPDATE harness_model_listings SET status = 'running', runner_id = $1, claimed_at = now(), claim_id = uuid_v7()
WHERE (workspace_id, harness) = (
  SELECT l.workspace_id, l.harness FROM harness_model_listings l
  WHERE l.status = 'pending' OR (l.status = 'running' AND l.claimed_at < now() - interval '2 minutes')
  ORDER BY l.requested_at LIMIT 1 FOR UPDATE SKIP LOCKED)
RETURNING *;

-- name: FinishModelListing :execrows
-- Only the holder of the current claim may finish it.
UPDATE harness_model_listings
SET status = 'done', models = $2, models_version = login_version, listed_at = now(), finished_at = now(), error = ''
WHERE claim_id = $1 AND status = 'running';

-- name: FailModelListing :execrows
UPDATE harness_model_listings
SET status = 'failed', error = $2, finished_at = now()
WHERE claim_id = $1 AND status = 'running';

-- name: ExpireModelListings :execrows
-- A runner starting may bring a different harness CLI, whose aliases resolve
-- to different models. Saved models stay readable while they're listed again.
UPDATE harness_model_listings SET listed_at = NULL WHERE listed_at IS NOT NULL;

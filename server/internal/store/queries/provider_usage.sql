-- Harness plan allowance readings per workspace (see internal/usage).

-- name: ProviderUsage :many
SELECT * FROM provider_usage WHERE workspace_id = @workspace_id ORDER BY harness;

-- name: ProviderUsageFor :one
SELECT * FROM provider_usage WHERE workspace_id = @workspace_id AND harness = @harness;

-- name: SaveProviderUsage :exec
INSERT INTO provider_usage (workspace_id, harness, plan, available, windows, observed_at)
VALUES (@workspace_id, @harness, @plan, @available, @windows, @observed_at)
ON CONFLICT (workspace_id, harness)
DO UPDATE SET plan = EXCLUDED.plan, available = EXCLUDED.available, windows = EXCLUDED.windows, observed_at = EXCLUDED.observed_at;

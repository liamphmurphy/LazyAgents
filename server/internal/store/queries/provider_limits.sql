-- Harness usage limits per workspace (see runner/fallback.go).

-- name: MarkProviderLimited :exec
INSERT INTO provider_limits (workspace_id, harness, until, reason)
VALUES (@workspace_id, @harness, @until, @reason)
ON CONFLICT (workspace_id, harness)
DO UPDATE SET until = EXCLUDED.until, reason = EXCLUDED.reason, updated_at = now();

-- name: ActiveProviderLimits :many
SELECT harness, until FROM provider_limits
WHERE workspace_id = @workspace_id AND until > now();

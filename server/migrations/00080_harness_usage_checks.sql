-- +goose Up
-- Requests to read a workspace's Claude or Codex plan allowance. The Docker
-- API image has no harness CLI, so the API queues a check here while someone
-- watches the activity orb, and a Docker runner answers it by asking the CLI
-- in a throwaway agent container with the saved login. Readings themselves go
-- to provider_usage.
CREATE TABLE harness_usage_checks (
  workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  harness      text NOT NULL CHECK (harness IN ('claude-code', 'codex')),
  -- The harness secret's version (see harness_model_listings) the latest
  -- request is for: a new login is checked at once, past any back-off.
  login_version text NOT NULL,
  -- pending: waiting for a runner. running: a runner is checking.
  status       text NOT NULL CHECK (status IN ('pending', 'running', 'done')),
  runner_id    text,
  -- Set by each claim, so a runner that lost its claim can't settle a newer one.
  claim_id     uuid,
  requested_at timestamptz NOT NULL DEFAULT now(),
  claimed_at   timestamptz,
  finished_at  timestamptz,
  -- No new request queues before this: a failed, signed-out or outdated
  -- check backs off instead of starting a container on every poll.
  retry_after  timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (workspace_id, harness)
);
CREATE INDEX harness_usage_checks_pending ON harness_usage_checks (requested_at) WHERE status = 'pending';

-- +goose Down
DROP TABLE harness_usage_checks;

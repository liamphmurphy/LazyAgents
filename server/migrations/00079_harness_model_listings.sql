-- +goose Up
-- The models a workspace's Claude or Codex plan login offers. Plan logins
-- can't list models over a provider API, only through the harness CLI, and
-- the Docker API image has none: the API queues a request here and a Docker
-- runner lists them in a throwaway agent container with the saved login.
CREATE TABLE harness_model_listings (
  workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  harness      text NOT NULL CHECK (harness IN ('claude-code', 'codex')),
  -- The harness secret's version (its encryption nonce, which changes on
  -- every write but not when the secret is merely read) the latest request
  -- is for: a changed login makes older results stale.
  login_version text NOT NULL,
  -- pending: waiting for a runner. running: a runner is listing.
  status       text NOT NULL CHECK (status IN ('pending', 'running', 'done', 'failed')),
  -- Fixed public-safe text only, never CLI output.
  error        text NOT NULL DEFAULT '',
  -- The last successful listing, kept while a newer request is in flight.
  models         jsonb NOT NULL DEFAULT '[]',
  models_version text, -- the login_version the models came from
  listed_at      timestamptz,
  runner_id    text,
  -- Set by each claim, so a runner that lost its claim can't settle a newer one.
  claim_id     uuid,
  requested_at timestamptz NOT NULL DEFAULT now(),
  claimed_at   timestamptz,
  finished_at  timestamptz,
  PRIMARY KEY (workspace_id, harness)
);
CREATE INDEX harness_model_listings_pending ON harness_model_listings (requested_at) WHERE status = 'pending';

-- +goose Down
DROP TABLE harness_model_listings;

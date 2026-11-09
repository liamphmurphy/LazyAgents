-- +goose Up
-- The latest reading of each harness's plan allowance (session and weekly
-- windows), from the CLI's usage check or the rate-limit events a run streams.
-- Like provider_limits, it is per workspace: each holds its own logins.
CREATE TABLE provider_usage (
  workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  harness      text NOT NULL,
  plan         text NOT NULL DEFAULT '',
  -- false when plan limits don't apply or can't be read (API key, signed out).
  available    boolean NOT NULL DEFAULT true,
  -- [{id, kind, label, used_percent, resets_at}] (usage.Window)
  windows      jsonb NOT NULL DEFAULT '[]',
  observed_at  timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (workspace_id, harness)
);

-- Which harnesses are working right now (the activity orb polls this).
CREATE INDEX runs_active_harness_idx ON runs (workspace_id, harness) WHERE state IN ('starting', 'running');

-- +goose Down
DROP INDEX runs_active_harness_idx;
DROP TABLE provider_usage;

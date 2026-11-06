-- +goose Up
-- A harness's usage limit is per account (one per CLI), so once any run hits it
-- every run in the workspace skips straight to its fallback until it resets.
CREATE TABLE provider_limits (
  workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  harness      text NOT NULL,
  until        timestamptz NOT NULL,
  reason       text NOT NULL DEFAULT '',
  updated_at   timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (workspace_id, harness)
);

-- +goose Down
DROP TABLE provider_limits;

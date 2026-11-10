-- +goose Up
-- The spec a repository's playground last started with. A person can start
-- a playground from it in any chat on the repository, without asking an
-- agent; a repository can only run one way at a time, so the latest wins.
CREATE TABLE playground_specs (
  workspace_id       uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  repo               text NOT NULL,
  spec               jsonb NOT NULL,
  saved_from_chat_id uuid REFERENCES chat_sessions(id) ON DELETE SET NULL,
  updated_at         timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (workspace_id, repo)
);

-- +goose Down
DROP TABLE playground_specs;

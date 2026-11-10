-- +goose Up
-- What a repository's tests need in a turn (testenv.Recipe): services to start
-- and Nix packages to install. Keyed by repository rather than project, so a
-- coding chat and ticket runs on the same repository share what they learned.
-- inferred_from lists the files the first recipe was read from; edited_at is
-- set once a person replaces it.
CREATE TABLE test_environments (
  workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  repo text NOT NULL,
  recipe jsonb NOT NULL DEFAULT '{}',
  inferred_from text[] NOT NULL DEFAULT '{}',
  learned_from_run_id uuid REFERENCES runs(id) ON DELETE SET NULL,
  edited_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (workspace_id, repo)
);

-- +goose Down
DROP TABLE test_environments;

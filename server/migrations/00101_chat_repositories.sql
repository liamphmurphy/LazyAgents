-- +goose Up
ALTER TABLE chat_sessions ADD COLUMN pending_repo_url text;

-- Saved sources are discovery shortcuts, never authorization rules.
CREATE TABLE repository_sources (
  workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  kind text NOT NULL CHECK (kind IN ('repository', 'organization')),
  name text NOT NULL,
  PRIMARY KEY (workspace_id, user_id, kind, name)
);
INSERT INTO repository_sources (workspace_id, user_id, kind, name)
SELECT DISTINCT workspace_id, user_id, 'repository', repo
FROM personas CROSS JOIN LATERAL unnest(repos) AS repo ON CONFLICT DO NOTHING;

CREATE TABLE chat_repositories (
  chat_id uuid NOT NULL REFERENCES chat_sessions(id) ON DELETE CASCADE,
  repo_url text NOT NULL,
  directory text NOT NULL,
  work_branches text[] NOT NULL DEFAULT '{}',
  checkpoint_branch text,
  PRIMARY KEY (chat_id, repo_url),
  UNIQUE (chat_id, directory)
);
-- Existing workspaces keep their root checkout, including uncommitted work.
INSERT INTO chat_repositories (chat_id, repo_url, directory, work_branches)
SELECT id, repo_url, '.', work_branches FROM chat_sessions WHERE repo_url IS NOT NULL;

-- +goose Down
ALTER TABLE chat_sessions DROP COLUMN pending_repo_url;
DROP TABLE chat_repositories;
DROP TABLE repository_sources;

-- +goose Up
-- The latest review of each repository a coding chat works in: its branch
-- against the default branch, committed and uncommitted, as the runner last
-- read it from the workspace. Kept after the workspace is gone, so the Code
-- dock still shows what the persona left. live is set while a turn runs.
CREATE TABLE chat_code_reviews (
  chat_id    uuid NOT NULL REFERENCES chat_sessions(id) ON DELETE CASCADE,
  repo_url   text NOT NULL,
  branch     text,
  base_sha   text,
  head_sha   text,
  commits    jsonb NOT NULL DEFAULT '[]',
  files      jsonb NOT NULL DEFAULT '[]',
  dirty      boolean NOT NULL DEFAULT false,
  live       boolean NOT NULL DEFAULT false,
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (chat_id, repo_url)
);
CREATE TRIGGER realtime_change_ins AFTER INSERT ON chat_code_reviews REFERENCING NEW TABLE AS new_rows FOR EACH STATEMENT EXECUTE FUNCTION notify_chat_change('code_review.updated', 'chat_id');
CREATE TRIGGER realtime_change_upd AFTER UPDATE ON chat_code_reviews REFERENCING NEW TABLE AS new_rows FOR EACH STATEMENT EXECUTE FUNCTION notify_chat_change('code_review.updated', 'chat_id');

-- Repositories whose push requests the person approved for good ("Always
-- allow" on a card): the runner pushes and opens the pull request itself.
-- Personas still never hold a credential that can push.
CREATE TABLE repository_push_policies (
  workspace_id uuid NOT NULL REFERENCES workspaces(id),
  user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  repo_url     text NOT NULL,
  auto_push    boolean NOT NULL DEFAULT false,
  updated_at   timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, repo_url)
);

-- A branch a persona asked to push from a coding chat. The persona's turn has
-- no credential that can push: at the end of the turn the runner copies the
-- branch's tip into a clean repository and pushes it to staging_ref, so the
-- exact commit reviewed is the one that ships, and approving needs neither the
-- workspace nor the persona. Approving points branch at head_sha through the
-- GitHub API and opens the pull request.
--   staging: asked for, waiting for the turn to end
--   pending: staged, waiting for the person
--   pushing: approved, the GitHub calls are under way
--   pushed | declined | failed: decided
CREATE TABLE push_requests (
  id           uuid PRIMARY KEY DEFAULT uuid_v7(),
  workspace_id uuid NOT NULL REFERENCES workspaces(id),
  user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  chat_id      uuid NOT NULL REFERENCES chat_sessions(id) ON DELETE CASCADE,
  run_id       uuid REFERENCES runs(id) ON DELETE SET NULL,
  persona_id   uuid REFERENCES personas(id) ON DELETE SET NULL,
  agent_id     uuid NOT NULL REFERENCES agents(id),
  repo_url     text NOT NULL,
  branch       text NOT NULL,
  base_branch  text,
  base_sha     text,
  head_sha     text,
  staging_ref  text,
  commits      jsonb NOT NULL DEFAULT '[]',
  files        jsonb NOT NULL DEFAULT '[]',
  title        text NOT NULL,
  body         text NOT NULL DEFAULT '',
  draft        boolean NOT NULL DEFAULT false,
  state        text NOT NULL DEFAULT 'staging'
    CHECK (state IN ('staging', 'pending', 'pushing', 'pushed', 'declined', 'failed')),
  auto         boolean NOT NULL DEFAULT false,
  pr_url       text,
  pr_number    integer,
  error        text,
  created_at   timestamptz NOT NULL DEFAULT now(),
  decided_at   timestamptz
);
CREATE INDEX push_requests_chat ON push_requests (chat_id, created_at);
CREATE INDEX push_requests_open ON push_requests (chat_id) WHERE state IN ('staging', 'pending', 'pushing');
CREATE TRIGGER realtime_change_ins AFTER INSERT ON push_requests REFERENCING NEW TABLE AS new_rows FOR EACH STATEMENT EXECUTE FUNCTION notify_chat_change('push_request.updated', 'chat_id');
CREATE TRIGGER realtime_change_upd AFTER UPDATE ON push_requests REFERENCING NEW TABLE AS new_rows FOR EACH STATEMENT EXECUTE FUNCTION notify_chat_change('push_request.updated', 'chat_id');

-- +goose Down
DROP TABLE push_requests;
DROP TABLE repository_push_policies;
DROP TABLE chat_code_reviews;

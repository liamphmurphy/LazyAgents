-- +goose Up
-- A Claude, Codex or GitHub sign-in started from Settings. The Docker runner
-- runs the provider's own CLI in a throwaway container, shows its link and
-- code here, and stores the resulting login as a secret.
CREATE TABLE sign_ins (
  id           uuid PRIMARY KEY DEFAULT uuid_v7(),
  workspace_id uuid NOT NULL REFERENCES workspaces(id),
  provider     text NOT NULL CHECK (provider IN ('claude', 'codex', 'github')),
  -- pending: waiting for a runner. running: the CLI is up (url set once it
  -- printed one). verifying: Claude's pasted code was handed to the CLI.
  status       text NOT NULL DEFAULT 'pending'
               CHECK (status IN ('pending', 'running', 'verifying', 'done', 'failed', 'cancelled')),
  url          text NOT NULL DEFAULT '',
  user_code    text NOT NULL DEFAULT '',
  -- The code Claude's sign-in page shows, held only until the runner reads it.
  pasted_code  text,
  error        text NOT NULL DEFAULT '',
  runner_id    text,
  requested_by uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  created_at   timestamptz NOT NULL DEFAULT now(),
  updated_at   timestamptz NOT NULL DEFAULT now(),
  expires_at   timestamptz NOT NULL
);
-- One sign-in in progress per provider.
CREATE UNIQUE INDEX sign_ins_active ON sign_ins (workspace_id, provider)
  WHERE status IN ('pending', 'running', 'verifying');
CREATE INDEX sign_ins_pending ON sign_ins (created_at) WHERE status = 'pending';

-- +goose Down
DROP TABLE sign_ins;

-- +goose Up
-- Phase 4b: agent tokens and encrypted secrets (PLAN.md §4, §5.4, §6.3).

-- Bearer tokens agents use on /mcp and /api/v1/agent. Stored as sha256;
-- the token itself is shown once. Managed runs get kind=run tokens that
-- expire with the run (Phase 5); people mint unmanaged ones.
CREATE TABLE agent_tokens (
  id           uuid PRIMARY KEY DEFAULT uuid_v7(),
  agent_id     uuid NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
  token_hash   bytea NOT NULL UNIQUE,
  prefix       text NOT NULL,                 -- "la_3fQk" for display
  kind         text NOT NULL CHECK (kind IN ('run', 'unmanaged')),
  label        text NOT NULL DEFAULT '',
  run_id       uuid,                          -- FK once runs exist (4c)
  created_by_user_id uuid REFERENCES users(id),
  created_at   timestamptz NOT NULL DEFAULT now(),
  expires_at   timestamptz,
  last_used_at timestamptz,
  revoked_at   timestamptz
);
CREATE INDEX agent_tokens_agent_idx ON agent_tokens (agent_id);

-- Secret values are AES-256-GCM ciphertext; the key comes from SECRETS_KEY
-- and never touches the database. Write-only through the API.
CREATE TABLE secrets (
  id           uuid PRIMARY KEY DEFAULT uuid_v7(),
  workspace_id uuid NOT NULL REFERENCES workspaces(id),
  project_id   uuid REFERENCES projects(id) ON DELETE CASCADE,   -- NULL = workspace-wide
  name         text NOT NULL CHECK (name ~ '^[a-z][a-z0-9-]*:[a-z0-9][a-z0-9-]*$'),
  grants_scope text,                          -- the scope this secret backs, e.g. db:migrate
  nonce        bytea NOT NULL,
  ciphertext   bytea NOT NULL,
  preview      text NOT NULL DEFAULT '',      -- last 4 characters
  created_by_user_id uuid REFERENCES users(id),
  created_at   timestamptz NOT NULL DEFAULT now(),
  updated_at   timestamptz NOT NULL DEFAULT now(),
  last_used_at timestamptz
);
-- One secret per name per scope (workspace, or each project).
CREATE UNIQUE INDEX secrets_name_key ON secrets (workspace_id, coalesce(project_id, '00000000-0000-0000-0000-000000000000'::uuid), name);
CREATE TRIGGER secrets_touch BEFORE UPDATE ON secrets FOR EACH ROW EXECUTE FUNCTION touch_updated_at();

-- +goose Down
DROP TABLE secrets;
DROP TABLE agent_tokens;

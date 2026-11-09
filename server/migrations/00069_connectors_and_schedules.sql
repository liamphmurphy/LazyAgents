-- +goose Up
-- MCP connectors a person adds and scheduled tasks that run on their own.
ALTER TABLE personas ADD COLUMN can_create_connectors boolean NOT NULL DEFAULT false;
-- NULL: every connector the persona may use. A scheduled task narrows it.
ALTER TABLE chat_sessions ADD COLUMN connector_ids uuid[];

CREATE TABLE connectors (
  id              uuid PRIMARY KEY DEFAULT uuid_v7(),
  workspace_id    uuid NOT NULL REFERENCES workspaces(id),
  user_id         uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name            text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 60),
  slug            text NOT NULL CHECK (slug ~ '^[a-z][a-z0-9-]*$'),
  url             text NOT NULL,
  auth_kind       text NOT NULL CHECK (auth_kind IN ('oauth', 'token', 'none')),
  status          text NOT NULL DEFAULT 'needs_sign_in' CHECK (status IN ('connected', 'needs_sign_in', 'offline')),
  last_error      text NOT NULL DEFAULT '',
  last_checked_at timestamptz,
  -- [{name, description, input_schema, write, policy}]; policy: allow, ask, off.
  tools           jsonb NOT NULL DEFAULT '[]',
  cred_nonce      bytea,
  cred_ciphertext bytea,
  -- Client registration and endpoints, plus the state of a sign-in in progress.
  oauth           jsonb NOT NULL DEFAULT '{}',
  oauth_state     text,
  revision        integer NOT NULL DEFAULT 1,
  created_at      timestamptz NOT NULL DEFAULT now(),
  updated_at      timestamptz NOT NULL DEFAULT now(),
  UNIQUE (workspace_id, user_id, slug)
);
CREATE UNIQUE INDEX connectors_oauth_state ON connectors (oauth_state) WHERE oauth_state IS NOT NULL;

CREATE TABLE connector_personas (
  connector_id uuid NOT NULL REFERENCES connectors(id) ON DELETE CASCADE,
  persona_id   uuid NOT NULL REFERENCES personas(id) ON DELETE CASCADE,
  PRIMARY KEY (connector_id, persona_id)
);
CREATE INDEX connector_personas_persona ON connector_personas (persona_id);

CREATE TABLE connector_approvals (
  id           uuid PRIMARY KEY DEFAULT uuid_v7(),
  workspace_id uuid NOT NULL REFERENCES workspaces(id),
  user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  connector_id uuid NOT NULL REFERENCES connectors(id) ON DELETE CASCADE,
  persona_id   uuid REFERENCES personas(id) ON DELETE SET NULL,
  chat_id      uuid NOT NULL REFERENCES chat_sessions(id) ON DELETE CASCADE,
  tool         text NOT NULL,
  arguments    jsonb NOT NULL DEFAULT '{}',
  state        text NOT NULL DEFAULT 'pending' CHECK (state IN ('pending', 'allowed', 'denied', 'expired')),
  created_at   timestamptz NOT NULL DEFAULT now(),
  decided_at   timestamptz
);
CREATE INDEX connector_approvals_chat ON connector_approvals (chat_id, created_at) WHERE state = 'pending';

CREATE TABLE connector_proposals (
  id           uuid PRIMARY KEY DEFAULT uuid_v7(),
  chat_id      uuid NOT NULL REFERENCES chat_sessions(id) ON DELETE CASCADE,
  run_id       uuid NOT NULL REFERENCES runs(id),
  proposed_by  uuid REFERENCES personas(id) ON DELETE SET NULL,
  name         text NOT NULL,
  url          text NOT NULL,
  auth_kind    text NOT NULL CHECK (auth_kind IN ('oauth', 'token', 'none')),
  reason       text NOT NULL DEFAULT '',
  state        text NOT NULL DEFAULT 'pending' CHECK (state IN ('pending', 'approved', 'declined')),
  connector_id uuid REFERENCES connectors(id) ON DELETE SET NULL,
  created_at   timestamptz NOT NULL DEFAULT now(),
  decided_at   timestamptz
);
CREATE INDEX connector_proposals_chat ON connector_proposals (chat_id, created_at);

CREATE TABLE scheduled_tasks (
  id              uuid PRIMARY KEY DEFAULT uuid_v7(),
  workspace_id    uuid NOT NULL REFERENCES workspaces(id),
  user_id         uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name            text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 120),
  kind            text NOT NULL CHECK (kind IN ('prompt', 'workflow')),
  instructions    text NOT NULL DEFAULT '',
  workflow_id     uuid REFERENCES workflows(id) ON DELETE CASCADE,
  persona_id      uuid REFERENCES personas(id) ON DELETE SET NULL,
  project_id      uuid REFERENCES projects(id) ON DELETE SET NULL,
  connector_ids   uuid[] NOT NULL DEFAULT '{}',
  frequency       text NOT NULL CHECK (frequency IN ('daily', 'weekly', 'monthly', 'cron')),
  weekdays        integer[] NOT NULL DEFAULT '{}',
  month_day       integer NOT NULL DEFAULT 1 CHECK (month_day BETWEEN 1 AND 31),
  at_time         text NOT NULL DEFAULT '09:00',
  timezone        text NOT NULL DEFAULT 'UTC',
  cron            text NOT NULL DEFAULT '',
  enabled         boolean NOT NULL DEFAULT true,
  max_minutes     integer NOT NULL DEFAULT 15 CHECK (max_minutes BETWEEN 1 AND 240),
  max_cost_micros bigint NOT NULL DEFAULT 1000000 CHECK (max_cost_micros > 0),
  next_run_at     timestamptz,
  revision        integer NOT NULL DEFAULT 1,
  created_at      timestamptz NOT NULL DEFAULT now(),
  updated_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX scheduled_tasks_due ON scheduled_tasks (next_run_at) WHERE enabled;
CREATE INDEX scheduled_tasks_user ON scheduled_tasks (workspace_id, user_id, created_at);

CREATE TABLE scheduled_task_runs (
  id              uuid PRIMARY KEY DEFAULT uuid_v7(),
  task_id         uuid NOT NULL REFERENCES scheduled_tasks(id) ON DELETE CASCADE,
  chat_id         uuid REFERENCES chat_sessions(id) ON DELETE SET NULL,
  workflow_run_id uuid REFERENCES workflow_runs(id) ON DELETE SET NULL,
  trigger         text NOT NULL CHECK (trigger IN ('schedule', 'manual', 'catchup')),
  state           text NOT NULL DEFAULT 'running' CHECK (state IN ('running', 'waiting', 'done', 'failed')),
  summary         text NOT NULL DEFAULT '',
  error           text NOT NULL DEFAULT '',
  cost_micros     bigint NOT NULL DEFAULT 0,
  started_at      timestamptz NOT NULL DEFAULT now(),
  finished_at     timestamptz
);
CREATE INDEX scheduled_task_runs_task ON scheduled_task_runs (task_id, started_at DESC);
CREATE INDEX scheduled_task_runs_active ON scheduled_task_runs (started_at) WHERE state IN ('running', 'waiting');

-- +goose Down
DROP TABLE scheduled_task_runs;
DROP TABLE scheduled_tasks;
DROP TABLE connector_proposals;
DROP TABLE connector_approvals;
DROP TABLE connector_personas;
DROP TABLE connectors;
ALTER TABLE chat_sessions DROP COLUMN connector_ids;
ALTER TABLE personas DROP COLUMN can_create_connectors;

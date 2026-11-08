-- +goose Up
-- Workflows: a person-authored DAG Vesper executes step by step. The draft is
-- edited freely; publishing freezes a copy, and every run keeps its own
-- snapshot of the graph it started with.
ALTER TABLE runs DROP CONSTRAINT runs_purpose_check;
ALTER TABLE runs ADD CONSTRAINT runs_purpose_check CHECK (purpose IN ('ticket', 'planning', 'chat', 'research', 'workflow'));

ALTER TABLE library_items DROP CONSTRAINT library_items_kind_check;
ALTER TABLE library_items ADD CONSTRAINT library_items_kind_check CHECK (kind IN ('image', 'diagram', 'link', 'document'));

CREATE TABLE workflows (
  id                uuid PRIMARY KEY DEFAULT uuid_v7(),
  workspace_id      uuid NOT NULL REFERENCES workspaces(id),
  user_id           uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name              text NOT NULL CHECK (length(name) BETWEEN 1 AND 120),
  description       text NOT NULL DEFAULT '' CHECK (length(description) <= 1000),
  draft             jsonb NOT NULL,
  published         jsonb,
  published_version integer NOT NULL DEFAULT 0,
  created_at        timestamptz NOT NULL DEFAULT now(),
  updated_at        timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX workflows_owner ON workflows (workspace_id, user_id, updated_at DESC);

-- A persona's suggested change to a draft; nothing changes until it's applied.
CREATE TABLE workflow_proposals (
  id          uuid PRIMARY KEY DEFAULT uuid_v7(),
  workflow_id uuid NOT NULL REFERENCES workflows(id) ON DELETE CASCADE,
  chat_id     uuid REFERENCES chat_sessions(id) ON DELETE SET NULL,
  summary     text NOT NULL CHECK (length(summary) BETWEEN 1 AND 500),
  ops         jsonb NOT NULL,
  graph       jsonb NOT NULL,
  state       text NOT NULL DEFAULT 'pending' CHECK (state IN ('pending', 'applied', 'discarded')),
  created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX workflow_proposals_pending ON workflow_proposals (workflow_id, created_at) WHERE state = 'pending';

CREATE TABLE workflow_runs (
  id            uuid PRIMARY KEY DEFAULT uuid_v7(),
  workspace_id  uuid NOT NULL REFERENCES workspaces(id),
  user_id       uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  chat_id       uuid NOT NULL REFERENCES chat_sessions(id) ON DELETE CASCADE,
  project_id    uuid NOT NULL REFERENCES projects(id),
  workflow_id   uuid REFERENCES workflows(id) ON DELETE SET NULL,
  -- The reply that proposed this run; a /workflow start has none.
  message_id    uuid UNIQUE REFERENCES chat_messages(id) ON DELETE SET NULL,
  name          text NOT NULL,
  version       integer NOT NULL,
  graph         jsonb NOT NULL,
  input         text NOT NULL,
  state         text NOT NULL DEFAULT 'running' CHECK (state IN ('running', 'waiting', 'done', 'failed', 'stopped')),
  revision_note text NOT NULL DEFAULT '',
  error         text NOT NULL DEFAULT '',
  created_at    timestamptz NOT NULL DEFAULT now(),
  finished_at   timestamptz
);
CREATE INDEX workflow_runs_chat ON workflow_runs (chat_id, created_at);
CREATE INDEX workflow_runs_owner ON workflow_runs (workspace_id, user_id, created_at DESC);
CREATE INDEX workflow_runs_active ON workflow_runs (created_at) WHERE state IN ('running', 'waiting');

-- One execution of one node. "Request changes" marks steps stale and the node
-- runs again as a new attempt, so the graph itself stays acyclic.
CREATE TABLE workflow_steps (
  id           uuid PRIMARY KEY DEFAULT uuid_v7(),
  run_id       uuid NOT NULL REFERENCES workflow_runs(id) ON DELETE CASCADE,
  node_id      text NOT NULL,
  attempt      integer NOT NULL,
  state        text NOT NULL CHECK (state IN ('running', 'waiting', 'done', 'failed', 'stale')),
  output       text NOT NULL DEFAULT '',
  agent_run_id uuid UNIQUE REFERENCES runs(id) ON DELETE SET NULL,
  decision     text NOT NULL DEFAULT '',
  note         text NOT NULL DEFAULT '',
  created_at   timestamptz NOT NULL DEFAULT now(),
  finished_at  timestamptz,
  UNIQUE (run_id, node_id, attempt)
);
CREATE INDEX workflow_steps_waiting ON workflow_steps (run_id) WHERE state = 'waiting';
CREATE TRIGGER realtime_change AFTER INSERT OR DELETE OR UPDATE ON workflow_steps FOR EACH STATEMENT EXECUTE FUNCTION notify_change('approval.updated');

-- +goose Down
DROP TABLE workflow_steps;
DROP TABLE workflow_runs;
DROP TABLE workflow_proposals;
DROP TABLE workflows;
DELETE FROM runs WHERE purpose = 'workflow';
ALTER TABLE runs DROP CONSTRAINT runs_purpose_check;
ALTER TABLE runs ADD CONSTRAINT runs_purpose_check CHECK (purpose IN ('ticket', 'planning', 'chat', 'research'));
DELETE FROM library_items WHERE kind = 'document';
ALTER TABLE library_items DROP CONSTRAINT library_items_kind_check;
ALTER TABLE library_items ADD CONSTRAINT library_items_kind_check CHECK (kind IN ('image', 'diagram', 'link'));

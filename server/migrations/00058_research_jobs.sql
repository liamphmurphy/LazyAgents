-- +goose Up
-- Deep research: a person approves a plan, one read-only run researches each
-- question, and the chat's persona writes up what they found.
ALTER TABLE runs DROP CONSTRAINT runs_purpose_check;
ALTER TABLE runs ADD CONSTRAINT runs_purpose_check CHECK (purpose IN ('ticket', 'planning', 'chat', 'research'));

CREATE TABLE research_jobs (
  id                 uuid PRIMARY KEY DEFAULT uuid_v7(),
  workspace_id       uuid NOT NULL REFERENCES workspaces(id),
  chat_id            uuid NOT NULL REFERENCES chat_sessions(id) ON DELETE CASCADE,
  user_id            uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  -- The reply holding the plan; a chat edit may remove it.
  message_id         uuid UNIQUE REFERENCES chat_messages(id) ON DELETE SET NULL,
  title              text NOT NULL CHECK (length(title) BETWEEN 1 AND 200),
  state              text NOT NULL DEFAULT 'running' CHECK (state IN ('running', 'finishing', 'ready', 'writing', 'done', 'stopped', 'failed')),
  max_sources        integer NOT NULL,
  max_minutes        integer NOT NULL,
  -- NULL is unlimited: a subscription accrues no real API cost.
  max_cost_micros    bigint,
  stop_reason        text NOT NULL DEFAULT '',
  error              text NOT NULL DEFAULT '',
  report_artifact_id uuid REFERENCES artifacts(id) ON DELETE SET NULL,
  report             jsonb,
  created_at         timestamptz NOT NULL DEFAULT now(),
  finished_at        timestamptz
);
CREATE INDEX research_jobs_chat ON research_jobs (chat_id, created_at);
CREATE INDEX research_jobs_active ON research_jobs (state) WHERE state IN ('running', 'finishing', 'ready', 'writing');

CREATE TABLE research_tasks (
  id        uuid PRIMARY KEY DEFAULT uuid_v7(),
  job_id    uuid NOT NULL REFERENCES research_jobs(id) ON DELETE CASCADE,
  run_id    uuid NOT NULL UNIQUE REFERENCES runs(id) ON DELETE CASCADE,
  seq       integer NOT NULL,
  label     text NOT NULL,
  question  text NOT NULL,
  findings  jsonb NOT NULL DEFAULT '[]'
);
CREATE INDEX research_tasks_job ON research_tasks (job_id, seq);

-- +goose Down
DROP TABLE research_tasks;
DROP TABLE research_jobs;
DELETE FROM runs WHERE purpose = 'research';
ALTER TABLE runs DROP CONSTRAINT runs_purpose_check;
ALTER TABLE runs ADD CONSTRAINT runs_purpose_check CHECK (purpose IN ('ticket', 'planning', 'chat'));

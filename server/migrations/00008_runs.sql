-- +goose Up
-- Phase 4c: runs, turns, events, plans and approvals (PLAN.md §4, §6.4).

CREATE TABLE runs (
  id                    uuid PRIMARY KEY DEFAULT uuid_v7(),
  workspace_id          uuid NOT NULL REFERENCES workspaces(id),
  ticket_id             uuid NOT NULL REFERENCES tickets(id) ON DELETE CASCADE,
  agent_id              uuid NOT NULL REFERENCES agents(id),
  mode                  text NOT NULL CHECK (mode IN ('managed', 'unmanaged')),
  state                 text NOT NULL CHECK (state IN ('queued', 'starting', 'running', 'waiting', 'paused', 'blocked', 'completed', 'failed', 'cancelled')),
  wait_reason           text CHECK (wait_reason IN ('approval', 'human_reply', 'pause')),
  -- The definition merged with the project's override, frozen at start.
  resolved_spec         jsonb NOT NULL,
  definition_version_id uuid REFERENCES agent_definition_versions(id),
  harness               text NOT NULL,
  model                 text NOT NULL,
  harness_session_id    text,
  outcome               text,
  tool_calls            integer NOT NULL DEFAULT 0,
  input_tokens          bigint NOT NULL DEFAULT 0,
  output_tokens         bigint NOT NULL DEFAULT 0,
  cost_usd_micros       bigint NOT NULL DEFAULT 0,
  cost_source           text CHECK (cost_source IN ('harness', 'computed', 'reported')),
  started_at            timestamptz NOT NULL DEFAULT now(),
  ended_at              timestamptz,
  created_at            timestamptz NOT NULL DEFAULT now(),
  updated_at            timestamptz NOT NULL DEFAULT now()
);
-- One agent per ticket, one live run per ticket (PLAN.md §6.6).
CREATE UNIQUE INDEX runs_one_active_per_ticket ON runs (ticket_id)
  WHERE state NOT IN ('completed', 'failed', 'cancelled');
CREATE INDEX runs_agent_idx ON runs (agent_id, state);
CREATE TRIGGER runs_touch BEFORE UPDATE ON runs FOR EACH ROW EXECUTE FUNCTION touch_updated_at();

ALTER TABLE agent_tokens ADD CONSTRAINT agent_tokens_run_fk FOREIGN KEY (run_id) REFERENCES runs(id) ON DELETE CASCADE;
ALTER TABLE artifact_reads ADD CONSTRAINT artifact_reads_run_fk FOREIGN KEY (run_id) REFERENCES runs(id) ON DELETE SET NULL;

-- Each headless invocation (managed) or resumption after a wait.
CREATE TABLE run_turns (
  id            uuid PRIMARY KEY DEFAULT uuid_v7(),
  run_id        uuid NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
  seq           integer NOT NULL,
  trigger       text NOT NULL CHECK (trigger IN ('start', 'approval', 'comment', 'resume')),
  input_message text NOT NULL DEFAULT '',
  started_at    timestamptz NOT NULL DEFAULT now(),
  ended_at      timestamptz,
  exit_code     integer,
  UNIQUE (run_id, seq)
);

-- Normalized events for the run log.
CREATE TABLE run_events (
  id      uuid PRIMARY KEY DEFAULT uuid_v7(),
  run_id  uuid NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
  seq     bigint NOT NULL GENERATED ALWAYS AS IDENTITY,
  type    text NOT NULL CHECK (type IN ('message', 'tool_call', 'tool_result', 'usage', 'error', 'lifecycle')),
  payload jsonb NOT NULL DEFAULT '{}',
  at      timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX run_events_run_idx ON run_events (run_id, seq);

CREATE TABLE plan_steps (
  id       uuid PRIMARY KEY DEFAULT uuid_v7(),
  run_id   uuid NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
  position integer NOT NULL,
  text     text NOT NULL CHECK (length(text) BETWEEN 1 AND 500),
  state    text NOT NULL DEFAULT 'pending' CHECK (state IN ('pending', 'current', 'done', 'needs_human', 'failed')),
  UNIQUE (run_id, position)
);

-- An agent asking a person before a requires_approval scope. For brokered
-- scopes the approved action_detail is exactly what run_gated executes.
CREATE TABLE approval_requests (
  id                 uuid PRIMARY KEY DEFAULT uuid_v7(),
  workspace_id       uuid NOT NULL REFERENCES workspaces(id),
  run_id             uuid NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
  ticket_id          uuid NOT NULL REFERENCES tickets(id) ON DELETE CASCADE,
  agent_id           uuid NOT NULL REFERENCES agents(id),
  scope              text NOT NULL,
  action_summary     text NOT NULL CHECK (length(action_summary) BETWEEN 1 AND 300),
  action_detail      jsonb NOT NULL DEFAULT '{}',
  risk_summary       text NOT NULL DEFAULT '',
  state              text NOT NULL DEFAULT 'pending' CHECK (state IN ('pending', 'approved', 'denied', 'expired')),
  decided_by_user_id uuid REFERENCES users(id),
  decided_at         timestamptz,
  decision_note      text NOT NULL DEFAULT '',
  executed_at        timestamptz,              -- run_gated ran it (once)
  result             jsonb,
  created_at         timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX approval_requests_pending_idx ON approval_requests (workspace_id, created_at) WHERE state = 'pending';
CREATE INDEX approval_requests_ticket_idx ON approval_requests (ticket_id, created_at);

-- +goose Down
DROP TABLE approval_requests, plan_steps, run_events, run_turns;
ALTER TABLE artifact_reads DROP CONSTRAINT artifact_reads_run_fk;
ALTER TABLE agent_tokens DROP CONSTRAINT agent_tokens_run_fk;
DROP TABLE runs;

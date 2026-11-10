-- +goose Up
-- A chat's playground: the app an agent is working on, running from the
-- chat's workspace with fresh services next to it, for the person to click
-- through. One per chat. The API records what to run (playground.Spec) and
-- what the person or agent asked for; the runner that holds the workspace
-- claims it, starts the containers, serves it on one of its ports, and
-- clears each requested action once it has carried it out.
CREATE TABLE playgrounds (
  id            uuid PRIMARY KEY DEFAULT uuid_v7(),
  workspace_id  uuid NOT NULL REFERENCES workspaces(id),
  user_id       uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  chat_id       uuid NOT NULL UNIQUE REFERENCES chat_sessions(id) ON DELETE CASCADE,
  -- The chat segment whose workspace it runs. A later segment has a new
  -- workspace, so starting again moves it there.
  run_id        uuid REFERENCES runs(id) ON DELETE SET NULL,
  spec          jsonb NOT NULL,
  state         text NOT NULL DEFAULT 'starting' CHECK (state IN ('starting', 'running', 'stopped', 'failed')),
  -- Asked for while starting or running; the runner clears it when done.
  action        text CHECK (action IN ('reset', 'restart', 'stop')),
  -- The runner install (RUNNER_STACK) and process holding it, and its port.
  stack         text,
  runner_id     text,
  port          integer CHECK (port BETWEEN 1 AND 65535),
  -- How long the holding runner lets it sit unused (RUNNER_PLAYGROUND_IDLE_S).
  idle_ttl_s    integer CHECK (idle_ttl_s > 0),
  -- The branch checked out when it started, for the person's orientation.
  branch        text NOT NULL DEFAULT '',
  -- What it's doing while it starts, e.g. "Running setup".
  step          text NOT NULL DEFAULT '',
  error         text,
  error_log     text NOT NULL DEFAULT '',
  stop_reason   text NOT NULL DEFAULT '',
  started_at    timestamptz,
  -- When its services last started empty (start or Reset data).
  data_reset_at timestamptz,
  last_used_at  timestamptz,
  created_at    timestamptz NOT NULL DEFAULT now(),
  updated_at    timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT playgrounds_error CHECK ((error IS NOT NULL) = (state = 'failed')),
  CONSTRAINT playgrounds_held CHECK (state IN ('starting', 'running') OR (runner_id IS NULL AND port IS NULL AND action IS NULL))
);
-- A runner serves each active playground on a port of its own.
CREATE UNIQUE INDEX playgrounds_port ON playgrounds (stack, port) WHERE port IS NOT NULL;
CREATE INDEX playgrounds_active ON playgrounds (stack) WHERE state IN ('starting', 'running');
CREATE INDEX playgrounds_run ON playgrounds (run_id);

CREATE TRIGGER realtime_change_ins AFTER INSERT ON playgrounds REFERENCING NEW TABLE AS new_rows FOR EACH STATEMENT EXECUTE FUNCTION notify_chat_change('playground.updated', 'chat_id');
CREATE TRIGGER realtime_change_upd AFTER UPDATE ON playgrounds REFERENCING NEW TABLE AS new_rows FOR EACH STATEMENT EXECUTE FUNCTION notify_chat_change('playground.updated', 'chat_id');

-- +goose Down
DROP TABLE playgrounds;

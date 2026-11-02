-- +goose Up
-- Phase 5: the runner (PLAN.md §6.4–6.7). Managed runs are dispatched by a
-- person, queued, and executed turn by turn by `lazyagents runner`.

-- A managed turn is pending until the runner starts its process, running
-- while the harness runs, and done when the process has exited.
-- Unmanaged turns are bookkeeping only and are created done.
ALTER TABLE run_turns ADD COLUMN status text NOT NULL DEFAULT 'done'
  CHECK (status IN ('pending', 'running', 'done'));
ALTER TABLE run_turns ADD COLUMN process_started_at timestamptz;
ALTER TABLE run_turns ADD COLUMN log_key text;            -- raw harness stream in run-logs
ALTER TABLE run_turns ADD COLUMN usage jsonb NOT NULL DEFAULT '{}';
CREATE INDEX run_turns_pending_idx ON run_turns (run_id) WHERE status <> 'done';

ALTER TABLE runs ADD COLUMN dispatched_by_user_id uuid REFERENCES users(id);
ALTER TABLE runs ADD COLUMN runner_id text;              -- which runner owns it
ALTER TABLE runs ADD COLUMN workspace_volume text;       -- persists between turns
-- Turns that ended without complete_run, await_human or request_approval.
-- The runner nudges once, then hands the run to a person.
ALTER TABLE runs ADD COLUMN idle_turns integer NOT NULL DEFAULT 0;
CREATE INDEX runs_queued_idx ON runs (started_at) WHERE state = 'queued';

-- Runner heartbeats: the UI shows runner slots and an offline banner.
CREATE TABLE runners (
  id             text PRIMARY KEY,                       -- hostname-pid
  backend        text NOT NULL CHECK (backend IN ('docker', 'local')),
  max_concurrent integer NOT NULL,
  running        integer NOT NULL DEFAULT 0,
  version        text NOT NULL DEFAULT '',
  started_at     timestamptz NOT NULL DEFAULT now(),
  heartbeat_at   timestamptz NOT NULL DEFAULT now()
);

-- PR sync (§6.7): when the job last looked, and a closed-unmerged flag.
ALTER TABLE tickets ADD COLUMN pr_synced_at timestamptz;
ALTER TABLE tickets ADD COLUMN pr_closed_unmerged boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE tickets DROP COLUMN pr_closed_unmerged, DROP COLUMN pr_synced_at;
DROP TABLE runners;
DROP INDEX runs_queued_idx;
ALTER TABLE runs DROP COLUMN idle_turns, DROP COLUMN workspace_volume, DROP COLUMN runner_id, DROP COLUMN dispatched_by_user_id;
DROP INDEX run_turns_pending_idx;
ALTER TABLE run_turns DROP COLUMN usage, DROP COLUMN log_key, DROP COLUMN process_started_at, DROP COLUMN status;

-- +goose Up
-- Main agents take tickets; sub agents only run when a main agent delegates to
-- them (PLAN.md §6.8). A delegated run is a child of the main agent's run: same
-- ticket, same workspace and branch, one at a time.
ALTER TABLE agents ADD COLUMN kind text NOT NULL DEFAULT 'main' CHECK (kind IN ('main', 'sub'));

ALTER TABLE runs ADD COLUMN parent_run_id uuid REFERENCES runs(id) ON DELETE CASCADE;
CREATE INDEX runs_parent_idx ON runs (parent_run_id) WHERE parent_run_id IS NOT NULL;

ALTER TABLE runs DROP CONSTRAINT runs_wait_reason_check;
ALTER TABLE runs ADD CONSTRAINT runs_wait_reason_check CHECK (wait_reason IN ('approval', 'human_reply', 'pause', 'delegation'));
ALTER TABLE run_turns DROP CONSTRAINT run_turns_trigger_check;
ALTER TABLE run_turns ADD CONSTRAINT run_turns_trigger_check CHECK (trigger IN ('start', 'approval', 'comment', 'resume', 'delegation'));

-- The one-live-run rule is per ticket for top-level runs; delegated runs ride on their parent.
DROP INDEX runs_one_active_per_ticket;
CREATE UNIQUE INDEX runs_one_active_per_ticket ON runs (ticket_id)
  WHERE parent_run_id IS NULL AND state NOT IN ('completed', 'failed', 'cancelled');
-- At most one live delegated run per parent: they share one checkout.
CREATE UNIQUE INDEX runs_one_active_child ON runs (parent_run_id)
  WHERE parent_run_id IS NOT NULL AND state NOT IN ('completed', 'failed', 'cancelled');

-- +goose Down
DROP INDEX runs_one_active_child;
DROP INDEX runs_one_active_per_ticket;
DELETE FROM runs WHERE parent_run_id IS NOT NULL;
CREATE UNIQUE INDEX runs_one_active_per_ticket ON runs (ticket_id)
  WHERE state NOT IN ('completed', 'failed', 'cancelled');
ALTER TABLE run_turns DROP CONSTRAINT run_turns_trigger_check;
ALTER TABLE run_turns ADD CONSTRAINT run_turns_trigger_check CHECK (trigger IN ('start', 'approval', 'comment', 'resume'));
ALTER TABLE runs DROP CONSTRAINT runs_wait_reason_check;
ALTER TABLE runs ADD CONSTRAINT runs_wait_reason_check CHECK (wait_reason IN ('approval', 'human_reply', 'pause'));
DROP INDEX runs_parent_idx;
ALTER TABLE runs DROP COLUMN parent_run_id;
ALTER TABLE agents DROP COLUMN kind;

-- +goose Up
-- A run out of provider usage with no fallback left is held instead of failed,
-- and resumes at hold_until. hold_started_at and hold_attempts span consecutive
-- limited turns; a turn that gets past the limit clears them.
ALTER TABLE runs DROP CONSTRAINT runs_state_check;
ALTER TABLE runs ADD CONSTRAINT runs_state_check CHECK (state = ANY (ARRAY['queued', 'starting', 'running', 'waiting', 'waiting_for_quota', 'paused', 'blocked', 'completed', 'failed', 'cancelled']));
ALTER TABLE runs ADD COLUMN hold_until timestamptz;
ALTER TABLE runs ADD COLUMN hold_started_at timestamptz;
ALTER TABLE runs ADD COLUMN hold_attempts integer NOT NULL DEFAULT 0;
ALTER TABLE runs ADD COLUMN hold_reset_known boolean NOT NULL DEFAULT false;
CREATE INDEX runs_held_idx ON runs (hold_until) WHERE state = 'waiting_for_quota';
-- Where a ticket run's unpushed work went before its workspace was given up.
-- checkpoint_at is set once the attempt succeeded, even with nothing to save.
ALTER TABLE runs ADD COLUMN checkpoint_branch text;
ALTER TABLE runs ADD COLUMN checkpoint_at timestamptz;
ALTER TABLE runs ADD COLUMN checkpoint_error text;

-- +goose Down
UPDATE runs SET state = 'failed', ended_at = now() WHERE state = 'waiting_for_quota';
DROP INDEX runs_held_idx;
ALTER TABLE runs DROP COLUMN checkpoint_error, DROP COLUMN checkpoint_at, DROP COLUMN checkpoint_branch,
  DROP COLUMN hold_reset_known, DROP COLUMN hold_attempts, DROP COLUMN hold_started_at, DROP COLUMN hold_until;
ALTER TABLE runs DROP CONSTRAINT runs_state_check;
ALTER TABLE runs ADD CONSTRAINT runs_state_check CHECK (state = ANY (ARRAY['queued', 'starting', 'running', 'waiting', 'paused', 'blocked', 'completed', 'failed', 'cancelled']));

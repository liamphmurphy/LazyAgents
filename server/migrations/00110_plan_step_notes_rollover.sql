-- +goose Up
-- A handoff note the agent leaves when it marks a step done: what the next
-- step needs that the code doesn't show. Shown in the run's state brief.
ALTER TABLE plan_steps ADD COLUMN note text;
-- Set by update_step(done) when a large lead should continue in a fresh
-- session; the runner clears it at the next turn start.
ALTER TABLE runs ADD COLUMN rollover_requested boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE runs DROP COLUMN rollover_requested;
ALTER TABLE plan_steps DROP COLUMN note;

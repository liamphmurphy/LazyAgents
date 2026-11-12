-- +goose Up
-- A delegated run's full result for its main agent (complete_run report),
-- rendered in the parent's wake message. NULL for runs that sent none.
ALTER TABLE runs ADD COLUMN report text;

-- +goose Down
ALTER TABLE runs DROP COLUMN report;

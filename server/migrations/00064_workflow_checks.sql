-- +goose Up
-- The verdict of a check step: what the engine enforces, never the step's text.
-- Kept per attempt, so a run's history shows how each try went.
ALTER TABLE workflow_steps ADD COLUMN check_result text NOT NULL DEFAULT '' CHECK (check_result IN ('', 'passed', 'failed'));

-- +goose Down
ALTER TABLE workflow_steps DROP COLUMN check_result;

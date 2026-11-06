-- +goose Up
-- Cumulative wall-clock time per run phase, in milliseconds. NULL until the
-- phase has been timed (runs from before this migration never are).
ALTER TABLE runs ADD COLUMN setup_ms bigint, ADD COLUMN harness_ms bigint, ADD COLUMN checks_ms bigint;

-- +goose Down
ALTER TABLE runs DROP COLUMN checks_ms, DROP COLUMN harness_ms, DROP COLUMN setup_ms;

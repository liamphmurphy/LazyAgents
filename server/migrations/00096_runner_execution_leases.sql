-- +goose Up
-- Each claimed setup/turn owns a renewable token. Runner heartbeats alone
-- cannot distinguish a live process from a worker that exited before saving.
ALTER TABLE runs ADD COLUMN execution_token uuid;
ALTER TABLE runs ADD COLUMN execution_expires_at timestamptz;
ALTER TABLE run_turns ADD COLUMN execution_token uuid;
ALTER TABLE run_turns ADD COLUMN execution_expires_at timestamptz;
CREATE INDEX runs_execution_expiry ON runs (execution_expires_at) WHERE state = 'starting';
CREATE INDEX run_turns_execution_expiry ON run_turns (execution_expires_at) WHERE status = 'running';

-- +goose Down
DROP INDEX run_turns_execution_expiry;
DROP INDEX runs_execution_expiry;
ALTER TABLE run_turns DROP COLUMN execution_expires_at, DROP COLUMN execution_token;
ALTER TABLE runs DROP COLUMN execution_expires_at, DROP COLUMN execution_token;

-- +goose Up
-- A delegated run's contract (acceptance criteria, allowed paths, test
-- command, base commit) and the deterministic checks the runner ran on its
-- result before waking the main agent.
ALTER TABLE runs ADD COLUMN contract jsonb, ADD COLUMN checks jsonb;

-- +goose Down
ALTER TABLE runs DROP COLUMN checks, DROP COLUMN contract;

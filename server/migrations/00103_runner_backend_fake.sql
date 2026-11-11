-- +goose Up
-- Runners are Docker only; the unisolated "local" backend is gone and its
-- test-only replacement, "fake", runs nothing but stand-in harnesses. A
-- local runner's row is a stale heartbeat: no current binary can start one.
DELETE FROM runners WHERE backend = 'local';
ALTER TABLE runners
  DROP CONSTRAINT runners_backend_check,
  ADD CONSTRAINT runners_backend_check CHECK (backend IN ('docker', 'fake'));

-- +goose Down
DELETE FROM runners WHERE backend = 'fake';
ALTER TABLE runners
  DROP CONSTRAINT runners_backend_check,
  ADD CONSTRAINT runners_backend_check CHECK (backend IN ('docker', 'local'));

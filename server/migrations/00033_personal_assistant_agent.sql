-- +goose Up
-- The Personal Assistant is its own kind of agent: every chat runs as it, it
-- never holds tickets, and each workspace has at most one (created on demand
-- by the first chat).
ALTER TABLE agents DROP CONSTRAINT agents_kind_check;
ALTER TABLE agents ADD CONSTRAINT agents_kind_check CHECK (kind IN ('main', 'sub', 'assistant'));
CREATE UNIQUE INDEX agents_one_assistant ON agents (workspace_id) WHERE kind = 'assistant';

-- +goose Down
DROP INDEX agents_one_assistant;
UPDATE agents SET kind = 'main' WHERE kind = 'assistant';
ALTER TABLE agents DROP CONSTRAINT agents_kind_check;
ALTER TABLE agents ADD CONSTRAINT agents_kind_check CHECK (kind IN ('main', 'sub'));

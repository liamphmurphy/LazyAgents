-- +goose Up
-- The prompt size, in tokens, of the main agent's latest request. Written
-- while a turn streams (throttled) and at turn end; 0 means unknown (Codex
-- doesn't report it).
ALTER TABLE runs ADD COLUMN context_tokens bigint NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE runs DROP COLUMN context_tokens;

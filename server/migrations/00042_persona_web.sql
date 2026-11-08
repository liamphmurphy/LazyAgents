-- +goose Up
-- Per-persona web access (wiki: Architecture/ADR - Assistant personas). Maps to
-- the web_search harness option in runs.ChatSpec. Existing coding personas keep
-- their current behaviour (no web): a coding chat holds git credentials and a shell.
ALTER TABLE personas ADD COLUMN can_use_web boolean NOT NULL DEFAULT true;
UPDATE personas SET can_use_web = false WHERE can_code;

-- +goose Down
ALTER TABLE personas DROP COLUMN can_use_web;

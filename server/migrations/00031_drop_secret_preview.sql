-- +goose Up
-- Secret values are write-only: the API no longer returns any part of them, so
-- the stored last-4-characters preview goes too (LAZYAGENTS-44).
ALTER TABLE secrets DROP COLUMN preview;

-- +goose Down
ALTER TABLE secrets ADD COLUMN preview text NOT NULL DEFAULT '';

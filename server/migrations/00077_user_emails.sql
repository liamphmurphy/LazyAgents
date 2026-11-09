-- +goose Up
-- A person can have several email addresses. email stays the primary one (and
-- the unique account address); extra_emails holds the rest, in the order the
-- person listed them.
ALTER TABLE users ADD COLUMN extra_emails text[] NOT NULL DEFAULT '{}';

-- +goose Down
ALTER TABLE users DROP COLUMN extra_emails;

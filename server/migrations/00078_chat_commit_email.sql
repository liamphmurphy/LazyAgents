-- +goose Up
-- The address a coding chat's commits are authored as: one of its creator's
-- emails, picked when they have several. Null until picked, when their
-- primary is used.
ALTER TABLE chat_sessions ADD COLUMN commit_email text;

-- +goose Down
ALTER TABLE chat_sessions DROP COLUMN commit_email;

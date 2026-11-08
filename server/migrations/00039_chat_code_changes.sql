-- +goose Up
-- The files a coding chat's turn changed (git diff from the turn's starting
-- commit), stored on the assistant message that ended the turn.
ALTER TABLE chat_messages ADD COLUMN code_changes jsonb NOT NULL DEFAULT '[]';

-- +goose Down
ALTER TABLE chat_messages DROP COLUMN code_changes;

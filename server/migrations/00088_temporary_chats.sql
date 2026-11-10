-- +goose Up
-- A temporary chat runs without the person's memory: its persona neither reads
-- nor writes it, can't search past chats, and the chat itself is left out of
-- search and suggestions and deleted once it has sat idle for a day. Fixed for
-- the chat's life.
ALTER TABLE chat_sessions ADD COLUMN temporary boolean NOT NULL DEFAULT false;
-- The sweep that deletes idle temporary chats.
CREATE INDEX chat_sessions_temporary ON chat_sessions (COALESCE(last_message_at, created_at)) WHERE temporary;

-- +goose Down
DROP INDEX chat_sessions_temporary;
ALTER TABLE chat_sessions DROP COLUMN temporary;

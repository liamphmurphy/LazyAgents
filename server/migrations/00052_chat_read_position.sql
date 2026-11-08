-- +goose Up
-- The last message of a chat its creator has seen. Existing chats start read,
-- so the persona dock doesn't light up with old conversations.
ALTER TABLE chat_sessions ADD COLUMN last_read_seq bigint NOT NULL DEFAULT 0;
UPDATE chat_sessions c SET last_read_seq = coalesce((SELECT max(m.seq) FROM chat_messages m WHERE m.session_id = c.id), 0);

-- +goose Down
ALTER TABLE chat_sessions DROP COLUMN last_read_seq;

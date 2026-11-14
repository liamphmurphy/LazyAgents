-- +goose Up
-- A thread is an aside anchored to one message of its parent chat: a side
-- conversation about that message, started from the message's toolbar at any
-- time, not only while a reply runs. Unanchored asides (about the running
-- reply) leave it null. Deleting the message keeps the thread, unanchored.
ALTER TABLE chat_sessions ADD COLUMN anchor_message_id uuid REFERENCES chat_messages(id) ON DELETE SET NULL;

-- +goose Down
ALTER TABLE chat_sessions DROP COLUMN anchor_message_id;

-- +goose Up
-- A message the person sent while a reply was still running: the reply is
-- stopped, and once its process has exited the next turn answers this message
-- together with what the interrupted reply had done so far. NULL is none.
ALTER TABLE chat_sessions ADD COLUMN interrupt_message_id uuid REFERENCES chat_messages(id) ON DELETE SET NULL;

-- +goose Down
ALTER TABLE chat_sessions DROP COLUMN interrupt_message_id;

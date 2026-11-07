-- +goose Up
-- Deleting a chat used to cascade its messages' attachment rows away, leaving
-- their objects in the bucket with nothing pointing at them. The rows now
-- detach instead (message_id NULL), so the runner's sweep, which deletes
-- unattached uploads older than a day along with their objects, picks them up.
ALTER TABLE chat_attachments
  DROP CONSTRAINT chat_attachments_message_id_fkey,
  ADD CONSTRAINT chat_attachments_message_id_fkey FOREIGN KEY (message_id) REFERENCES chat_messages(id) ON DELETE SET NULL;
CREATE INDEX chat_attachments_unattached ON chat_attachments (created_at) WHERE message_id IS NULL;

-- +goose Down
DROP INDEX chat_attachments_unattached;
ALTER TABLE chat_attachments
  DROP CONSTRAINT chat_attachments_message_id_fkey,
  ADD CONSTRAINT chat_attachments_message_id_fkey FOREIGN KEY (message_id) REFERENCES chat_messages(id) ON DELETE CASCADE;

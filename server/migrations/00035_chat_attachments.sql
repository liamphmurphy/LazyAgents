-- +goose Up
-- Files a person uploads in the assistant conversation. An upload belongs to
-- exactly one person and stays unattached (message_id NULL) until a message
-- they post lists it; it is then delivered to that turn's harness workspace.
-- The bytes live in object storage under blob_key.
CREATE TABLE chat_attachments (
  id           uuid PRIMARY KEY DEFAULT uuid_v7(),
  workspace_id uuid NOT NULL REFERENCES workspaces(id),
  user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  message_id   uuid REFERENCES chat_messages(id) ON DELETE CASCADE,
  filename     text NOT NULL CHECK (length(filename) BETWEEN 1 AND 255),
  content_type text NOT NULL,
  size_bytes   bigint NOT NULL CHECK (size_bytes > 0),
  blob_key     text NOT NULL,
  created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX chat_attachments_owner ON chat_attachments (workspace_id, user_id);
CREATE INDEX chat_attachments_message ON chat_attachments (message_id) WHERE message_id IS NOT NULL;

-- +goose Down
DROP TABLE chat_attachments;

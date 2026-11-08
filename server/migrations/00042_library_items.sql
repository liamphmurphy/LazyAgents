-- +goose Up
-- The Library: a person's permanent, private keepsakes from their assistant
-- chats. Files live in the artifact bucket like chat attachments; an image a
-- persona generated shares its attachment's object, so the Library keeps it
-- after the chat (and its attachment row) is deleted.
CREATE TABLE library_items (
  id           uuid PRIMARY KEY DEFAULT uuid_v7(),
  workspace_id uuid NOT NULL REFERENCES workspaces(id),
  user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  kind         text NOT NULL DEFAULT 'image' CHECK (kind IN ('image')),
  title        text NOT NULL CHECK (length(title) BETWEEN 1 AND 300),
  filename     text NOT NULL CHECK (length(filename) BETWEEN 1 AND 255),
  content_type text NOT NULL,
  size_bytes   bigint NOT NULL CHECK (size_bytes > 0),
  blob_key     text NOT NULL,
  -- Where it came from; each link outlives its source as NULL.
  chat_id      uuid REFERENCES chat_sessions(id) ON DELETE SET NULL,
  message_id   uuid REFERENCES chat_messages(id) ON DELETE SET NULL,
  persona_id   uuid REFERENCES personas(id) ON DELETE SET NULL,
  created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX library_items_owner ON library_items (workspace_id, user_id, created_at DESC, id DESC);
CREATE INDEX library_items_blob ON library_items (blob_key);
-- Deleting either side checks whether the other still uses the object.
CREATE INDEX chat_attachments_blob ON chat_attachments (blob_key);

-- +goose Down
DROP INDEX chat_attachments_blob;
DROP TABLE library_items;

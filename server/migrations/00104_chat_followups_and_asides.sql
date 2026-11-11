-- +goose Up
-- Messages the person queued while a reply ran: the runner sends the oldest
-- once a turn ends normally (not when it was stopped or failed), one per turn.
CREATE TABLE chat_followups (
  id uuid PRIMARY KEY DEFAULT uuid_v7(),
  session_id uuid NOT NULL REFERENCES chat_sessions(id) ON DELETE CASCADE,
  body text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX chat_followups_session_idx ON chat_followups (session_id, created_at, id);

-- An aside is a hidden chat that answers one question about its parent's
-- running turn without interrupting it.
ALTER TABLE chat_sessions ADD COLUMN parent_chat_id uuid REFERENCES chat_sessions(id) ON DELETE CASCADE;
CREATE INDEX chat_sessions_parent_idx ON chat_sessions (parent_chat_id, created_at) WHERE parent_chat_id IS NOT NULL;

-- +goose Down
DROP INDEX chat_sessions_parent_idx;
ALTER TABLE chat_sessions DROP COLUMN parent_chat_id;
DROP TABLE chat_followups;

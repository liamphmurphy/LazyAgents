-- +goose Up
-- Whether a persona's ticket edits wait for the person: 'ask' records each
-- one as a ticket change request the person applies or declines in the chat;
-- 'allow' applies them at once. A cancel always asks.
ALTER TABLE personas ADD COLUMN ticket_edits text NOT NULL DEFAULT 'ask'
  CHECK (ticket_edits IN ('ask', 'allow'));

-- A ticket edit a persona proposed from a chat. arguments is the tool input,
-- applied as the persona once the person approves; changes is the
-- before/after preview the card shows, taken when it was proposed.
CREATE TABLE ticket_change_requests (
  id           uuid PRIMARY KEY DEFAULT uuid_v7(),
  workspace_id uuid NOT NULL REFERENCES workspaces(id),
  user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  chat_id      uuid NOT NULL REFERENCES chat_sessions(id) ON DELETE CASCADE,
  persona_id   uuid REFERENCES personas(id) ON DELETE SET NULL,
  agent_id     uuid NOT NULL REFERENCES agents(id),
  ticket_id    uuid NOT NULL REFERENCES tickets(id) ON DELETE CASCADE,
  tool         text NOT NULL CHECK (tool IN ('update_ticket', 'comment_ticket', 'update_ticket_blockers')),
  arguments    jsonb NOT NULL,
  changes      jsonb NOT NULL DEFAULT '[]',
  state        text NOT NULL DEFAULT 'pending' CHECK (state IN ('pending', 'applied', 'declined', 'failed')),
  error        text,
  created_at   timestamptz NOT NULL DEFAULT now(),
  decided_at   timestamptz
);
CREATE INDEX ticket_change_requests_chat ON ticket_change_requests (chat_id, created_at) WHERE state = 'pending';

-- +goose Down
DROP TABLE ticket_change_requests;
ALTER TABLE personas DROP COLUMN ticket_edits;

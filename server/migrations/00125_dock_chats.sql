-- +goose Up
-- The chats an app dock keeps for one place: a Knowledge page ('page:<id>')
-- or an app's screens that aren't an item ('app:knowledge', 'app:finance').
-- A dock lists them in its chat switcher, newest first. The page's own
-- chat_session_id stays the one it opens on.
CREATE TABLE dock_chats (
  workspace_id uuid NOT NULL REFERENCES workspaces(id),
  user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  scope        text NOT NULL CHECK (scope ~ '^(page|app):[A-Za-z0-9_-]+$'),
  chat_id      uuid NOT NULL REFERENCES chat_sessions(id) ON DELETE CASCADE,
  attached_at  timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, scope, chat_id)
);
CREATE INDEX dock_chats_chat ON dock_chats (chat_id);

-- Every page's current chat is its first dock chat.
INSERT INTO dock_chats (workspace_id, user_id, scope, chat_id)
SELECT a.workspace_id, c.created_by, 'page:' || a.id, c.id
FROM artifacts a JOIN chat_sessions c ON c.id = a.chat_session_id
ON CONFLICT DO NOTHING;

-- +goose Down
DROP TABLE dock_chats;

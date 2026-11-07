-- +goose Up
-- Assistant personas (wiki: Architecture/ADR - Assistant personas). A persona
-- is a named voice for the Personal Assistant, private to one person like
-- memories: every query filters on (workspace_id, user_id). A chat keeps the
-- persona it was created with; a NULL persona is the built-in Assistant.
CREATE TABLE personas (
  id                  uuid PRIMARY KEY DEFAULT uuid_v7(),
  workspace_id        uuid NOT NULL REFERENCES workspaces(id),
  user_id             uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name                text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 60),
  tone                text NOT NULL DEFAULT 'neutral' CHECK (tone IN ('neutral', 'friendly', 'concise', 'formal', 'playful')),
  color               text NOT NULL DEFAULT '#64748b' CHECK (color ~ '^#[0-9a-f]{6}$'),
  avatar_key          text,
  avatar_content_type text,
  custom_instructions text NOT NULL DEFAULT '' CHECK (length(custom_instructions) <= 2000),
  is_default          boolean NOT NULL DEFAULT false,
  last_used_at        timestamptz,
  revision            integer NOT NULL DEFAULT 1,
  created_at          timestamptz NOT NULL DEFAULT now(),
  updated_at          timestamptz NOT NULL DEFAULT now(),
  CHECK ((avatar_key IS NULL) = (avatar_content_type IS NULL))
);
-- At most one default per person; a swap clears the old one first, in one transaction.
CREATE UNIQUE INDEX personas_one_default ON personas (workspace_id, user_id) WHERE is_default;
CREATE UNIQUE INDEX personas_name ON personas (workspace_id, user_id, lower(btrim(name)));

ALTER TABLE chat_sessions ADD COLUMN persona_id uuid REFERENCES personas(id) ON DELETE SET NULL;
-- Serves the grouped chat list: one creator and persona, newest activity first.
CREATE INDEX chat_sessions_persona ON chat_sessions (created_by, persona_id, (COALESCE(last_message_at, created_at)) DESC, id DESC) WHERE NOT archived;

-- Every existing person starts with one default "Assistant" persona, and
-- their existing chats move to it.
INSERT INTO personas (workspace_id, user_id, name, is_default)
SELECT u.workspace_id, u.id, 'Assistant', true FROM users u;

UPDATE chat_sessions c SET persona_id = p.id
FROM personas p
WHERE p.user_id = c.created_by AND p.workspace_id = c.workspace_id AND p.is_default;

-- persona.updated carries the persona ids, like chat.updated (notify_chat_change
-- is generic: TG_ARGV[0] is the event type, TG_ARGV[1] the id column).
CREATE TRIGGER realtime_change_ins AFTER INSERT ON personas REFERENCING NEW TABLE AS new_rows FOR EACH STATEMENT EXECUTE FUNCTION notify_chat_change('persona.updated', 'id');
CREATE TRIGGER realtime_change_upd AFTER UPDATE ON personas REFERENCING NEW TABLE AS new_rows FOR EACH STATEMENT EXECUTE FUNCTION notify_chat_change('persona.updated', 'id');
CREATE TRIGGER realtime_change_del AFTER DELETE ON personas REFERENCING OLD TABLE AS old_rows FOR EACH STATEMENT EXECUTE FUNCTION notify_chat_change('persona.updated', 'id');

-- +goose Down
DROP INDEX chat_sessions_persona;
ALTER TABLE chat_sessions DROP COLUMN persona_id;
DROP TABLE personas;

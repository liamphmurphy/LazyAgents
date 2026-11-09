-- +goose Up
-- Spaces group a person's chats around a topic. A space's instructions and
-- sources reach every chat in it, and with space memory on, what the assistant
-- learns there stays there. Spaces are strictly per person.
CREATE TABLE chat_spaces (
  id            uuid PRIMARY KEY DEFAULT uuid_v7(),
  workspace_id  uuid NOT NULL REFERENCES workspaces(id),
  user_id       uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name          text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 80),
  description   text NOT NULL DEFAULT '' CHECK (length(description) <= 300),
  color         text NOT NULL DEFAULT 'green' CHECK (color IN ('green', 'amber', 'red', 'blue', 'violet', 'pink', 'teal', 'orange')),
  instructions  text NOT NULL DEFAULT '' CHECK (length(instructions) <= 4000),
  -- Defaults for new chats. NULL persona: the person's default persona.
  persona_id    uuid REFERENCES personas(id) ON DELETE SET NULL,
  project_id    uuid REFERENCES projects(id) ON DELETE SET NULL,
  -- The connectors a chat here may use; empty is none.
  connector_ids uuid[] NOT NULL DEFAULT '{}',
  space_memory  boolean NOT NULL DEFAULT true,
  created_at    timestamptz NOT NULL DEFAULT now(),
  updated_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX chat_spaces_owner ON chat_spaces (workspace_id, user_id, created_at);

-- A source is a snapshot taken when it was added: text in body, or a file's
-- bytes in the artifact bucket under blob_key.
CREATE TABLE chat_space_sources (
  id           uuid PRIMARY KEY DEFAULT uuid_v7(),
  space_id     uuid NOT NULL REFERENCES chat_spaces(id) ON DELETE CASCADE,
  kind         text NOT NULL CHECK (kind IN ('file', 'text', 'website', 'library', 'link', 'canvas')),
  title        text NOT NULL CHECK (length(btrim(title)) BETWEEN 1 AND 300),
  url          text,
  body         text NOT NULL DEFAULT '',
  filename     text,
  content_type text NOT NULL DEFAULT 'text/markdown',
  size_bytes   bigint NOT NULL DEFAULT 0 CHECK (size_bytes >= 0),
  blob_key     text,
  -- Estimated from the text; 0 for a file that isn't text.
  tokens       integer NOT NULL DEFAULT 0 CHECK (tokens >= 0),
  created_at   timestamptz NOT NULL DEFAULT now(),
  CHECK (blob_key IS NOT NULL OR body <> '')
);
CREATE INDEX chat_space_sources_space ON chat_space_sources (space_id, created_at);

ALTER TABLE chat_sessions ADD COLUMN space_id uuid REFERENCES chat_spaces(id) ON DELETE SET NULL;
CREATE INDEX chat_sessions_space ON chat_sessions (space_id, last_message_at) WHERE space_id IS NOT NULL;

-- A space memory belongs to the space: only chats in it see it, and it goes
-- with the space.
-- Only what the assistant remembers about the person can be a space's, never
-- a persona's own memory.
ALTER TABLE memories ADD COLUMN space_id uuid REFERENCES chat_spaces(id) ON DELETE CASCADE,
  ADD CONSTRAINT memories_space_person CHECK (space_id IS NULL OR persona_id IS NULL);
CREATE INDEX memories_space ON memories (space_id) WHERE space_id IS NOT NULL;

-- +goose Down
DROP INDEX memories_space;
ALTER TABLE memories DROP CONSTRAINT memories_space_person, DROP COLUMN space_id;
DROP INDEX chat_sessions_space;
ALTER TABLE chat_sessions DROP COLUMN space_id;
DROP TABLE chat_space_sources;
DROP TABLE chat_spaces;

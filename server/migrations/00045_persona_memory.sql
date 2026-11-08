-- +goose Up
-- A persona's own memory: who it is and the moments it shared with its
-- person, kept apart from what it remembers about the person. persona_id NULL
-- is the person's memory, as before; set, the entry is that persona's (still
-- owned by the same workspace and user). Moments are a persona's only.
ALTER TABLE memories ADD COLUMN persona_id uuid REFERENCES personas(id) ON DELETE CASCADE;
ALTER TABLE memories DROP CONSTRAINT memories_type_check;
ALTER TABLE memories ADD CONSTRAINT memories_type_check
  CHECK (type IN ('preference', 'project', 'person', 'life', 'work', 'fact', 'other', 'moment'));
ALTER TABLE memories ADD CONSTRAINT memories_moment_persona CHECK (type <> 'moment' OR persona_id IS NOT NULL);
CREATE INDEX memories_persona ON memories (persona_id, updated_at DESC, id DESC) WHERE persona_id IS NOT NULL;

-- +goose Down
-- Persona entries would otherwise turn into the person's memory.
DELETE FROM memories WHERE persona_id IS NOT NULL;
DROP INDEX memories_persona;
ALTER TABLE memories DROP CONSTRAINT memories_moment_persona;
ALTER TABLE memories DROP CONSTRAINT memories_type_check;
ALTER TABLE memories ADD CONSTRAINT memories_type_check
  CHECK (type IN ('preference', 'project', 'person', 'life', 'work', 'fact', 'other'));
ALTER TABLE memories DROP COLUMN persona_id;

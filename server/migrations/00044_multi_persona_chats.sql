-- +goose Up
-- Several of a person's personas in one chat. The chat's persona_id stays its
-- host (repository, work branch, the default voice); others join when someone
-- mentions them by @name. Each turn is answered by one speaker.

-- Whether a persona may propose new personas in a chat; each proposal waits
-- for the person's approval before anything is created.
ALTER TABLE personas ADD COLUMN can_create_personas boolean NOT NULL DEFAULT false;

-- The persona answering the current (or last) turn; NULL is the host.
ALTER TABLE chat_sessions ADD COLUMN speaker_persona_id uuid REFERENCES personas(id) ON DELETE SET NULL;
-- Personas still due to answer, in order, after the speaker: further mentions
-- in a message, then the ones a reply hands over to. Deleted ids are skipped.
ALTER TABLE chat_sessions ADD COLUMN queued_persona_ids uuid[] NOT NULL DEFAULT '{}';
-- Replies personas have handed to each other since the person last wrote;
-- bounded so two personas can't keep mentioning each other forever.
ALTER TABLE chat_sessions ADD COLUMN relay_turns integer NOT NULL DEFAULT 0;

-- Which persona wrote an assistant message; NULL is the chat's host (and every
-- message from before personas could share a chat).
ALTER TABLE chat_messages ADD COLUMN persona_id uuid REFERENCES personas(id) ON DELETE SET NULL;

-- Personas brought into a chat besides its host.
CREATE TABLE chat_participants (
  chat_id    uuid NOT NULL REFERENCES chat_sessions(id) ON DELETE CASCADE,
  persona_id uuid NOT NULL REFERENCES personas(id) ON DELETE CASCADE,
  joined_at  timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (chat_id, persona_id)
);
CREATE INDEX chat_participants_persona ON chat_participants (persona_id);

-- A persona's request to create another persona, decided by the person.
CREATE TABLE persona_proposals (
  id                  uuid PRIMARY KEY DEFAULT uuid_v7(),
  chat_id             uuid NOT NULL REFERENCES chat_sessions(id) ON DELETE CASCADE,
  run_id              uuid REFERENCES runs(id) ON DELETE SET NULL,
  proposed_by         uuid REFERENCES personas(id) ON DELETE SET NULL,
  name                text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 60),
  tone                text NOT NULL CHECK (tone IN ('neutral', 'friendly', 'concise', 'formal', 'playful')),
  color               text NOT NULL CHECK (color ~ '^#[0-9a-f]{6}$'),
  custom_instructions text NOT NULL DEFAULT '' CHECK (length(custom_instructions) <= 2000),
  reason              text NOT NULL DEFAULT '' CHECK (length(reason) <= 500),
  state               text NOT NULL DEFAULT 'pending' CHECK (state IN ('pending', 'approved', 'declined')),
  -- The persona an approval created, while it exists.
  persona_id          uuid REFERENCES personas(id) ON DELETE SET NULL,
  created_at          timestamptz NOT NULL DEFAULT now(),
  decided_at          timestamptz
);
CREATE INDEX persona_proposals_chat ON persona_proposals (chat_id, created_at);

-- +goose Down
DROP TABLE persona_proposals;
DROP TABLE chat_participants;
ALTER TABLE chat_messages DROP COLUMN persona_id;
ALTER TABLE chat_sessions DROP COLUMN relay_turns;
ALTER TABLE chat_sessions DROP COLUMN queued_persona_ids;
ALTER TABLE chat_sessions DROP COLUMN speaker_persona_id;
ALTER TABLE personas DROP COLUMN can_create_personas;

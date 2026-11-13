-- +goose Up
ALTER TABLE artifacts ADD COLUMN persona_direct boolean NOT NULL DEFAULT false,
  ADD COLUMN persona_paused boolean NOT NULL DEFAULT false,
  ADD COLUMN chat_session_id uuid REFERENCES chat_sessions(id) ON DELETE SET NULL;
UPDATE artifacts SET persona_direct = true WHERE audience = 'private';
ALTER TABLE artifact_versions ADD COLUMN changes jsonb NOT NULL DEFAULT '[]';

-- +goose Down
ALTER TABLE artifact_versions DROP COLUMN changes;
ALTER TABLE artifacts DROP COLUMN persona_direct, DROP COLUMN persona_paused, DROP COLUMN chat_session_id;

-- +goose Up
ALTER TABLE artifacts
  ADD COLUMN persona_direct boolean NOT NULL DEFAULT false,
  ADD COLUMN persona_paused boolean NOT NULL DEFAULT false,
  ADD COLUMN chat_session_id uuid REFERENCES chat_sessions(id) ON DELETE SET NULL;
ALTER TABLE artifact_versions
  ADD COLUMN changes jsonb CHECK (changes IS NULL OR jsonb_typeof(changes) = 'array'),
  ADD COLUMN persona_id uuid REFERENCES personas(id) ON DELETE SET NULL,
  ADD COLUMN source_chat_id uuid REFERENCES chat_sessions(id) ON DELETE SET NULL,
  ADD COLUMN source_run_id uuid REFERENCES runs(id) ON DELETE SET NULL;

-- +goose Down
ALTER TABLE artifact_versions DROP COLUMN changes, DROP COLUMN persona_id, DROP COLUMN source_chat_id, DROP COLUMN source_run_id;
ALTER TABLE artifacts DROP COLUMN persona_direct, DROP COLUMN persona_paused, DROP COLUMN chat_session_id;

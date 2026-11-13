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
-- Indexes keep ON DELETE SET NULL from scanning every version.
CREATE INDEX artifacts_chat_session_idx ON artifacts (chat_session_id) WHERE chat_session_id IS NOT NULL;
CREATE INDEX artifact_versions_persona_idx ON artifact_versions (persona_id) WHERE persona_id IS NOT NULL;
CREATE INDEX artifact_versions_source_chat_idx ON artifact_versions (source_chat_id) WHERE source_chat_id IS NOT NULL;
CREATE INDEX artifact_versions_source_run_idx ON artifact_versions (source_run_id) WHERE source_run_id IS NOT NULL;

-- +goose Down
ALTER TABLE artifact_versions DROP COLUMN changes, DROP COLUMN persona_id, DROP COLUMN source_chat_id, DROP COLUMN source_run_id;
ALTER TABLE artifacts DROP COLUMN persona_direct, DROP COLUMN persona_paused, DROP COLUMN chat_session_id;

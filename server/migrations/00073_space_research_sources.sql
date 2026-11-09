-- +goose Up
-- A deep research report written in a space's chat becomes one of the space's
-- sources, so every chat there can use its findings. The source is a snapshot
-- of the report; it remembers the job (one source per report) and the chat it
-- came from.
ALTER TABLE chat_space_sources DROP CONSTRAINT chat_space_sources_kind_check;
ALTER TABLE chat_space_sources ADD CONSTRAINT chat_space_sources_kind_check
  CHECK (kind IN ('file', 'text', 'website', 'library', 'link', 'canvas', 'research'));
ALTER TABLE chat_space_sources
  ADD COLUMN research_job_id uuid UNIQUE REFERENCES research_jobs(id) ON DELETE SET NULL,
  ADD COLUMN chat_id uuid REFERENCES chat_sessions(id) ON DELETE SET NULL;

-- +goose Down
DELETE FROM chat_space_sources WHERE kind = 'research';
ALTER TABLE chat_space_sources DROP COLUMN chat_id, DROP COLUMN research_job_id;
ALTER TABLE chat_space_sources DROP CONSTRAINT chat_space_sources_kind_check;
ALTER TABLE chat_space_sources ADD CONSTRAINT chat_space_sources_kind_check
  CHECK (kind IN ('file', 'text', 'website', 'library', 'link', 'canvas'));

-- +goose Up
-- Fuzzy find_pattern: trigram similarity (typos, partial words) over note
-- titles and paths.
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE INDEX artifacts_note_title_trgm ON artifacts USING gin (lower(title) gin_trgm_ops) WHERE type = 'note';
CREATE INDEX artifacts_note_path_trgm ON artifacts USING gin (lower(path) gin_trgm_ops) WHERE type = 'note';

-- +goose Down
-- The extension stays: dropping it could break unrelated objects.
DROP INDEX artifacts_note_path_trgm;
DROP INDEX artifacts_note_title_trgm;

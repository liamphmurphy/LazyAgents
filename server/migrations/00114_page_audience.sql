-- +goose Up
ALTER TABLE artifacts
  ADD COLUMN audience text NOT NULL DEFAULT 'shared' CHECK (audience IN ('private', 'shared', 'agent')),
  ADD COLUMN owner_user_id uuid REFERENCES users(id) ON DELETE CASCADE,
  ADD CONSTRAINT artifacts_private_owner CHECK (audience <> 'private' OR owner_user_id IS NOT NULL);
UPDATE artifacts SET audience = 'agent'
WHERE type = 'note' AND (lower(path) LIKE 'agents/%' OR lower(path) LIKE 'patterns/proposed/%');
ALTER TABLE artifact_aliases DROP CONSTRAINT artifact_aliases_pkey;
ALTER TABLE artifact_aliases ADD PRIMARY KEY (artifact_id, path);
DROP INDEX artifacts_note_path_idx;
CREATE UNIQUE INDEX artifacts_note_path_idx ON artifacts(workspace_id, lower(path))
  WHERE path IS NOT NULL AND audience <> 'private';
CREATE UNIQUE INDEX artifacts_private_path_idx ON artifacts(workspace_id, owner_user_id, lower(path))
  WHERE path IS NOT NULL AND audience = 'private';
-- +goose StatementBegin
CREATE FUNCTION artifact_visible(audience text, owner uuid, viewer uuid) RETURNS boolean
LANGUAGE sql IMMUTABLE AS $$ SELECT audience <> 'private' OR (viewer IS NOT NULL AND owner = viewer) $$;
-- +goose StatementEnd

-- +goose Down
ALTER TABLE artifact_aliases DROP CONSTRAINT artifact_aliases_pkey;
ALTER TABLE artifact_aliases ADD PRIMARY KEY (workspace_id, path);
-- Refuse rollback with overlapping paths rather than silently losing a page.
DROP INDEX artifacts_private_path_idx;
DROP INDEX artifacts_note_path_idx;
CREATE UNIQUE INDEX artifacts_note_path_idx ON artifacts(workspace_id, lower(path)) WHERE path IS NOT NULL;
DROP FUNCTION artifact_visible(text, uuid, uuid);
ALTER TABLE artifacts DROP CONSTRAINT artifacts_private_owner, DROP COLUMN owner_user_id, DROP COLUMN audience;

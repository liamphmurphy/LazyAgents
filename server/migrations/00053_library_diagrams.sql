-- +goose Up
-- Diagrams: SVG images a person chose to keep from a chat. Their object key is
-- derived from the content, so saving the same diagram again finds its row.
ALTER TABLE library_items DROP CONSTRAINT library_items_kind_check;
ALTER TABLE library_items ADD CONSTRAINT library_items_kind_check CHECK (kind IN ('image', 'diagram'));
CREATE UNIQUE INDEX library_items_diagram ON library_items (workspace_id, user_id, blob_key) WHERE kind = 'diagram';

-- +goose Down
DROP INDEX library_items_diagram;
DELETE FROM library_items WHERE kind = 'diagram';
ALTER TABLE library_items DROP CONSTRAINT library_items_kind_check;
ALTER TABLE library_items ADD CONSTRAINT library_items_kind_check CHECK (kind IN ('image'));

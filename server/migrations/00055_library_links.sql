-- +goose Up
-- Links: pages and videos a person chose to keep from a persona's
-- recommendation. A link may have no image, so it may have no stored bytes.
ALTER TABLE library_items DROP CONSTRAINT library_items_kind_check;
ALTER TABLE library_items ADD CONSTRAINT library_items_kind_check CHECK (kind IN ('image', 'diagram', 'link'));
ALTER TABLE library_items DROP CONSTRAINT library_items_size_bytes_check;
ALTER TABLE library_items ADD CONSTRAINT library_items_size_bytes_check CHECK (size_bytes > 0 OR kind = 'link');
ALTER TABLE library_items
  ADD COLUMN url      text NOT NULL DEFAULT '',
  ADD COLUMN note     text NOT NULL DEFAULT '',
  ADD COLUMN site     text NOT NULL DEFAULT '',
  ADD COLUMN video_id text NOT NULL DEFAULT '';
CREATE UNIQUE INDEX library_items_link ON library_items (workspace_id, user_id, url) WHERE kind = 'link';

-- +goose Down
DROP INDEX library_items_link;
DELETE FROM library_items WHERE kind = 'link';
ALTER TABLE library_items DROP COLUMN url, DROP COLUMN note, DROP COLUMN site, DROP COLUMN video_id;
ALTER TABLE library_items DROP CONSTRAINT library_items_size_bytes_check;
ALTER TABLE library_items ADD CONSTRAINT library_items_size_bytes_check CHECK (size_bytes > 0);
ALTER TABLE library_items DROP CONSTRAINT library_items_kind_check;
ALTER TABLE library_items ADD CONSTRAINT library_items_kind_check CHECK (kind IN ('image', 'diagram'));

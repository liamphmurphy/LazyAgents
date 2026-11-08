-- +goose Up
-- Saved links double as a reading queue, and can keep a summary of the page.
ALTER TABLE library_items
  ADD COLUMN link_status text NOT NULL DEFAULT 'to_read' CHECK (link_status IN ('to_read', 'done')),
  ADD COLUMN summary     text NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE library_items DROP COLUMN link_status, DROP COLUMN summary;

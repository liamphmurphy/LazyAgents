-- +goose Up
-- Two more Library kinds. A `file` is anything that isn't media: code, a zip,
-- a spreadsheet, a PDF someone uploaded or a persona handed over (save_file).
-- An `export` is a frozen copy of what Slides, Designs or Docs produced, with
-- a link back to its source so the Library can say whether the source has
-- changed since. Exports used to be saved as plain image, diagram or document
-- items; those stay as they are, new saves are exports.
ALTER TABLE library_items DROP CONSTRAINT library_items_kind_check;
ALTER TABLE library_items ADD CONSTRAINT library_items_kind_check CHECK (kind IN ('image', 'diagram', 'link', 'document', 'video', 'audio', 'file', 'export'));
-- One line the tile shows under the name: "18 files", "112 lines", "1,204 rows · 6 columns".
ALTER TABLE library_items ADD COLUMN detail text NOT NULL DEFAULT '' CHECK (length(detail) <= 200);
-- An export's source canvas and when it was last edited at export time. The
-- canvas may be deleted later, so there's no foreign key: a missing source is a state.
ALTER TABLE library_items ADD COLUMN source_id uuid;
ALTER TABLE library_items ADD COLUMN source_stamp timestamptz;
ALTER TABLE library_items ADD CONSTRAINT library_items_export_source CHECK (kind <> 'export' OR (source_id IS NOT NULL AND source_stamp IS NOT NULL));
CREATE INDEX library_items_source ON library_items (source_id) WHERE source_id IS NOT NULL;

-- +goose Down
DELETE FROM library_items WHERE kind IN ('file', 'export');
DROP INDEX library_items_source;
ALTER TABLE library_items DROP CONSTRAINT library_items_export_source;
ALTER TABLE library_items DROP COLUMN source_stamp, DROP COLUMN source_id, DROP COLUMN detail;
ALTER TABLE library_items DROP CONSTRAINT library_items_kind_check;
ALTER TABLE library_items ADD CONSTRAINT library_items_kind_check CHECK (kind IN ('image', 'diagram', 'link', 'document', 'video', 'audio'));

-- +goose Up
-- Audio joins the Library next to images, diagrams, links, documents and
-- videos. peaks is the waveform the tiles draw: one byte (0-255) per bucket,
-- computed when the file is stored so the browser never decodes audio to
-- show a tile. A format the server can't decode keeps NULL peaks and the
-- tile draws a flat waveform.
ALTER TABLE library_items DROP CONSTRAINT library_items_kind_check;
ALTER TABLE library_items ADD CONSTRAINT library_items_kind_check CHECK (kind IN ('image', 'diagram', 'link', 'document', 'video', 'audio'));
ALTER TABLE library_items ADD COLUMN peaks bytea;
-- How a generated audio file was made, in the persona's words (e.g. MIDI →
-- FluidSynth); empty for an upload.
ALTER TABLE library_items ADD COLUMN method text NOT NULL DEFAULT '' CHECK (length(method) <= 200);

-- +goose Down
DELETE FROM library_items WHERE kind = 'audio';
ALTER TABLE library_items DROP COLUMN peaks, DROP COLUMN method;
ALTER TABLE library_items DROP CONSTRAINT library_items_kind_check;
ALTER TABLE library_items ADD CONSTRAINT library_items_kind_check CHECK (kind IN ('image', 'diagram', 'link', 'document', 'video'));

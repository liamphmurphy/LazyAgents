-- +goose Up
-- The last version the person has reviewed. The Docs app shows a persona's
-- versions after it (and after the person's own last edit) as changes to
-- accept or reject; accepting them all writes nothing, so this records it.
ALTER TABLE chat_canvases ADD COLUMN reviewed_seq integer NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE chat_canvases DROP COLUMN reviewed_seq;

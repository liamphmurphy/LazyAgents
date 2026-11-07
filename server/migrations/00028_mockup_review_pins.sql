-- +goose Up
-- Comments a reviewer pinned on a mockup version, saved with its review:
-- [{"x": 0.42, "y": 0.18, "text": "…"}], x and y as fractions of the drawing's
-- width and height. Their text is also gathered into review_note as a numbered
-- list, so agents read them without knowing about this column.
ALTER TABLE artifact_versions ADD COLUMN review_pins jsonb NOT NULL DEFAULT '[]';

-- +goose Down
ALTER TABLE artifact_versions DROP COLUMN review_pins;

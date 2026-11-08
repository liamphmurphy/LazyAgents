-- +goose Up
-- Whether a persona's replies may show SVG images in chat.
ALTER TABLE personas ADD COLUMN can_create_svgs boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE personas DROP COLUMN can_create_svgs;

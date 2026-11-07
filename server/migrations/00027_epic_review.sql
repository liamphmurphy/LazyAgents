-- +goose Up
-- The server creates one "final review" ticket per epic when its last regular
-- ticket finishes. Epic tickets only build; the review runs the tests once.
ALTER TABLE tickets ADD COLUMN epic_review boolean NOT NULL DEFAULT false;
CREATE UNIQUE INDEX tickets_epic_review_idx ON tickets (epic_id) WHERE epic_review;

-- +goose Down
DROP INDEX tickets_epic_review_idx;
ALTER TABLE tickets DROP COLUMN epic_review;

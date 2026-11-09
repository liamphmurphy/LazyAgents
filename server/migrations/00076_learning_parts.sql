-- +goose Up
-- A long module is built in parts: the outline plans them all, and each part is
-- outlined, approved, researched, written and reviewed in turn. part is the one
-- being built, or the last one built once the module is ready.
ALTER TABLE learning_courses ADD COLUMN part integer NOT NULL DEFAULT 0;
-- Which part a run worked on, so a part's spend limit counts only its own runs.
ALTER TABLE learning_runs ADD COLUMN part integer NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE learning_runs DROP COLUMN part;
ALTER TABLE learning_courses DROP COLUMN part;

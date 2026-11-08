-- +goose Up
-- Whether a persona may propose workflows and workflow edits.
ALTER TABLE personas ADD COLUMN can_create_workflows boolean NOT NULL DEFAULT false;
-- A chat that works behind another screen, such as the workflow editor's build
-- chat: reachable by id, but left out of chat lists, search and the dock.
ALTER TABLE chat_sessions ADD COLUMN hidden boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE chat_sessions DROP COLUMN hidden;
ALTER TABLE personas DROP COLUMN can_create_workflows;

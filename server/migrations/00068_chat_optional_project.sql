-- +goose Up
-- Chats live above projects: a project is something a chat may attach, not
-- something it belongs to. Existing chats keep theirs as the attached project.
ALTER TABLE chat_sessions ALTER COLUMN project_id DROP NOT NULL;
ALTER TABLE workflow_runs ALTER COLUMN project_id DROP NOT NULL;
-- A research report from a chat with no project stays project-less.
ALTER TABLE artifacts DROP CONSTRAINT artifacts_project_required;
ALTER TABLE artifacts ADD CONSTRAINT artifacts_project_required CHECK (type IN ('note', 'report') OR project_id IS NOT NULL);

-- +goose Down
-- Project-less rows have no project to return to, so they go.
DELETE FROM artifacts WHERE type = 'report' AND project_id IS NULL;
ALTER TABLE artifacts DROP CONSTRAINT artifacts_project_required;
ALTER TABLE artifacts ADD CONSTRAINT artifacts_project_required CHECK (type = 'note' OR project_id IS NOT NULL);
DELETE FROM workflow_runs WHERE project_id IS NULL;
DELETE FROM chat_sessions WHERE project_id IS NULL;
ALTER TABLE workflow_runs ALTER COLUMN project_id SET NOT NULL;
ALTER TABLE chat_sessions ALTER COLUMN project_id SET NOT NULL;

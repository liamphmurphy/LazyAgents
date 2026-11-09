-- +goose Up
-- Chats live above projects: a project is something a chat may attach, not
-- something it belongs to. Existing chats keep theirs as the attached project.
ALTER TABLE chat_sessions ALTER COLUMN project_id DROP NOT NULL;
ALTER TABLE workflow_runs ALTER COLUMN project_id DROP NOT NULL;
-- A research report from a chat with no project stays project-less.
ALTER TABLE artifacts DROP CONSTRAINT artifacts_project_required;
ALTER TABLE artifacts ADD CONSTRAINT artifacts_project_required CHECK (type IN ('note', 'report') OR project_id IS NOT NULL);

-- +goose Down
-- The old schema needs a project on every row, so project-less rows move to
-- their workspace's oldest project, preferring an active one. Only a workspace
-- with no projects at all has nowhere to put them, and it loses them.
UPDATE chat_sessions c SET project_id = (SELECT p.id FROM projects p WHERE p.workspace_id = c.workspace_id ORDER BY p.archived, p.created_at, p.id LIMIT 1) WHERE c.project_id IS NULL;
UPDATE workflow_runs r SET project_id = (SELECT p.id FROM projects p WHERE p.workspace_id = r.workspace_id ORDER BY p.archived, p.created_at, p.id LIMIT 1) WHERE r.project_id IS NULL;
UPDATE artifacts a SET project_id = (SELECT p.id FROM projects p WHERE p.workspace_id = a.workspace_id ORDER BY p.archived, p.created_at, p.id LIMIT 1) WHERE a.type = 'report' AND a.project_id IS NULL;
DELETE FROM artifacts WHERE type = 'report' AND project_id IS NULL;
DELETE FROM workflow_runs WHERE project_id IS NULL;
DELETE FROM chat_sessions WHERE project_id IS NULL;
ALTER TABLE artifacts DROP CONSTRAINT artifacts_project_required;
ALTER TABLE artifacts ADD CONSTRAINT artifacts_project_required CHECK (type = 'note' OR project_id IS NOT NULL);
ALTER TABLE workflow_runs ALTER COLUMN project_id SET NOT NULL;
ALTER TABLE chat_sessions ALTER COLUMN project_id SET NOT NULL;

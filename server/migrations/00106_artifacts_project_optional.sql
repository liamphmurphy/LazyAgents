-- +goose Up
-- A chat is above projects, so what it saves from there (a canvas as a design
-- or mockup, not just a report) needs no project either. PRDs, specs and ADRs
-- remain project documents.
ALTER TABLE artifacts DROP CONSTRAINT artifacts_project_required;
ALTER TABLE artifacts ADD CONSTRAINT artifacts_project_required CHECK (type IN ('note', 'report', 'mockup', 'design') OR project_id IS NOT NULL);

-- +goose Down
-- Project-less mockups and designs move to their workspace's oldest project,
-- preferring an active one. Only a workspace with no projects has nowhere to
-- put them, and it loses them.
UPDATE artifacts a SET project_id = (SELECT p.id FROM projects p WHERE p.workspace_id = a.workspace_id ORDER BY p.archived, p.created_at, p.id LIMIT 1) WHERE a.type IN ('mockup', 'design') AND a.project_id IS NULL;
DELETE FROM artifacts WHERE type IN ('mockup', 'design') AND project_id IS NULL;
ALTER TABLE artifacts DROP CONSTRAINT artifacts_project_required;
ALTER TABLE artifacts ADD CONSTRAINT artifacts_project_required CHECK (type IN ('note', 'report') OR project_id IS NOT NULL);

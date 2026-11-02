-- +goose Up
-- Phase 4.5: wiki pages and agent memory (PLAN.md §12). A wiki page is an
-- artifact of type `note` with a path; [[links]] in notes, other markdown
-- artifacts, ticket descriptions and comments are stored in knowledge_links.

ALTER TABLE artifacts DROP CONSTRAINT artifacts_type_check;
ALTER TABLE artifacts ADD CONSTRAINT artifacts_type_check
  CHECK (type IN ('prd', 'spec', 'adr', 'report', 'design', 'note'));

-- Notes are workspace-wide unless given a project; every other type keeps
-- its required project.
ALTER TABLE artifacts ALTER COLUMN project_id DROP NOT NULL;
ALTER TABLE artifacts ADD CONSTRAINT artifacts_project_required
  CHECK (type = 'note' OR project_id IS NOT NULL);

-- "Payments/Ledger/Rollback runbook": the page tree comes from the path and
-- the last segment is the title. Only notes have one.
ALTER TABLE artifacts ADD COLUMN path text;
ALTER TABLE artifacts ADD CONSTRAINT artifacts_path_only_notes
  CHECK ((type = 'note') = (path IS NOT NULL));
CREATE UNIQUE INDEX artifacts_note_path_idx ON artifacts (workspace_id, lower(path)) WHERE path IS NOT NULL;
CREATE INDEX artifacts_note_title_idx ON artifacts (workspace_id, lower(title)) WHERE type = 'note';

-- Old paths of renamed notes, so [[Old name]] keeps resolving.
CREATE TABLE artifact_aliases (
  workspace_id uuid NOT NULL REFERENCES workspaces(id),
  artifact_id  uuid NOT NULL REFERENCES artifacts(id) ON DELETE CASCADE,
  path         text NOT NULL,
  created_at   timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (workspace_id, path)
);
CREATE INDEX artifact_aliases_lower_idx ON artifact_aliases (workspace_id, lower(path));

-- Agent edits to shared notes under wiki:write = requires_approval are kept
-- as suggested versions until a person accepts (published) or rejects them.
ALTER TABLE artifact_versions ADD COLUMN state text NOT NULL DEFAULT 'published'
  CHECK (state IN ('published', 'suggested', 'rejected'));
ALTER TABLE artifact_versions ADD COLUMN based_on_version_id uuid REFERENCES artifact_versions(id) ON DELETE SET NULL;
ALTER TABLE artifact_versions ADD COLUMN decided_by_id uuid REFERENCES users(id);
ALTER TABLE artifact_versions ADD COLUMN decided_at timestamptz;
CREATE INDEX artifact_versions_suggested_idx ON artifact_versions (artifact_id) WHERE state = 'suggested';

-- Every [[link]] in current content. Rewritten for a source on each save.
CREATE TABLE knowledge_links (
  id                uuid PRIMARY KEY DEFAULT uuid_v7(),
  workspace_id      uuid NOT NULL REFERENCES workspaces(id),
  source_kind       text NOT NULL CHECK (source_kind IN ('artifact', 'ticket', 'comment')),
  source_id         uuid NOT NULL,          -- artifact, ticket or comment id
  source_version_id uuid REFERENCES artifact_versions(id) ON DELETE CASCADE,
  source_ticket_id  uuid REFERENCES tickets(id) ON DELETE CASCADE, -- ticket and comment sources
  target_kind       text NOT NULL CHECK (target_kind IN ('artifact', 'section', 'ticket', 'epic', 'agent', 'unresolved')),
  target_id         uuid,                   -- NULL when unresolved
  target_anchor     text NOT NULL DEFAULT '',
  target_text       text NOT NULL,          -- the ref as written, before "#" and "|"
  raw_text          text NOT NULL,          -- the whole [[…]]
  snippet           text NOT NULL DEFAULT '',
  position          integer NOT NULL,       -- order within the source
  created_at        timestamptz NOT NULL DEFAULT now(),
  CHECK ((target_kind = 'unresolved') = (target_id IS NULL))
);
CREATE INDEX knowledge_links_target_idx ON knowledge_links (target_kind, target_id);
CREATE INDEX knowledge_links_source_idx ON knowledge_links (source_kind, source_id);
CREATE INDEX knowledge_links_unresolved_idx ON knowledge_links (workspace_id, lower(target_text)) WHERE target_kind = 'unresolved';

-- +goose Down
DROP TABLE knowledge_links, artifact_aliases;
ALTER TABLE artifact_versions DROP COLUMN decided_at, DROP COLUMN decided_by_id, DROP COLUMN based_on_version_id, DROP COLUMN state;
DELETE FROM artifacts WHERE type = 'note';
ALTER TABLE artifacts DROP CONSTRAINT artifacts_path_only_notes, DROP COLUMN path;
ALTER TABLE artifacts DROP CONSTRAINT artifacts_project_required;
ALTER TABLE artifacts ALTER COLUMN project_id SET NOT NULL;
ALTER TABLE artifacts DROP CONSTRAINT artifacts_type_check;
ALTER TABLE artifacts ADD CONSTRAINT artifacts_type_check
  CHECK (type IN ('prd', 'spec', 'adr', 'report', 'design'));

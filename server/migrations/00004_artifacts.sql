-- +goose Up
-- Phase 3: artifacts (PLAN.md §4). Markdown bodies live in Postgres so they
-- can be versioned, searched, outlined and fed to agents; binaries (designs,
-- PDFs, images) live in object storage under artifact_versions.blob_key.

CREATE TABLE artifacts (
  id                 uuid PRIMARY KEY DEFAULT uuid_v7(),
  workspace_id       uuid NOT NULL REFERENCES workspaces(id),
  project_id         uuid NOT NULL REFERENCES projects(id),
  epic_id            uuid REFERENCES epics(id) ON DELETE SET NULL,
  type               text NOT NULL CHECK (type IN ('prd', 'spec', 'adr', 'report', 'design')),
  title              text NOT NULL CHECK (length(title) BETWEEN 1 AND 300),
  number             integer,                 -- ADR-012; NULL for other types
  summary            text NOT NULL DEFAULT '',
  author_type        text NOT NULL CHECK (author_type IN ('user', 'agent')),
  author_id          uuid NOT NULL,
  current_version_id uuid,                    -- FK added below (circular)
  -- Plain text of the current version, kept in sync on every new version.
  search_text        text NOT NULL DEFAULT '',
  search             tsvector GENERATED ALWAYS AS (
                       setweight(to_tsvector('english', title), 'A') ||
                       setweight(to_tsvector('english', summary), 'B') ||
                       setweight(to_tsvector('english', search_text), 'C')
                     ) STORED,
  created_at         timestamptz NOT NULL DEFAULT now(),
  updated_at         timestamptz NOT NULL DEFAULT now(),
  UNIQUE (workspace_id, type, number)
);
CREATE INDEX artifacts_project_idx ON artifacts (project_id);
CREATE INDEX artifacts_search_idx ON artifacts USING gin (search);

CREATE TABLE artifact_versions (
  id            uuid PRIMARY KEY DEFAULT uuid_v7(),
  artifact_id   uuid NOT NULL REFERENCES artifacts(id) ON DELETE CASCADE,
  seq           integer NOT NULL,             -- 1, 2, 3… per artifact
  label         text NOT NULL,                -- "v3", "v1.2", "draft"
  body_md       text,                         -- text artifacts
  blob_key      text,                         -- binary artifacts
  filename      text,
  content_type  text NOT NULL DEFAULT 'text/markdown',
  size_bytes    bigint NOT NULL DEFAULT 0,
  note          text NOT NULL DEFAULT '',     -- what changed
  created_by_type text NOT NULL CHECK (created_by_type IN ('user', 'agent')),
  created_by_id uuid NOT NULL,
  created_at    timestamptz NOT NULL DEFAULT now(),
  CHECK ((body_md IS NULL) <> (blob_key IS NULL)),
  UNIQUE (artifact_id, seq)
);

ALTER TABLE artifacts ADD CONSTRAINT artifacts_current_version_fk
  FOREIGN KEY (current_version_id) REFERENCES artifact_versions(id) ON DELETE SET NULL;

-- Headings extracted from a markdown version, in document order.
CREATE TABLE artifact_sections (
  id         uuid PRIMARY KEY DEFAULT uuid_v7(),
  version_id uuid NOT NULL REFERENCES artifact_versions(id) ON DELETE CASCADE,
  position   integer NOT NULL,
  level      integer NOT NULL CHECK (level BETWEEN 1 AND 6),
  anchor     text NOT NULL,
  heading    text NOT NULL,
  UNIQUE (version_id, position),
  UNIQUE (version_id, anchor)
);

-- Agent reads, for "Read by" chips and per-section citation counts.
-- Written by the Agent API (Phase 4); run_id gains its FK when runs exist.
CREATE TABLE artifact_reads (
  id          uuid PRIMARY KEY DEFAULT uuid_v7(),
  version_id  uuid NOT NULL REFERENCES artifact_versions(id) ON DELETE CASCADE,
  section_id  uuid REFERENCES artifact_sections(id) ON DELETE SET NULL,
  agent_id    uuid NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
  run_id      uuid,
  read_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX artifact_reads_version_idx ON artifact_reads (version_id);
CREATE INDEX artifact_reads_section_idx ON artifact_reads (section_id) WHERE section_id IS NOT NULL;

CREATE TABLE ticket_artifacts (
  ticket_id   uuid NOT NULL REFERENCES tickets(id) ON DELETE CASCADE,
  artifact_id uuid NOT NULL REFERENCES artifacts(id) ON DELETE CASCADE,
  relation    text NOT NULL DEFAULT 'context' CHECK (relation IN ('context', 'generated')),
  created_at  timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (ticket_id, artifact_id)
);
CREATE INDEX ticket_artifacts_artifact_idx ON ticket_artifacts (artifact_id);

CREATE TRIGGER artifacts_touch BEFORE UPDATE ON artifacts FOR EACH ROW EXECUTE FUNCTION touch_updated_at();

-- +goose Down
ALTER TABLE artifacts DROP CONSTRAINT artifacts_current_version_fk;
DROP TABLE ticket_artifacts, artifact_reads, artifact_sections, artifact_versions, artifacts;

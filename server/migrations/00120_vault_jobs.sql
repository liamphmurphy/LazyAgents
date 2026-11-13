-- +goose Up
-- Imported timestamps are explicit. Ordinary edits still get the database's
-- current time; only a deliberate timestamp change bypasses the touch.
-- +goose StatementBegin
CREATE FUNCTION knowledge_touch_updated_at() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.updated_at IS NOT DISTINCT FROM OLD.updated_at THEN NEW.updated_at:=now(); END IF;
 RETURN NEW;
END $$;
-- +goose StatementEnd
DROP TRIGGER artifacts_touch ON artifacts;
CREATE TRIGGER artifacts_touch BEFORE UPDATE ON artifacts FOR EACH ROW EXECUTE FUNCTION knowledge_touch_updated_at();
CREATE TABLE knowledge_imports (
 id uuid PRIMARY KEY DEFAULT uuid_v7(),workspace_id uuid NOT NULL REFERENCES workspaces(id)ON DELETE CASCADE,
 user_id uuid NOT NULL REFERENCES users(id)ON DELETE CASCADE,blob_key text NOT NULL,
 folder text NOT NULL DEFAULT '',policy text NOT NULL DEFAULT 'skip' CHECK(policy IN ('skip','replace','keep_both')),
 state text NOT NULL DEFAULT 'queued' CHECK(state IN('queued','scanning','preview','queued_commit','importing','done','failed','undone')),
 plan jsonb NOT NULL DEFAULT '{}',report jsonb NOT NULL DEFAULT '[]',error text NOT NULL DEFAULT '',
 files_done integer NOT NULL DEFAULT 0,lease_id uuid,lease_until timestamptz,
 created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX knowledge_import_queue ON knowledge_imports(state,created_at);
CREATE TABLE knowledge_import_files (
 import_id uuid NOT NULL REFERENCES knowledge_imports(id)ON DELETE CASCADE,source_path text NOT NULL,
 artifact_id uuid REFERENCES artifacts(id)ON DELETE SET NULL,
 before_id uuid REFERENCES artifact_versions(id)ON DELETE SET NULL,
 applied_id uuid REFERENCES artifact_versions(id)ON DELETE SET NULL,
 asset_id uuid REFERENCES knowledge_assets(id)ON DELETE SET NULL,
 created boolean NOT NULL DEFAULT false,status text NOT NULL,applied_at timestamptz NOT NULL DEFAULT now(),
 before_title text,before_path text,applied_title text,applied_path text,
 PRIMARY KEY(import_id,source_path)
);
CREATE TABLE knowledge_import_sources (
 workspace_id uuid NOT NULL REFERENCES workspaces(id)ON DELETE CASCADE,user_id uuid NOT NULL REFERENCES users(id)ON DELETE CASCADE,
 folder text NOT NULL,source_path text NOT NULL,artifact_id uuid NOT NULL REFERENCES artifacts(id)ON DELETE CASCADE,
 version_id uuid REFERENCES artifact_versions(id)ON DELETE SET NULL,
 PRIMARY KEY(workspace_id,user_id,folder,source_path)
);
-- +goose Down
DROP TRIGGER artifacts_touch ON artifacts;
CREATE TRIGGER artifacts_touch BEFORE UPDATE ON artifacts FOR EACH ROW EXECUTE FUNCTION touch_updated_at();
DROP FUNCTION knowledge_touch_updated_at();
DROP TABLE knowledge_import_sources;
DROP TABLE knowledge_import_files;
DROP TABLE knowledge_imports;

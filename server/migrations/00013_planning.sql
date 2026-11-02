-- +goose Up
-- Planning runs have no ticket until a person accepts their proposals.
ALTER TABLE runs ALTER COLUMN ticket_id DROP NOT NULL;
CREATE TABLE planning_sessions (
 id uuid PRIMARY KEY DEFAULT uuid_v7(),
 workspace_id uuid NOT NULL REFERENCES workspaces(id),
 project_id uuid NOT NULL REFERENCES projects(id),
 epic_id uuid REFERENCES epics(id),
 artifact_id uuid REFERENCES artifacts(id),
 source_version_id uuid REFERENCES artifact_versions(id),
 goal text NOT NULL,
 run_id uuid NOT NULL UNIQUE REFERENCES runs(id),
 state text NOT NULL DEFAULT 'planning' CHECK(state IN ('planning','review','accepted','dismissed')),
 proposals jsonb NOT NULL DEFAULT '[]',
 ticket_keys text[] NOT NULL DEFAULT '{}',
 revision integer NOT NULL DEFAULT 1,
 created_by uuid NOT NULL REFERENCES users(id),
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX planning_sessions_workspace ON planning_sessions(workspace_id,created_at DESC);
CREATE TRIGGER realtime_change AFTER INSERT OR UPDATE OR DELETE ON planning_sessions FOR EACH STATEMENT EXECUTE FUNCTION notify_change('planning.updated');

-- +goose Down
DROP TABLE planning_sessions;
-- A down migration cannot preserve standalone planning runs.
DELETE FROM runs WHERE ticket_id IS NULL;
ALTER TABLE runs ALTER COLUMN ticket_id SET NOT NULL;

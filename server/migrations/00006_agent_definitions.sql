-- +goose Up
-- Phase 4: agent definitions (PLAN.md §6.1). A definition version is
-- immutable; editing an agent writes a new version. Projects override parts
-- of it, and a project can only tighten scopes (enforced in agentdef).

CREATE TABLE agent_definition_versions (
  id                 uuid PRIMARY KEY DEFAULT uuid_v7(),
  agent_id           uuid NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
  version            integer NOT NULL,
  spec               jsonb NOT NULL,
  note               text NOT NULL DEFAULT '',
  created_by_user_id uuid REFERENCES users(id),
  created_at         timestamptz NOT NULL DEFAULT now(),
  UNIQUE (agent_id, version)
);

ALTER TABLE agents ADD COLUMN current_definition_version_id uuid REFERENCES agent_definition_versions(id);

CREATE TABLE agent_project_overrides (
  id                 uuid PRIMARY KEY DEFAULT uuid_v7(),
  agent_id           uuid NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
  project_id         uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  spec               jsonb NOT NULL,
  version            integer NOT NULL DEFAULT 1,
  updated_by_user_id uuid REFERENCES users(id),
  created_at         timestamptz NOT NULL DEFAULT now(),
  updated_at         timestamptz NOT NULL DEFAULT now(),
  UNIQUE (agent_id, project_id)
);
CREATE TRIGGER agent_project_overrides_touch BEFORE UPDATE ON agent_project_overrides FOR EACH ROW EXECUTE FUNCTION touch_updated_at();

-- +goose Down
DROP TABLE agent_project_overrides;
ALTER TABLE agents DROP COLUMN current_definition_version_id;
DROP TABLE agent_definition_versions;

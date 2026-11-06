-- +goose Up
-- UI design mockups: editable SVG artifacts with a person-approval gate. The
-- SVG source lives in artifact_versions.body_md (content_type image/svg+xml);
-- review_state is NULL for every other artifact type.

ALTER TABLE artifacts DROP CONSTRAINT artifacts_type_check;
ALTER TABLE artifacts ADD CONSTRAINT artifacts_type_check
  CHECK (type IN ('prd', 'spec', 'adr', 'report', 'design', 'note', 'mockup'));

ALTER TABLE artifact_versions ADD COLUMN review_state text
  CHECK (review_state IN ('pending', 'approved', 'changes_requested'));
ALTER TABLE artifact_versions ADD COLUMN reviewed_by_user_id uuid REFERENCES users(id);
ALTER TABLE artifact_versions ADD COLUMN reviewed_at timestamptz;
ALTER TABLE artifact_versions ADD COLUMN review_note text NOT NULL DEFAULT '';

ALTER TABLE tickets ADD COLUMN requires_mockup boolean NOT NULL DEFAULT false;
ALTER TABLE epics ADD COLUMN requires_mockup boolean NOT NULL DEFAULT false;

ALTER TABLE runs DROP CONSTRAINT runs_wait_reason_check;
ALTER TABLE runs ADD CONSTRAINT runs_wait_reason_check CHECK (wait_reason IN ('approval', 'human_reply', 'pause', 'delegation', 'mockup_review'));

-- +goose Down
UPDATE runs SET state = 'cancelled', wait_reason = NULL, ended_at = now() WHERE wait_reason = 'mockup_review';
ALTER TABLE runs DROP CONSTRAINT runs_wait_reason_check;
ALTER TABLE runs ADD CONSTRAINT runs_wait_reason_check CHECK (wait_reason IN ('approval', 'human_reply', 'pause', 'delegation'));
ALTER TABLE epics DROP COLUMN requires_mockup;
ALTER TABLE tickets DROP COLUMN requires_mockup;
ALTER TABLE artifact_versions DROP COLUMN review_note, DROP COLUMN reviewed_at, DROP COLUMN reviewed_by_user_id, DROP COLUMN review_state;
UPDATE planning_sessions SET artifact_id = NULL
  WHERE artifact_id IN (SELECT id FROM artifacts WHERE type = 'mockup');
UPDATE planning_sessions SET source_version_id = NULL
  WHERE source_version_id IN (SELECT v.id FROM artifact_versions v JOIN artifacts a ON a.id = v.artifact_id WHERE a.type = 'mockup');
DELETE FROM artifacts WHERE type = 'mockup';
ALTER TABLE artifacts DROP CONSTRAINT artifacts_type_check;
ALTER TABLE artifacts ADD CONSTRAINT artifacts_type_check
  CHECK (type IN ('prd', 'spec', 'adr', 'report', 'design', 'note'));

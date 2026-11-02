-- +goose Up
-- River maintains 24h, 7d and 30d snapshots; current workflow counts stay live.
CREATE TABLE metric_snapshots (
 workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
 range text NOT NULL CHECK (range IN ('24h','7d','30d')),
 payload jsonb NOT NULL,
 generated_at timestamptz NOT NULL,
 PRIMARY KEY(workspace_id,range)
);
CREATE INDEX activity_completion_idx ON activity_events(workspace_id,created_at,ticket_id)
 WHERE verb='status_changed' AND payload->>'to'='done';
CREATE TRIGGER realtime_change AFTER INSERT OR UPDATE OR DELETE ON metric_snapshots
 FOR EACH STATEMENT EXECUTE FUNCTION notify_change('metrics.updated');

-- +goose Down
DROP INDEX activity_completion_idx;
DROP TABLE metric_snapshots;

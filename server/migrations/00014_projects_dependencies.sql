-- +goose Up
ALTER TABLE projects ADD COLUMN archived boolean NOT NULL DEFAULT false;
ALTER TABLE projects ADD COLUMN revision integer NOT NULL DEFAULT 1;
ALTER TABLE tickets ADD COLUMN dependency_revision integer NOT NULL DEFAULT 1;
CREATE TABLE ticket_dependencies (
 ticket_id uuid NOT NULL REFERENCES tickets(id) ON DELETE CASCADE,
 blocker_id uuid NOT NULL REFERENCES tickets(id) ON DELETE CASCADE,
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(ticket_id,blocker_id),
 CHECK(ticket_id<>blocker_id)
);
CREATE INDEX ticket_dependencies_blocker ON ticket_dependencies(blocker_id);
CREATE TRIGGER realtime_change AFTER INSERT OR UPDATE OR DELETE ON ticket_dependencies FOR EACH STATEMENT EXECUTE FUNCTION notify_change('ticket.updated');

-- +goose Down
DROP TABLE ticket_dependencies;
ALTER TABLE tickets DROP COLUMN dependency_revision;
ALTER TABLE projects DROP COLUMN revision, DROP COLUMN archived;

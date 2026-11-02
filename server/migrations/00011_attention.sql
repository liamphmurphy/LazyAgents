-- +goose Up
CREATE TABLE inbox_reads (
 user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 item_id text NOT NULL,
 read_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY (user_id, item_id)
);
-- Notifications contain only invalidation hints, never content or credentials.
-- Statement triggers coalesce bulk changes; PostgreSQL delivers on commit.
-- +goose StatementBegin
CREATE FUNCTION notify_change() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 PERFORM pg_notify('lazyagents_changes', json_build_object('type', TG_ARGV[0], 'ids', ARRAY[]::text[])::text);
 RETURN NULL;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER realtime_change AFTER INSERT OR UPDATE OR DELETE ON tickets FOR EACH STATEMENT EXECUTE FUNCTION notify_change('ticket.updated');
CREATE TRIGGER realtime_change AFTER INSERT OR UPDATE OR DELETE ON activity_events FOR EACH STATEMENT EXECUTE FUNCTION notify_change('activity.created');
CREATE TRIGGER realtime_change AFTER INSERT OR UPDATE OR DELETE ON approval_requests FOR EACH STATEMENT EXECUTE FUNCTION notify_change('approval.updated');
CREATE TRIGGER realtime_change AFTER INSERT OR UPDATE OR DELETE ON runs FOR EACH STATEMENT EXECUTE FUNCTION notify_change('run.updated');
CREATE TRIGGER realtime_change AFTER INSERT OR UPDATE OR DELETE ON run_events FOR EACH STATEMENT EXECUTE FUNCTION notify_change('run.events');
CREATE TRIGGER realtime_change AFTER INSERT OR UPDATE OR DELETE ON run_turns FOR EACH STATEMENT EXECUTE FUNCTION notify_change('run.updated');
CREATE TRIGGER realtime_change AFTER INSERT OR UPDATE OR DELETE ON plan_steps FOR EACH STATEMENT EXECUTE FUNCTION notify_change('run.updated');
CREATE TRIGGER realtime_change AFTER INSERT OR UPDATE OR DELETE ON agents FOR EACH STATEMENT EXECUTE FUNCTION notify_change('agent.updated');
CREATE TRIGGER realtime_change AFTER INSERT OR UPDATE OR DELETE ON artifacts FOR EACH STATEMENT EXECUTE FUNCTION notify_change('artifact.updated');
CREATE TRIGGER realtime_change AFTER INSERT OR UPDATE OR DELETE ON artifact_versions FOR EACH STATEMENT EXECUTE FUNCTION notify_change('artifact.updated');
CREATE TRIGGER realtime_change AFTER INSERT OR UPDATE OR DELETE ON artifact_reads FOR EACH STATEMENT EXECUTE FUNCTION notify_change('artifact.updated');
CREATE TRIGGER realtime_change AFTER INSERT OR UPDATE OR DELETE ON ticket_artifacts FOR EACH STATEMENT EXECUTE FUNCTION notify_change('ticket.updated');
CREATE TRIGGER realtime_change AFTER INSERT OR UPDATE OR DELETE ON acceptance_criteria FOR EACH STATEMENT EXECUTE FUNCTION notify_change('ticket.updated');
CREATE TRIGGER realtime_change AFTER INSERT OR UPDATE OR DELETE ON comments FOR EACH STATEMENT EXECUTE FUNCTION notify_change('ticket.updated');
CREATE TRIGGER realtime_change AFTER INSERT OR UPDATE OR DELETE ON knowledge_links FOR EACH STATEMENT EXECUTE FUNCTION notify_change('knowledge.updated');
CREATE TRIGGER realtime_change AFTER INSERT OR UPDATE OR DELETE ON projects FOR EACH STATEMENT EXECUTE FUNCTION notify_change('project.updated');
CREATE TRIGGER realtime_change AFTER INSERT OR UPDATE OR DELETE ON epics FOR EACH STATEMENT EXECUTE FUNCTION notify_change('project.updated');

CREATE TRIGGER realtime_change AFTER INSERT OR UPDATE OR DELETE ON inbox_reads FOR EACH STATEMENT EXECUTE FUNCTION notify_change('inbox.updated');

-- +goose Down
DROP TRIGGER realtime_change ON inbox_reads;
DROP TRIGGER realtime_change ON tickets;
DROP TRIGGER realtime_change ON activity_events;
DROP TRIGGER realtime_change ON approval_requests;
DROP TRIGGER realtime_change ON runs;
DROP TRIGGER realtime_change ON run_events;
DROP TRIGGER realtime_change ON run_turns;
DROP TRIGGER realtime_change ON plan_steps;
DROP TRIGGER realtime_change ON agents;
DROP TRIGGER realtime_change ON artifacts;
DROP TRIGGER realtime_change ON artifact_versions;
DROP TRIGGER realtime_change ON artifact_reads;
DROP TRIGGER realtime_change ON ticket_artifacts;
DROP TRIGGER realtime_change ON acceptance_criteria;
DROP TRIGGER realtime_change ON comments;
DROP TRIGGER realtime_change ON knowledge_links;
DROP TRIGGER realtime_change ON projects;
DROP TRIGGER realtime_change ON epics;
DROP FUNCTION notify_change();
DROP TABLE inbox_reads;

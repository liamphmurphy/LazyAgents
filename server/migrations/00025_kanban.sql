-- +goose Up
-- Kanban board, ticket estimates and epic acceptance criteria
-- (Architecture/ADR - Kanban board, estimates and epic criteria, LAZYAGENTS-29).

-- Decision 1: add 'canceled'. Every existing value (backlog, todo, in_progress,
-- in_review, done) is already a board column, so no row changes status.
ALTER TABLE tickets DROP CONSTRAINT tickets_status_check;
ALTER TABLE tickets ADD CONSTRAINT tickets_status_check
  CHECK (status IN ('backlog', 'todo', 'in_progress', 'in_review', 'done', 'canceled'));

-- Decision 2: optimistic concurrency for PATCH /tickets/{key}.
ALTER TABLE tickets ADD COLUMN revision integer NOT NULL DEFAULT 1;

-- Decision 3: point estimates. The v1 scale is Fibonacci 1-13 (tracker.EstimateScale);
-- widening it (open question 1) only needs this CHECK replaced. The legacy free-text
-- `estimate` column stays as a note.
ALTER TABLE tickets
  ADD COLUMN estimate_points integer
    CONSTRAINT tickets_estimate_points_check CHECK (estimate_points IN (1, 2, 3, 5, 8, 13)),
  ADD COLUMN estimate_rationale text
    CONSTRAINT tickets_estimate_rationale_check CHECK (length(estimate_rationale) <= 2000),
  ADD COLUMN estimated_by_type text
    CONSTRAINT tickets_estimated_by_type_check CHECK (estimated_by_type IN ('user', 'agent')),
  ADD COLUMN estimated_by_id uuid,
  ADD COLUMN estimated_at timestamptz;

-- Decision 4: epic status and acceptance criteria. Existing epics start open.
ALTER TABLE epics ADD COLUMN status text NOT NULL DEFAULT 'open'
  CONSTRAINT epics_status_check CHECK (status IN ('open', 'done'));
-- Epic rollups, the done gate and ON DELETE SET NULL look tickets up by epic.
CREATE INDEX tickets_epic_idx ON tickets (epic_id) WHERE epic_id IS NOT NULL;

CREATE TABLE epic_criteria (
  id              uuid PRIMARY KEY DEFAULT uuid_v7(),
  epic_id         uuid NOT NULL REFERENCES epics(id) ON DELETE CASCADE,
  position        integer NOT NULL CHECK (position >= 1),
  text            text NOT NULL CHECK (length(text) BETWEEN 1 AND 1000),
  done            boolean NOT NULL DEFAULT false,
  checked_by_type text CHECK (checked_by_type IN ('user', 'agent')),
  checked_by_id   uuid,
  checked_at      timestamptz,
  created_at      timestamptz NOT NULL DEFAULT now(),
  updated_at      timestamptz NOT NULL DEFAULT now(),
  -- Deferrable so renumbering after a delete (position = position - 1) is checked
  -- at the end of the statement rather than row by row. Also serves (epic_id, position) lookups.
  CONSTRAINT epic_criteria_position_key UNIQUE (epic_id, position) DEFERRABLE INITIALLY IMMEDIATE
);
CREATE TRIGGER epic_criteria_touch BEFORE UPDATE ON epic_criteria FOR EACH ROW EXECUTE FUNCTION touch_updated_at();

-- Decision 5: realtime hints with real ids. notify_change() always sends empty ids,
-- and notify_chat_change() silently truncates at 50. notify_entity_change(type, id_col...)
-- sends the distinct non-null ids found in the given columns of the transition table.
-- More than 50 ids are sent as empty ids ("refetch all of this type"), which keeps the
-- payload far below pg_notify's 8000 byte limit without ever sending a partial list.
-- A statement that touched no rows (or only rows with null ids) sends nothing, so empty
-- ids always mean "refetch all", never "nothing changed".
-- +goose StatementBegin
CREATE FUNCTION notify_entity_change() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE ids text[];
DECLARE cols text[] := TG_ARGV[1:TG_NARGS - 1];
BEGIN
 IF TG_OP = 'DELETE' THEN
  SELECT array_agg(i) INTO ids FROM (SELECT DISTINCT to_jsonb(r)->>c AS i FROM old_rows r CROSS JOIN unnest(cols) c WHERE to_jsonb(r)->>c IS NOT NULL LIMIT 51) s;
 ELSE
  SELECT array_agg(i) INTO ids FROM (SELECT DISTINCT to_jsonb(r)->>c AS i FROM new_rows r CROSS JOIN unnest(cols) c WHERE to_jsonb(r)->>c IS NOT NULL LIMIT 51) s;
 END IF;
 IF ids IS NULL THEN
  RETURN NULL;
 END IF;
 IF cardinality(ids) > 50 THEN
  ids := ARRAY[]::text[];
 END IF;
 PERFORM pg_notify('lazyagents_changes', json_build_object('type', TG_ARGV[0], 'ids', ids)::text);
 RETURN NULL;
END;
$$;
-- +goose StatementEnd

-- An epic's rollups (points, open tickets) change when one of its tickets is added,
-- removed, moved between epics, or changes status or estimate. This sends epic.updated
-- with the affected epic ids (old and new epic on a move); it sends nothing when no
-- epic is affected.
-- +goose StatementBegin
CREATE FUNCTION notify_ticket_epic_change() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE ids text[];
BEGIN
 IF TG_OP = 'INSERT' THEN
  SELECT array_agg(i) INTO ids FROM (SELECT DISTINCT epic_id::text AS i FROM new_rows WHERE epic_id IS NOT NULL LIMIT 51) s;
 ELSIF TG_OP = 'DELETE' THEN
  SELECT array_agg(i) INTO ids FROM (SELECT DISTINCT epic_id::text AS i FROM old_rows WHERE epic_id IS NOT NULL LIMIT 51) s;
 ELSE
  SELECT array_agg(i) INTO ids FROM (
   SELECT DISTINCT e::text AS i
   FROM old_rows o JOIN new_rows n ON n.id = o.id
   CROSS JOIN LATERAL (VALUES (o.epic_id), (n.epic_id)) v(e)
   WHERE e IS NOT NULL
     AND (o.epic_id IS DISTINCT FROM n.epic_id
       OR o.status IS DISTINCT FROM n.status
       OR o.estimate_points IS DISTINCT FROM n.estimate_points)
   LIMIT 51) s;
 END IF;
 IF ids IS NULL THEN
  RETURN NULL;
 END IF;
 IF cardinality(ids) > 50 THEN
  ids := ARRAY[]::text[];
 END IF;
 PERFORM pg_notify('lazyagents_changes', json_build_object('type', 'epic.updated', 'ids', ids)::text);
 RETURN NULL;
END;
$$;
-- +goose StatementEnd

-- Transition tables are not allowed on multi-event triggers, so one trigger per event.
DROP TRIGGER realtime_change ON tickets;
CREATE TRIGGER realtime_change_ins AFTER INSERT ON tickets REFERENCING NEW TABLE AS new_rows FOR EACH STATEMENT EXECUTE FUNCTION notify_entity_change('ticket.updated', 'id');
CREATE TRIGGER realtime_change_upd AFTER UPDATE ON tickets REFERENCING NEW TABLE AS new_rows FOR EACH STATEMENT EXECUTE FUNCTION notify_entity_change('ticket.updated', 'id');
CREATE TRIGGER realtime_change_del AFTER DELETE ON tickets REFERENCING OLD TABLE AS old_rows FOR EACH STATEMENT EXECUTE FUNCTION notify_entity_change('ticket.updated', 'id');
CREATE TRIGGER realtime_epic_ins AFTER INSERT ON tickets REFERENCING NEW TABLE AS new_rows FOR EACH STATEMENT EXECUTE FUNCTION notify_ticket_epic_change();
CREATE TRIGGER realtime_epic_upd AFTER UPDATE ON tickets REFERENCING OLD TABLE AS old_rows NEW TABLE AS new_rows FOR EACH STATEMENT EXECUTE FUNCTION notify_ticket_epic_change();
CREATE TRIGGER realtime_epic_del AFTER DELETE ON tickets REFERENCING OLD TABLE AS old_rows FOR EACH STATEMENT EXECUTE FUNCTION notify_ticket_epic_change();

DROP TRIGGER realtime_change ON acceptance_criteria;
CREATE TRIGGER realtime_change_ins AFTER INSERT ON acceptance_criteria REFERENCING NEW TABLE AS new_rows FOR EACH STATEMENT EXECUTE FUNCTION notify_entity_change('ticket.updated', 'ticket_id');
CREATE TRIGGER realtime_change_upd AFTER UPDATE ON acceptance_criteria REFERENCING NEW TABLE AS new_rows FOR EACH STATEMENT EXECUTE FUNCTION notify_entity_change('ticket.updated', 'ticket_id');
CREATE TRIGGER realtime_change_del AFTER DELETE ON acceptance_criteria REFERENCING OLD TABLE AS old_rows FOR EACH STATEMENT EXECUTE FUNCTION notify_entity_change('ticket.updated', 'ticket_id');

DROP TRIGGER realtime_change ON comments;
CREATE TRIGGER realtime_change_ins AFTER INSERT ON comments REFERENCING NEW TABLE AS new_rows FOR EACH STATEMENT EXECUTE FUNCTION notify_entity_change('ticket.updated', 'ticket_id');
CREATE TRIGGER realtime_change_upd AFTER UPDATE ON comments REFERENCING NEW TABLE AS new_rows FOR EACH STATEMENT EXECUTE FUNCTION notify_entity_change('ticket.updated', 'ticket_id');
CREATE TRIGGER realtime_change_del AFTER DELETE ON comments REFERENCING OLD TABLE AS old_rows FOR EACH STATEMENT EXECUTE FUNCTION notify_entity_change('ticket.updated', 'ticket_id');

DROP TRIGGER realtime_change ON ticket_artifacts;
CREATE TRIGGER realtime_change_ins AFTER INSERT ON ticket_artifacts REFERENCING NEW TABLE AS new_rows FOR EACH STATEMENT EXECUTE FUNCTION notify_entity_change('ticket.updated', 'ticket_id');
CREATE TRIGGER realtime_change_upd AFTER UPDATE ON ticket_artifacts REFERENCING NEW TABLE AS new_rows FOR EACH STATEMENT EXECUTE FUNCTION notify_entity_change('ticket.updated', 'ticket_id');
CREATE TRIGGER realtime_change_del AFTER DELETE ON ticket_artifacts REFERENCING OLD TABLE AS old_rows FOR EACH STATEMENT EXECUTE FUNCTION notify_entity_change('ticket.updated', 'ticket_id');

-- Both ends of the edge: the dependent's blocked_by and the blocker's blocks list.
DROP TRIGGER realtime_change ON ticket_dependencies;
CREATE TRIGGER realtime_change_ins AFTER INSERT ON ticket_dependencies REFERENCING NEW TABLE AS new_rows FOR EACH STATEMENT EXECUTE FUNCTION notify_entity_change('ticket.updated', 'ticket_id', 'blocker_id');
CREATE TRIGGER realtime_change_upd AFTER UPDATE ON ticket_dependencies REFERENCING NEW TABLE AS new_rows FOR EACH STATEMENT EXECUTE FUNCTION notify_entity_change('ticket.updated', 'ticket_id', 'blocker_id');
CREATE TRIGGER realtime_change_del AFTER DELETE ON ticket_dependencies REFERENCING OLD TABLE AS old_rows FOR EACH STATEMENT EXECUTE FUNCTION notify_entity_change('ticket.updated', 'ticket_id', 'blocker_id');

-- Epics stop sending project.updated for their own rows and send epic.updated instead.
DROP TRIGGER realtime_change ON epics;
CREATE TRIGGER realtime_change_ins AFTER INSERT ON epics REFERENCING NEW TABLE AS new_rows FOR EACH STATEMENT EXECUTE FUNCTION notify_entity_change('epic.updated', 'id');
CREATE TRIGGER realtime_change_upd AFTER UPDATE ON epics REFERENCING NEW TABLE AS new_rows FOR EACH STATEMENT EXECUTE FUNCTION notify_entity_change('epic.updated', 'id');
CREATE TRIGGER realtime_change_del AFTER DELETE ON epics REFERENCING OLD TABLE AS old_rows FOR EACH STATEMENT EXECUTE FUNCTION notify_entity_change('epic.updated', 'id');

CREATE TRIGGER realtime_change_ins AFTER INSERT ON epic_criteria REFERENCING NEW TABLE AS new_rows FOR EACH STATEMENT EXECUTE FUNCTION notify_entity_change('epic.updated', 'epic_id');
CREATE TRIGGER realtime_change_upd AFTER UPDATE ON epic_criteria REFERENCING NEW TABLE AS new_rows FOR EACH STATEMENT EXECUTE FUNCTION notify_entity_change('epic.updated', 'epic_id');
CREATE TRIGGER realtime_change_del AFTER DELETE ON epic_criteria REFERENCING OLD TABLE AS old_rows FOR EACH STATEMENT EXECUTE FUNCTION notify_entity_change('epic.updated', 'epic_id');

-- +goose Down
DROP TABLE epic_criteria;
DROP INDEX tickets_epic_idx;

DROP TRIGGER realtime_change_ins ON epics;
DROP TRIGGER realtime_change_upd ON epics;
DROP TRIGGER realtime_change_del ON epics;
CREATE TRIGGER realtime_change AFTER INSERT OR UPDATE OR DELETE ON epics FOR EACH STATEMENT EXECUTE FUNCTION notify_change('project.updated');

DROP TRIGGER realtime_change_ins ON ticket_dependencies;
DROP TRIGGER realtime_change_upd ON ticket_dependencies;
DROP TRIGGER realtime_change_del ON ticket_dependencies;
CREATE TRIGGER realtime_change AFTER INSERT OR UPDATE OR DELETE ON ticket_dependencies FOR EACH STATEMENT EXECUTE FUNCTION notify_change('ticket.updated');

DROP TRIGGER realtime_change_ins ON ticket_artifacts;
DROP TRIGGER realtime_change_upd ON ticket_artifacts;
DROP TRIGGER realtime_change_del ON ticket_artifacts;
CREATE TRIGGER realtime_change AFTER INSERT OR UPDATE OR DELETE ON ticket_artifacts FOR EACH STATEMENT EXECUTE FUNCTION notify_change('ticket.updated');

DROP TRIGGER realtime_change_ins ON comments;
DROP TRIGGER realtime_change_upd ON comments;
DROP TRIGGER realtime_change_del ON comments;
CREATE TRIGGER realtime_change AFTER INSERT OR UPDATE OR DELETE ON comments FOR EACH STATEMENT EXECUTE FUNCTION notify_change('ticket.updated');

DROP TRIGGER realtime_change_ins ON acceptance_criteria;
DROP TRIGGER realtime_change_upd ON acceptance_criteria;
DROP TRIGGER realtime_change_del ON acceptance_criteria;
CREATE TRIGGER realtime_change AFTER INSERT OR UPDATE OR DELETE ON acceptance_criteria FOR EACH STATEMENT EXECUTE FUNCTION notify_change('ticket.updated');

DROP TRIGGER realtime_epic_ins ON tickets;
DROP TRIGGER realtime_epic_upd ON tickets;
DROP TRIGGER realtime_epic_del ON tickets;
DROP TRIGGER realtime_change_ins ON tickets;
DROP TRIGGER realtime_change_upd ON tickets;
DROP TRIGGER realtime_change_del ON tickets;
CREATE TRIGGER realtime_change AFTER INSERT OR UPDATE OR DELETE ON tickets FOR EACH STATEMENT EXECUTE FUNCTION notify_change('ticket.updated');

DROP FUNCTION notify_ticket_epic_change();
DROP FUNCTION notify_entity_change();

ALTER TABLE epics DROP COLUMN status;
ALTER TABLE tickets
  DROP COLUMN estimated_at,
  DROP COLUMN estimated_by_id,
  DROP COLUMN estimated_by_type,
  DROP COLUMN estimate_rationale,
  DROP COLUMN estimate_points,
  DROP COLUMN revision;
-- The old schema has no canceled status. Canceled tickets become done: both are
-- closed, and unlike backlog/todo, done is never picked up by dispatch.
UPDATE tickets SET status = 'done' WHERE status = 'canceled';
ALTER TABLE tickets DROP CONSTRAINT tickets_status_check;
ALTER TABLE tickets ADD CONSTRAINT tickets_status_check
  CHECK (status IN ('backlog', 'todo', 'in_progress', 'in_review', 'done'));

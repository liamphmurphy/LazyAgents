-- +goose Up
-- Empty queue polling must not invalidate the UI. Transition-table triggers
-- emit run.updated only for affected runs, using migration 25's empty-row guard.
DROP TRIGGER realtime_change ON runs;
CREATE TRIGGER realtime_change_ins AFTER INSERT ON runs
  REFERENCING NEW TABLE AS new_rows FOR EACH STATEMENT
  EXECUTE FUNCTION notify_entity_change('run.updated', 'id');
CREATE TRIGGER realtime_change_upd AFTER UPDATE ON runs
  REFERENCING NEW TABLE AS new_rows FOR EACH STATEMENT
  EXECUTE FUNCTION notify_entity_change('run.updated', 'id');
CREATE TRIGGER realtime_change_del AFTER DELETE ON runs
  REFERENCING OLD TABLE AS old_rows FOR EACH STATEMENT
  EXECUTE FUNCTION notify_entity_change('run.updated', 'id');
DROP TRIGGER realtime_change ON run_turns;
CREATE TRIGGER realtime_change_ins AFTER INSERT ON run_turns
  REFERENCING NEW TABLE AS new_rows FOR EACH STATEMENT
  EXECUTE FUNCTION notify_entity_change('run.updated', 'run_id');
CREATE TRIGGER realtime_change_upd AFTER UPDATE ON run_turns
  REFERENCING NEW TABLE AS new_rows FOR EACH STATEMENT
  EXECUTE FUNCTION notify_entity_change('run.updated', 'run_id');
CREATE TRIGGER realtime_change_del AFTER DELETE ON run_turns
  REFERENCING OLD TABLE AS old_rows FOR EACH STATEMENT
  EXECUTE FUNCTION notify_entity_change('run.updated', 'run_id');
DROP TRIGGER realtime_change ON plan_steps;
CREATE TRIGGER realtime_change_ins AFTER INSERT ON plan_steps
  REFERENCING NEW TABLE AS new_rows FOR EACH STATEMENT
  EXECUTE FUNCTION notify_entity_change('run.updated', 'run_id');
CREATE TRIGGER realtime_change_upd AFTER UPDATE ON plan_steps
  REFERENCING NEW TABLE AS new_rows FOR EACH STATEMENT
  EXECUTE FUNCTION notify_entity_change('run.updated', 'run_id');
CREATE TRIGGER realtime_change_del AFTER DELETE ON plan_steps
  REFERENCING OLD TABLE AS old_rows FOR EACH STATEMENT
  EXECUTE FUNCTION notify_entity_change('run.updated', 'run_id');

-- +goose Down
DROP TRIGGER realtime_change_ins ON plan_steps;
DROP TRIGGER realtime_change_upd ON plan_steps;
DROP TRIGGER realtime_change_del ON plan_steps;
CREATE TRIGGER realtime_change AFTER INSERT OR UPDATE OR DELETE ON plan_steps
  FOR EACH STATEMENT EXECUTE FUNCTION notify_change('run.updated');
DROP TRIGGER realtime_change_ins ON run_turns;
DROP TRIGGER realtime_change_upd ON run_turns;
DROP TRIGGER realtime_change_del ON run_turns;
CREATE TRIGGER realtime_change AFTER INSERT OR UPDATE OR DELETE ON run_turns
  FOR EACH STATEMENT EXECUTE FUNCTION notify_change('run.updated');
DROP TRIGGER realtime_change_ins ON runs;
DROP TRIGGER realtime_change_upd ON runs;
DROP TRIGGER realtime_change_del ON runs;
CREATE TRIGGER realtime_change AFTER INSERT OR UPDATE OR DELETE ON runs
  FOR EACH STATEMENT EXECUTE FUNCTION notify_change('run.updated');

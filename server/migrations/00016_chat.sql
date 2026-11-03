-- +goose Up
-- runs.purpose makes the runner's ticket/planning/chat branches explicit instead of
-- inferring them from ticket_id IS NULL. Delegated (sub agent) runs keep the parent's
-- ticket_id, so every ticket-less run today is a planning run. A ticket-less run with
-- no planning_sessions row (planning.go creates the run before its session) is still
-- backfilled as 'planning', which is how the runner already treated it.
ALTER TABLE runs ADD COLUMN purpose text;
UPDATE runs SET purpose = CASE
  WHEN ticket_id IS NULL OR EXISTS (SELECT 1 FROM planning_sessions p WHERE p.run_id = runs.id) THEN 'planning'
  ELSE 'ticket' END;
ALTER TABLE runs ALTER COLUMN purpose SET NOT NULL;
ALTER TABLE runs ADD CONSTRAINT runs_purpose_check CHECK (purpose IN ('ticket', 'planning', 'chat'));
-- Ticket runs have a ticket. Planning runs have none, and neither do chat runs.
ALTER TABLE runs ADD CONSTRAINT runs_purpose_ticket_check CHECK ((purpose = 'ticket') = (ticket_id IS NOT NULL));

CREATE TABLE chat_sessions (
 id uuid PRIMARY KEY DEFAULT uuid_v7(),
 workspace_id uuid NOT NULL REFERENCES workspaces(id),
 project_id uuid NOT NULL REFERENCES projects(id),
 title text NOT NULL DEFAULT '',
 created_by uuid NOT NULL REFERENCES users(id),
 agent_id uuid NOT NULL REFERENCES agents(id),
 harness text NOT NULL,
 model text NOT NULL,
 current_run_id uuid REFERENCES runs(id) ON DELETE SET NULL,
 state text NOT NULL DEFAULT 'idle' CHECK (state IN ('idle', 'running', 'failed')),
 archived boolean NOT NULL DEFAULT false,
 revision integer NOT NULL DEFAULT 1,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 last_message_at timestamptz
);
-- Serves ListChatSessions: one project and creator, newest activity first.
CREATE INDEX chat_sessions_list ON chat_sessions (project_id, created_by, (COALESCE(last_message_at, created_at)) DESC, id DESC) WHERE NOT archived;

CREATE TABLE chat_messages (
 id uuid PRIMARY KEY DEFAULT uuid_v7(),
 session_id uuid NOT NULL REFERENCES chat_sessions(id) ON DELETE CASCADE,
 seq bigint NOT NULL,
 role text NOT NULL CHECK (role IN ('user', 'assistant', 'system')),
 body_md text NOT NULL,
 harness text,
 model text,
 run_id uuid REFERENCES runs(id) ON DELETE SET NULL,
 run_turn_id uuid REFERENCES run_turns(id) ON DELETE SET NULL,
 context_refs jsonb NOT NULL DEFAULT '[]',
 citations jsonb NOT NULL DEFAULT '[]',
 usage jsonb,
 created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE (session_id, seq)
);

-- notify_change() always sends empty ids. Chat clients need the chat id, so these
-- triggers use transition tables and send the distinct chat ids (capped so the
-- payload stays well under pg_notify's 8000 byte limit). TG_ARGV[0] is the event
-- type, TG_ARGV[1] the column holding the chat id.
-- +goose StatementBegin
CREATE FUNCTION notify_chat_change() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE ids text[];
BEGIN
 IF TG_OP = 'DELETE' THEN
  SELECT coalesce(array_agg(i), ARRAY[]::text[]) INTO ids FROM (SELECT DISTINCT to_jsonb(r)->>TG_ARGV[1] AS i FROM old_rows r LIMIT 50) s;
 ELSE
  SELECT coalesce(array_agg(i), ARRAY[]::text[]) INTO ids FROM (SELECT DISTINCT to_jsonb(r)->>TG_ARGV[1] AS i FROM new_rows r LIMIT 50) s;
 END IF;
 PERFORM pg_notify('lazyagents_changes', json_build_object('type', TG_ARGV[0], 'ids', ids)::text);
 RETURN NULL;
END;
$$;
-- +goose StatementEnd
-- Transition tables are not allowed on multi-event triggers, so one trigger per event.
CREATE TRIGGER realtime_change_ins AFTER INSERT ON chat_sessions REFERENCING NEW TABLE AS new_rows FOR EACH STATEMENT EXECUTE FUNCTION notify_chat_change('chat.updated', 'id');
CREATE TRIGGER realtime_change_upd AFTER UPDATE ON chat_sessions REFERENCING NEW TABLE AS new_rows FOR EACH STATEMENT EXECUTE FUNCTION notify_chat_change('chat.updated', 'id');
CREATE TRIGGER realtime_change_del AFTER DELETE ON chat_sessions REFERENCING OLD TABLE AS old_rows FOR EACH STATEMENT EXECUTE FUNCTION notify_chat_change('chat.updated', 'id');
CREATE TRIGGER realtime_change_ins AFTER INSERT ON chat_messages REFERENCING NEW TABLE AS new_rows FOR EACH STATEMENT EXECUTE FUNCTION notify_chat_change('chat.updated', 'session_id');
CREATE TRIGGER realtime_change_upd AFTER UPDATE ON chat_messages REFERENCING NEW TABLE AS new_rows FOR EACH STATEMENT EXECUTE FUNCTION notify_chat_change('chat.updated', 'session_id');
CREATE TRIGGER realtime_change_del AFTER DELETE ON chat_messages REFERENCING OLD TABLE AS old_rows FOR EACH STATEMENT EXECUTE FUNCTION notify_chat_change('chat.updated', 'session_id');

-- +goose Down
DROP TABLE chat_messages;
DROP TABLE chat_sessions;
DROP FUNCTION notify_chat_change();
-- A down migration cannot keep chat runs: the old schema requires every ticket-less run to be planning.
DELETE FROM runs WHERE purpose = 'chat';
ALTER TABLE runs DROP CONSTRAINT runs_purpose_ticket_check;
ALTER TABLE runs DROP CONSTRAINT runs_purpose_check;
ALTER TABLE runs DROP COLUMN purpose;

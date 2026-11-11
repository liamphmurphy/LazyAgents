-- +goose Up
-- One reply can offer several tickets in its ```dispatch card, and the person
-- can start each of them, so a reply starts at most one run per ticket rather
-- than one run in all.
ALTER TABLE chat_ticket_runs ADD COLUMN ticket_id uuid REFERENCES tickets(id) ON DELETE CASCADE;
UPDATE chat_ticket_runs ctr SET ticket_id = r.ticket_id FROM runs r WHERE r.id = ctr.run_id;
-- Chats only start ticket runs, but keep any row without a ticket rather than fail the migration.
ALTER TABLE chat_ticket_runs DROP CONSTRAINT chat_ticket_runs_message_id_key;
CREATE UNIQUE INDEX chat_ticket_runs_message_ticket ON chat_ticket_runs (message_id, ticket_id);

-- +goose Down
-- Lossy: when a reply started several runs, the chat stops following all but the first.
DELETE FROM chat_ticket_runs a USING chat_ticket_runs b
  WHERE a.message_id = b.message_id AND (a.created_at, a.run_id) > (b.created_at, b.run_id);
DROP INDEX chat_ticket_runs_message_ticket;
ALTER TABLE chat_ticket_runs ADD CONSTRAINT chat_ticket_runs_message_id_key UNIQUE (message_id);
ALTER TABLE chat_ticket_runs DROP COLUMN ticket_id;

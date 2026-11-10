-- +goose Up
-- Ticket runs a chat started: the persona offered a ```dispatch card and the
-- person pressed Start. The chat follows these runs above its message box.
-- message_id is the reply that offered the card, so it can't start twice.
CREATE TABLE chat_ticket_runs (
  chat_id    uuid NOT NULL REFERENCES chat_sessions(id) ON DELETE CASCADE,
  run_id     uuid NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
  message_id uuid UNIQUE REFERENCES chat_messages(id) ON DELETE SET NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (chat_id, run_id)
);
CREATE INDEX chat_ticket_runs_run ON chat_ticket_runs (run_id);

-- +goose Down
DROP TABLE chat_ticket_runs;

-- +goose Up
-- What the chat was last told about each run it started, so a run that needs
-- the person, finishes or fails wakes the persona once: '' (nothing yet),
-- needs:<wait reason>:<turn id> for one wait, or the run's ended state.
ALTER TABLE chat_ticket_runs ADD COLUMN reported text NOT NULL DEFAULT '',
  ADD COLUMN reported_at timestamptz;
-- Runs that ended before follow-ups existed aren't announced after the fact.
UPDATE chat_ticket_runs ctr SET reported = r.state, reported_at = now()
FROM runs r WHERE r.id = ctr.run_id AND r.state IN ('completed', 'failed', 'blocked', 'cancelled');

-- +goose Down
ALTER TABLE chat_ticket_runs DROP COLUMN reported_at, DROP COLUMN reported;

-- +goose Up
-- A delegated run that called complete_run waits on 'checks' while the runner
-- checks its result against its contract, before its parent is woken.
ALTER TABLE runs DROP CONSTRAINT runs_wait_reason_check;
ALTER TABLE runs ADD CONSTRAINT runs_wait_reason_check CHECK (wait_reason = ANY (ARRAY['approval', 'human_reply', 'pause', 'delegation', 'mockup_review', 'checks']));
-- "Reported to the parent" is now checks.reported_turn <> 0 rather than checks
-- being set. Checks stored before were all stored when reported.
UPDATE runs SET checks = checks || '{"reported_turn": 1}'::jsonb
WHERE checks IS NOT NULL AND coalesce((checks->>'reported_turn')::int, 0) = 0;

-- +goose Down
-- Runs waiting on checks can't wait on anything older: complete them with the
-- outcome they reported (their parents' next report checks them).
UPDATE runs SET state = 'completed', wait_reason = NULL, ended_at = now() WHERE wait_reason = 'checks';
ALTER TABLE runs DROP CONSTRAINT runs_wait_reason_check;
ALTER TABLE runs ADD CONSTRAINT runs_wait_reason_check CHECK (wait_reason = ANY (ARRAY['approval', 'human_reply', 'pause', 'delegation', 'mockup_review']));

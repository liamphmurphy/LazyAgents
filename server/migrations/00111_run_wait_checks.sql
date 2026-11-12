-- +goose Up
-- A delegated run that called complete_run waits on 'checks' while the runner
-- checks its result against its contract, before its parent is woken.
ALTER TABLE runs DROP CONSTRAINT runs_wait_reason_check;
ALTER TABLE runs ADD CONSTRAINT runs_wait_reason_check CHECK (wait_reason = ANY (ARRAY['approval', 'human_reply', 'pause', 'delegation', 'mockup_review', 'checks']));

-- +goose Down
UPDATE runs SET wait_reason = NULL WHERE wait_reason = 'checks';
ALTER TABLE runs DROP CONSTRAINT runs_wait_reason_check;
ALTER TABLE runs ADD CONSTRAINT runs_wait_reason_check CHECK (wait_reason = ANY (ARRAY['approval', 'human_reply', 'pause', 'delegation', 'mockup_review']));

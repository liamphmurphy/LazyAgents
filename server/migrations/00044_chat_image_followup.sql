-- +goose Up
-- A chat turn whose generate_image jobs ran is followed by one automatic
-- image_followup turn that shows the assistant what came of them (each image
-- with its prompt, or why it failed) so it can react. It has no user message.
ALTER TABLE run_turns DROP CONSTRAINT run_turns_trigger_check;
ALTER TABLE run_turns ADD CONSTRAINT run_turns_trigger_check
  CHECK (trigger IN ('start', 'approval', 'comment', 'resume', 'delegation', 'image_followup'));

ALTER TABLE chat_image_jobs
  -- Why a failed job failed: refused by moderation, or anything else.
  ADD COLUMN failure_kind text CHECK (failure_kind IN ('moderation', 'error')),
  -- The outbox files the job's Codex step produced (new names after the step).
  ADD COLUMN outbox_files text[] NOT NULL DEFAULT '{}',
  -- The chat attachment its image became once the turn's reply was stored.
  ADD COLUMN attachment_id uuid REFERENCES chat_attachments(id) ON DELETE SET NULL,
  -- The image_followup turn that reported this job to the assistant.
  ADD COLUMN followup_turn_id uuid REFERENCES run_turns(id) ON DELETE SET NULL;
UPDATE chat_image_jobs SET failure_kind = 'error' WHERE state = 'failed';
ALTER TABLE chat_image_jobs ADD CONSTRAINT chat_image_jobs_failure_kind CHECK ((failure_kind IS NOT NULL) = (state = 'failed'));
CREATE INDEX chat_image_jobs_followup ON chat_image_jobs (followup_turn_id) WHERE followup_turn_id IS NOT NULL;

-- +goose Down
DROP INDEX chat_image_jobs_followup;
ALTER TABLE chat_image_jobs DROP CONSTRAINT chat_image_jobs_failure_kind,
  DROP COLUMN followup_turn_id, DROP COLUMN attachment_id, DROP COLUMN outbox_files, DROP COLUMN failure_kind;
UPDATE run_turns SET trigger = 'resume' WHERE trigger = 'image_followup';
ALTER TABLE run_turns DROP CONSTRAINT run_turns_trigger_check;
ALTER TABLE run_turns ADD CONSTRAINT run_turns_trigger_check
  CHECK (trigger IN ('start', 'approval', 'comment', 'resume', 'delegation'));

-- +goose Up
-- A proposed persona's picture: the image job propose_persona queued with the
-- proposal (its avatar_prompt). Approving the proposal makes that image the
-- new persona's avatar and its reference likeness in the Library.
ALTER TABLE persona_proposals ADD COLUMN avatar_job_id uuid REFERENCES chat_image_jobs(id) ON DELETE SET NULL;

-- +goose Down
ALTER TABLE persona_proposals DROP COLUMN avatar_job_id;

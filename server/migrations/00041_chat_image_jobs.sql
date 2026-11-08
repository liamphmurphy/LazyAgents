-- +goose Up
-- Images a non-Codex chat asked for with generate_image. The runner makes them
-- with a one-shot Codex step after the turn's reply, in the same workspace.
CREATE TABLE chat_image_jobs (
  id               uuid PRIMARY KEY DEFAULT uuid_v7(),
  workspace_id     uuid NOT NULL REFERENCES workspaces(id),
  run_id           uuid NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
  turn_id          uuid NOT NULL REFERENCES run_turns(id) ON DELETE CASCADE,
  chat_id          uuid NOT NULL REFERENCES chat_sessions(id) ON DELETE CASCADE,
  prompt           text NOT NULL CHECK (length(prompt) BETWEEN 1 AND 4000),
  -- The Codex model chosen when the job was queued (the catalog's default).
  model            text NOT NULL,
  state            text NOT NULL DEFAULT 'queued' CHECK (state IN ('queued', 'running', 'done', 'failed')),
  -- The Codex thread that made the image; the policy hook lets that step copy
  -- from its own generated_images directory while the job is running.
  codex_session_id text,
  error            text,
  created_at       timestamptz NOT NULL DEFAULT now(),
  finished_at      timestamptz
);
CREATE INDEX chat_image_jobs_turn ON chat_image_jobs (run_id, turn_id);

-- +goose Down
DROP TABLE chat_image_jobs;

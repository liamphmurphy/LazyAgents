-- +goose Up
-- Audio a coding chat renders with code during its turn (MIDI through a
-- soundfont, a synth script, ffmpeg). The agent moves a job through its
-- stages with the audio tools; the renderer it wrote drops progress.json into
-- .lazyagents/audio/<id>/, which the runner copies here while the turn runs.
-- A finished file is kept in the Library. Unlike a video there are no frames:
-- the card shows the stages, then an inline player.
CREATE TABLE chat_audio_jobs (
  id                  uuid PRIMARY KEY DEFAULT uuid_v7(),
  workspace_id        uuid NOT NULL REFERENCES workspaces(id),
  user_id             uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  -- Detached (NULL) when the chat is deleted; the sweep then removes the job
  -- and the object only it holds.
  chat_id             uuid REFERENCES chat_sessions(id) ON DELETE SET NULL,
  run_id              uuid REFERENCES runs(id) ON DELETE SET NULL,
  turn_id             uuid REFERENCES run_turns(id) ON DELETE SET NULL,
  -- The reply the job belongs to, once the turn's reply is stored.
  message_id          uuid REFERENCES chat_messages(id) ON DELETE SET NULL,
  persona_id          uuid REFERENCES personas(id) ON DELETE SET NULL,
  title               text NOT NULL CHECK (length(title) BETWEEN 1 AND 200),
  -- The planned length; the finished file's own length is output_duration_ms.
  duration_ms         integer NOT NULL CHECK (duration_ms BETWEEN 100 AND 3600000),
  -- The renderer, workspace-relative, and how the person is told it was made.
  script_path         text NOT NULL DEFAULT '',
  method              text NOT NULL DEFAULT '' CHECK (length(method) <= 200),
  stage               text NOT NULL DEFAULT 'compose' CHECK (stage IN ('compose', 'render', 'check', 'save')),
  state               text NOT NULL DEFAULT 'running' CHECK (state IN ('running', 'done', 'failed')),
  -- [{"label": "...", "passed": true|false|null}], what the agent measured on the render.
  checks              jsonb NOT NULL DEFAULT '[]',
  -- What the renderer reports in progress.json: 0-100, and a short line.
  percent             integer NOT NULL DEFAULT 0 CHECK (percent BETWEEN 0 AND 100),
  stage_note          text NOT NULL DEFAULT '',
  error               text,
  failure_kind        text CHECK (failure_kind IN ('error', 'stopped')),
  error_log           text NOT NULL DEFAULT '',
  output_blob_key     text,
  output_filename     text,
  output_content_type text,
  output_size_bytes   bigint,
  output_duration_ms  integer,
  library_item_id     uuid REFERENCES library_items(id) ON DELETE SET NULL,
  created_at          timestamptz NOT NULL DEFAULT now(),
  updated_at          timestamptz NOT NULL DEFAULT now(),
  finished_at         timestamptz,
  CONSTRAINT chat_audio_jobs_error CHECK ((error IS NOT NULL) = (state = 'failed') AND (failure_kind IS NOT NULL) = (state = 'failed')),
  CONSTRAINT chat_audio_jobs_output CHECK (state <> 'done' OR output_blob_key IS NOT NULL)
);
CREATE INDEX chat_audio_jobs_chat ON chat_audio_jobs (chat_id, created_at);
CREATE INDEX chat_audio_jobs_running ON chat_audio_jobs (run_id) WHERE state = 'running';
CREATE INDEX chat_audio_jobs_detached ON chat_audio_jobs (created_at) WHERE chat_id IS NULL;
CREATE INDEX chat_audio_jobs_blob ON chat_audio_jobs (output_blob_key);

CREATE TRIGGER realtime_change_ins AFTER INSERT ON chat_audio_jobs REFERENCING NEW TABLE AS new_rows FOR EACH STATEMENT EXECUTE FUNCTION notify_chat_change('audio.updated', 'chat_id');
CREATE TRIGGER realtime_change_upd AFTER UPDATE ON chat_audio_jobs REFERENCING NEW TABLE AS new_rows FOR EACH STATEMENT EXECUTE FUNCTION notify_chat_change('audio.updated', 'chat_id');

-- +goose Down
DROP TABLE chat_audio_jobs;

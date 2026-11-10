-- +goose Up
-- Videos a coding chat renders with code during its turn. The agent moves a
-- job through its stages with the video tools; the renderer it wrote drops
-- progress and preview frames into .lazyagents/videos/<id>/, which the runner
-- copies here while the turn runs. A finished video is kept in the Library.
CREATE TABLE chat_video_jobs (
  id                 uuid PRIMARY KEY DEFAULT uuid_v7(),
  workspace_id       uuid NOT NULL REFERENCES workspaces(id),
  user_id            uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  -- Detached (NULL) when the chat is deleted; the sweep then removes the job
  -- and the objects only it holds.
  chat_id            uuid REFERENCES chat_sessions(id) ON DELETE SET NULL,
  run_id             uuid REFERENCES runs(id) ON DELETE SET NULL,
  turn_id            uuid REFERENCES run_turns(id) ON DELETE SET NULL,
  -- The reply the job belongs to, once the turn's reply is stored.
  message_id         uuid REFERENCES chat_messages(id) ON DELETE SET NULL,
  persona_id         uuid REFERENCES personas(id) ON DELETE SET NULL,
  title              text NOT NULL CHECK (length(title) BETWEEN 1 AND 200),
  width              integer NOT NULL CHECK (width BETWEEN 16 AND 7680),
  height             integer NOT NULL CHECK (height BETWEEN 16 AND 4320),
  fps                integer NOT NULL CHECK (fps BETWEEN 1 AND 240),
  duration_ms        integer NOT NULL CHECK (duration_ms BETWEEN 100 AND 3600000),
  -- The renderer, workspace-relative, and what the chat shows of it.
  script_path        text NOT NULL DEFAULT '',
  script_lines       integer NOT NULL DEFAULT 0,
  script_tail        text NOT NULL DEFAULT '',
  stage              text NOT NULL DEFAULT 'write' CHECK (stage IN ('write', 'stills', 'render', 'encode', 'save')),
  state              text NOT NULL DEFAULT 'running' CHECK (state IN ('running', 'done', 'failed')),
  -- [{"label": "...", "passed": true|false|null}], what the agent checked on the stills.
  checks             jsonb NOT NULL DEFAULT '[]',
  frames_done        integer NOT NULL DEFAULT 0 CHECK (frames_done >= 0),
  frames_total       integer NOT NULL DEFAULT 0 CHECK (frames_total >= 0),
  -- A short line the renderer reports with its stage, e.g. the encoder settings.
  stage_note         text NOT NULL DEFAULT '',
  render_started_at  timestamptz,
  -- Why a failed job failed (stopped: the person stopped the reply), and the
  -- log tail the agent attached.
  error              text,
  failure_kind       text CHECK (failure_kind IN ('error', 'stopped')),
  error_log          text NOT NULL DEFAULT '',
  output_blob_key    text,
  output_filename    text,
  output_content_type text,
  output_size_bytes  bigint,
  output_codec       text NOT NULL DEFAULT '',
  output_duration_ms integer,
  library_item_id    uuid REFERENCES library_items(id) ON DELETE SET NULL,
  created_at         timestamptz NOT NULL DEFAULT now(),
  updated_at         timestamptz NOT NULL DEFAULT now(),
  finished_at        timestamptz,
  CONSTRAINT chat_video_jobs_error CHECK ((error IS NOT NULL) = (state = 'failed') AND (failure_kind IS NOT NULL) = (state = 'failed')),
  CONSTRAINT chat_video_jobs_output CHECK (state <> 'done' OR output_blob_key IS NOT NULL)
);
CREATE INDEX chat_video_jobs_chat ON chat_video_jobs (chat_id, created_at);
CREATE INDEX chat_video_jobs_running ON chat_video_jobs (run_id) WHERE state = 'running';
CREATE INDEX chat_video_jobs_detached ON chat_video_jobs (created_at) WHERE chat_id IS NULL;
CREATE INDEX chat_video_jobs_blob ON chat_video_jobs (output_blob_key);

-- Pictures of a job in progress: the stills it checked, the filmstrip cells
-- filled as the render advances, the latest rendered frame and the poster.
CREATE TABLE chat_video_frames (
  job_id       uuid NOT NULL REFERENCES chat_video_jobs(id) ON DELETE CASCADE,
  kind         text NOT NULL CHECK (kind IN ('still', 'strip', 'preview', 'poster')),
  -- A still's frame number, a strip cell (0–29), 0 for preview and poster.
  idx          integer NOT NULL CHECK (idx >= 0),
  blob_key     text NOT NULL,
  content_type text NOT NULL,
  size_bytes   bigint NOT NULL CHECK (size_bytes > 0),
  -- The source file's size and modification time, so an unchanged file isn't copied again.
  source_stamp text NOT NULL,
  updated_at   timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (job_id, kind, idx)
);

ALTER TABLE library_items DROP CONSTRAINT library_items_kind_check;
ALTER TABLE library_items ADD CONSTRAINT library_items_kind_check CHECK (kind IN ('image', 'diagram', 'link', 'document', 'video'));
ALTER TABLE library_items
  ADD COLUMN duration_ms integer,
  ADD COLUMN width       integer,
  ADD COLUMN height      integer,
  -- A video's poster image, owned by the item (deleted with it).
  ADD COLUMN poster_key  text;

CREATE TRIGGER realtime_change_ins AFTER INSERT ON chat_video_jobs REFERENCING NEW TABLE AS new_rows FOR EACH STATEMENT EXECUTE FUNCTION notify_chat_change('video.updated', 'chat_id');
CREATE TRIGGER realtime_change_upd AFTER UPDATE ON chat_video_jobs REFERENCING NEW TABLE AS new_rows FOR EACH STATEMENT EXECUTE FUNCTION notify_chat_change('video.updated', 'chat_id');
CREATE TRIGGER realtime_change_ins AFTER INSERT ON chat_video_frames FOR EACH STATEMENT EXECUTE FUNCTION notify_change('video.updated');
CREATE TRIGGER realtime_change_upd AFTER UPDATE ON chat_video_frames FOR EACH STATEMENT EXECUTE FUNCTION notify_change('video.updated');

-- +goose Down
DROP TABLE chat_video_frames;
DROP TABLE chat_video_jobs;
DELETE FROM library_items WHERE kind = 'video';
ALTER TABLE library_items DROP COLUMN duration_ms, DROP COLUMN width, DROP COLUMN height, DROP COLUMN poster_key;
ALTER TABLE library_items DROP CONSTRAINT library_items_kind_check;
ALTER TABLE library_items ADD CONSTRAINT library_items_kind_check CHECK (kind IN ('image', 'diagram', 'link', 'document'));

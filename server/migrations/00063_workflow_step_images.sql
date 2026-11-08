-- +goose Up
-- Images an agent step generated, one row per image, against the exact attempt
-- that made them: a retry's images never mix with an earlier attempt's. The
-- bytes live in the attachment store; outputs share the object by blob_key.
CREATE TABLE workflow_step_images (
  id           uuid PRIMARY KEY DEFAULT uuid_v7(),
  step_id      uuid NOT NULL REFERENCES workflow_steps(id) ON DELETE CASCADE,
  run_id       uuid NOT NULL REFERENCES workflow_runs(id) ON DELETE CASCADE,
  filename     text NOT NULL CHECK (length(filename) BETWEEN 1 AND 255),
  content_type text NOT NULL,
  size_bytes   bigint NOT NULL CHECK (size_bytes > 0),
  blob_key     text NOT NULL,
  created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX workflow_step_images_step ON workflow_step_images (step_id);
CREATE INDEX workflow_step_images_run ON workflow_step_images (run_id, created_at);
CREATE INDEX workflow_step_images_blob ON workflow_step_images (blob_key);

-- Native image tool calls this attempt has made, against the per-step limit.
ALTER TABLE workflow_steps ADD COLUMN image_calls integer NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE workflow_steps DROP COLUMN image_calls;
DROP TABLE workflow_step_images;

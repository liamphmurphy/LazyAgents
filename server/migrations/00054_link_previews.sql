-- +goose Up
-- What a recommended link looks like, fetched once per workspace and shown on
-- its card. An empty title records a fetch that found nothing.
CREATE TABLE link_previews (
  id           uuid PRIMARY KEY DEFAULT uuid_v7(),
  workspace_id uuid NOT NULL REFERENCES workspaces(id),
  url          text NOT NULL,
  kind         text NOT NULL CHECK (kind IN ('site', 'video')),
  title        text NOT NULL DEFAULT '',
  description  text NOT NULL DEFAULT '',
  site         text NOT NULL DEFAULT '',
  video_id     text NOT NULL DEFAULT '',
  blob_key     text NOT NULL DEFAULT '',
  image_type   text NOT NULL DEFAULT '',
  fetched_at   timestamptz NOT NULL DEFAULT now(),
  UNIQUE (workspace_id, url)
);

-- +goose Down
DROP TABLE link_previews;

-- +goose Up
-- Canvases: documents and SVG mockups a person and agent work on beside a chat.
-- Every change is a new immutable version; the highest seq is the current one.
CREATE TABLE chat_canvases (
  id         uuid PRIMARY KEY DEFAULT uuid_v7(),
  session_id uuid NOT NULL REFERENCES chat_sessions(id) ON DELETE CASCADE,
  kind       text NOT NULL CHECK (kind IN ('text', 'svg')),
  title      text NOT NULL CHECK (length(title) BETWEEN 1 AND 200),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX chat_canvases_session ON chat_canvases (session_id, created_at);

CREATE TABLE chat_canvas_versions (
  id          uuid PRIMARY KEY DEFAULT uuid_v7(),
  canvas_id   uuid NOT NULL REFERENCES chat_canvases(id) ON DELETE CASCADE,
  seq         integer NOT NULL,
  body        text NOT NULL,
  note        text NOT NULL DEFAULT '',
  author_type text NOT NULL CHECK (author_type IN ('user', 'agent')),
  author_id   uuid NOT NULL,
  created_at  timestamptz NOT NULL DEFAULT now(),
  UNIQUE (canvas_id, seq)
);

-- +goose Down
DROP TABLE chat_canvas_versions;
DROP TABLE chat_canvases;

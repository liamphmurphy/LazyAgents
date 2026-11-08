-- +goose Up
-- Comments on a canvas: on a passage of text or an SVG element, carried forward
-- as new versions are written. Replies have a parent and no anchor of their own.
CREATE TABLE chat_canvas_comments (
  id                 uuid PRIMARY KEY DEFAULT uuid_v7(),
  canvas_id          uuid NOT NULL REFERENCES chat_canvases(id) ON DELETE CASCADE,
  parent_id          uuid REFERENCES chat_canvas_comments(id) ON DELETE CASCADE,
  -- The version the anchor currently points into.
  seq                integer NOT NULL,
  author_type        text NOT NULL CHECK (author_type IN ('user', 'agent')),
  author_id          uuid NOT NULL,
  body               text NOT NULL CHECK (length(body) BETWEEN 1 AND 4000),
  anchor             jsonb NOT NULL DEFAULT '{}',
  target_lost        boolean NOT NULL DEFAULT false,
  status             text NOT NULL CHECK (status IN ('draft', 'open', 'addressed', 'resolved')),
  -- Replacement text an agent proposes for the anchored passage.
  suggestion         text,
  sent_in_message_id uuid REFERENCES chat_messages(id) ON DELETE SET NULL,
  created_at         timestamptz NOT NULL DEFAULT now(),
  updated_at         timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX chat_canvas_comments_canvas ON chat_canvas_comments (canvas_id, created_at);

ALTER TABLE chat_canvases ADD COLUMN promoted_artifact_id uuid REFERENCES artifacts(id) ON DELETE SET NULL;

-- +goose Down
ALTER TABLE chat_canvases DROP COLUMN promoted_artifact_id;
DROP TABLE chat_canvas_comments;

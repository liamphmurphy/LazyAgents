-- +goose Up
-- Slides (ADR: Slides deck model and export). A deck is a canvas of kind
-- 'deck' whose body is the deck JSON; its images live in the blob store and
-- are listed here, keyed by the asset id the deck JSON refers to.
ALTER TABLE chat_canvases DROP CONSTRAINT chat_canvases_kind_check;
ALTER TABLE chat_canvases ADD CONSTRAINT chat_canvases_kind_check CHECK (kind IN ('text', 'svg', 'deck'));
ALTER TABLE chat_spaces DROP CONSTRAINT chat_spaces_app_check;
ALTER TABLE chat_spaces ADD CONSTRAINT chat_spaces_app_check CHECK (app IN ('finance', 'docs', 'slides'));

CREATE TABLE deck_assets (
  canvas_id    uuid NOT NULL REFERENCES chat_canvases(id) ON DELETE CASCADE,
  id           text NOT NULL CHECK (id ~ '^a_[a-z0-9_]{1,40}$'),
  workspace_id uuid NOT NULL,
  user_id      uuid NOT NULL,
  blob_key     text NOT NULL,
  mime         text NOT NULL CHECK (mime IN ('image/png', 'image/jpeg')),
  width        integer NOT NULL CHECK (width > 0),
  height       integer NOT NULL CHECK (height > 0),
  size_bytes   bigint NOT NULL CHECK (size_bytes > 0),
  created_at   timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (canvas_id, id)
);
CREATE INDEX deck_assets_owner ON deck_assets (workspace_id, user_id);
CREATE INDEX deck_assets_blob ON deck_assets (blob_key);

-- +goose Down
-- Lossy by design: decks, their chats and their spaces go. Their image
-- blobs stay in storage (nothing references them any more).
DROP TABLE deck_assets;
DELETE FROM chat_sessions WHERE space_id IN (SELECT id FROM chat_spaces WHERE app = 'slides');
DELETE FROM chat_canvases WHERE kind = 'deck';
DELETE FROM chat_spaces WHERE app = 'slides';
ALTER TABLE chat_spaces DROP CONSTRAINT chat_spaces_app_check;
ALTER TABLE chat_spaces ADD CONSTRAINT chat_spaces_app_check CHECK (app IN ('finance', 'docs'));
ALTER TABLE chat_canvases DROP CONSTRAINT chat_canvases_kind_check;
ALTER TABLE chat_canvases ADD CONSTRAINT chat_canvases_kind_check CHECK (kind IN ('text', 'svg'));

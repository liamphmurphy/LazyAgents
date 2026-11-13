-- +goose Up
-- Images can be made by Codex or Antigravity. A workspace chooses an engine
-- ('auto' picks the first connected one, Codex first); a persona may override it
-- ('' inherits the workspace's); each chat image job records the harness that
-- ran it, since its model id means nothing without one.
ALTER TABLE workspaces ADD COLUMN image_engine text NOT NULL DEFAULT 'auto'
  CHECK (image_engine IN ('auto', 'codex', 'antigravity'));
ALTER TABLE personas ADD COLUMN image_engine text NOT NULL DEFAULT ''
  CHECK (image_engine IN ('', 'auto', 'codex', 'antigravity'));
ALTER TABLE chat_image_jobs ADD COLUMN harness text NOT NULL DEFAULT 'codex'
  CHECK (harness IN ('codex', 'antigravity'));

-- +goose Down
ALTER TABLE chat_image_jobs DROP COLUMN harness;
ALTER TABLE personas DROP COLUMN image_engine;
ALTER TABLE workspaces DROP COLUMN image_engine;

-- +goose Up
-- Designs (docs/design/ADR-designs.md §3.6). A design is a canvas of kind
-- 'design' whose body is the design JSON (schema 1). Its images, persona
-- proposals, per-design switches and ticket links live beside it.
ALTER TABLE chat_canvases DROP CONSTRAINT chat_canvases_kind_check;
ALTER TABLE chat_canvases ADD CONSTRAINT chat_canvases_kind_check CHECK (kind IN ('text', 'svg', 'deck', 'design'));
ALTER TABLE chat_spaces DROP CONSTRAINT chat_spaces_app_check;
ALTER TABLE chat_spaces ADD CONSTRAINT chat_spaces_app_check CHECK (app IN ('finance', 'docs', 'slides', 'designs'));

-- Switches outside the body, so toggling one isn't a version and no patch can
-- change it. A design without a row has the defaults.
CREATE TABLE design_settings (
  canvas_id      uuid PRIMARY KEY REFERENCES chat_canvases(id) ON DELETE CASCADE,
  direct_edit    boolean NOT NULL DEFAULT false,
  persona_paused boolean NOT NULL DEFAULT false,
  project_id     uuid REFERENCES projects(id) ON DELETE SET NULL,
  updated_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX design_settings_project ON design_settings (project_id) WHERE project_id IS NOT NULL;

-- Same shape as deck_assets; blobs live under designs/{workspace}/{user}/{sha256}.
CREATE TABLE design_assets (
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
CREATE INDEX design_assets_owner ON design_assets (workspace_id, user_id);
CREATE INDEX design_assets_blob ON design_assets (blob_key);

-- Released design images go into the Slides sweep queue; the sweep deletes a
-- key only when neither deck_assets nor design_assets uses it.
CREATE TRIGGER design_assets_queue_blob AFTER DELETE ON design_assets
  FOR EACH ROW EXECUTE FUNCTION deck_assets_queue_blob();

-- A persona's patch held for review (or, with direct edit on, recorded as
-- applied). decisions is [{i, state: accepted|rejected|stale, reason?}].
CREATE TABLE design_proposals (
  id          uuid PRIMARY KEY DEFAULT uuid_v7(),
  canvas_id   uuid NOT NULL REFERENCES chat_canvases(id) ON DELETE CASCADE,
  chat_id     uuid REFERENCES chat_sessions(id) ON DELETE SET NULL,
  run_id      uuid,
  agent_id    uuid NOT NULL,
  base_seq    integer NOT NULL,
  summary     text NOT NULL CHECK (length(summary) BETWEEN 1 AND 500),
  ops         jsonb NOT NULL,
  status      text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'resolved', 'applied', 'withdrawn')),
  decisions   jsonb NOT NULL DEFAULT '[]',
  result_seq  integer,
  created_at  timestamptz NOT NULL DEFAULT now(),
  resolved_at timestamptz
);
CREATE INDEX design_proposals_canvas ON design_proposals (canvas_id, created_at);
CREATE INDEX design_proposals_pending ON design_proposals (canvas_id) WHERE status = 'pending';

-- An index of the body's linked_tickets, rewritten with every version.
CREATE TABLE design_frame_links (
  canvas_id uuid NOT NULL REFERENCES chat_canvases(id) ON DELETE CASCADE,
  node_id   text NOT NULL,
  ticket_id uuid NOT NULL REFERENCES tickets(id) ON DELETE CASCADE,
  PRIMARY KEY (canvas_id, node_id, ticket_id)
);
CREATE INDEX design_frame_links_ticket ON design_frame_links (ticket_id);

CREATE TRIGGER realtime_change_ins AFTER INSERT ON design_proposals REFERENCING NEW TABLE AS new_rows FOR EACH STATEMENT EXECUTE FUNCTION notify_entity_change('design.updated', 'canvas_id');
CREATE TRIGGER realtime_change_upd AFTER UPDATE ON design_proposals REFERENCING NEW TABLE AS new_rows FOR EACH STATEMENT EXECUTE FUNCTION notify_entity_change('design.updated', 'canvas_id');
CREATE TRIGGER realtime_change_del AFTER DELETE ON design_proposals REFERENCING OLD TABLE AS old_rows FOR EACH STATEMENT EXECUTE FUNCTION notify_entity_change('design.updated', 'canvas_id');
CREATE TRIGGER realtime_change_ins AFTER INSERT ON design_settings REFERENCING NEW TABLE AS new_rows FOR EACH STATEMENT EXECUTE FUNCTION notify_entity_change('design.updated', 'canvas_id');
CREATE TRIGGER realtime_change_upd AFTER UPDATE ON design_settings REFERENCING NEW TABLE AS new_rows FOR EACH STATEMENT EXECUTE FUNCTION notify_entity_change('design.updated', 'canvas_id');

-- +goose Down
-- Lossy by design: designs, the chats of the Designs space and the space go.
-- Deleting them deletes their design_assets rows, whose trigger queues each
-- blob key for the sweep before the table drops, so no blob is orphaned.
DELETE FROM chat_sessions WHERE space_id IN (SELECT id FROM chat_spaces WHERE app = 'designs');
DELETE FROM chat_canvases WHERE kind = 'design';
DELETE FROM chat_spaces WHERE app = 'designs';
DROP TABLE design_frame_links;
DROP TABLE design_proposals;
DROP TABLE design_assets;
DROP TABLE design_settings;
ALTER TABLE chat_spaces DROP CONSTRAINT chat_spaces_app_check;
ALTER TABLE chat_spaces ADD CONSTRAINT chat_spaces_app_check CHECK (app IN ('finance', 'docs', 'slides'));
ALTER TABLE chat_canvases DROP CONSTRAINT chat_canvases_kind_check;
ALTER TABLE chat_canvases ADD CONSTRAINT chat_canvases_kind_check CHECK (kind IN ('text', 'svg', 'deck'));

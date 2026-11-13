-- +goose Up
-- Embeds, block references and attachments in pages (LAZYAGENTS-178).
-- knowledge_links gains targets for a block of a page ([[Page#^id]]), an
-- attachment (![[file.png]]), a Designs frame (![[design:<id>#<node>]]) and a
-- Library item (![[library:<id>]]).
ALTER TABLE knowledge_links DROP CONSTRAINT knowledge_links_target_kind_check;
ALTER TABLE knowledge_links ADD CONSTRAINT knowledge_links_target_kind_check CHECK (target_kind IN
  ('artifact', 'section', 'block', 'ticket', 'epic', 'agent', 'attachment', 'design', 'library', 'unresolved'));
CREATE INDEX knowledge_links_attachment ON knowledge_links (target_id) WHERE target_kind = 'attachment';

-- A file uploaded to a page. The page's audience applies to it. Blobs are
-- content-addressed (pages/{workspace}/{sha256}), so identical files share one
-- object. A row nothing embeds is deleted by the runner's sweep after a grace
-- period; deleting a row (or its page) queues the blob in deck_blob_sweep,
-- which deletes it once no deck, design or page attachment uses the key.
CREATE TABLE page_attachments (
  id           uuid PRIMARY KEY DEFAULT uuid_v7(),
  workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  artifact_id  uuid NOT NULL REFERENCES artifacts(id) ON DELETE CASCADE,
  name         text NOT NULL CHECK (length(name) BETWEEN 1 AND 255 AND name !~ '[/\\\[\]#|^]'),
  content_type text NOT NULL CHECK (content_type ~ '^(image|audio|video)/' OR content_type = 'application/pdf'),
  size_bytes   bigint NOT NULL CHECK (size_bytes > 0),
  sha256       text NOT NULL CHECK (sha256 ~ '^[0-9a-f]{64}$'),
  blob_key     text NOT NULL,
  created_by_type text NOT NULL CHECK (created_by_type IN ('user', 'agent')),
  created_by_id   uuid NOT NULL,
  created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX page_attachments_name ON page_attachments (artifact_id, lower(name));
CREATE INDEX page_attachments_workspace_name ON page_attachments (workspace_id, lower(name));
CREATE INDEX page_attachments_blob ON page_attachments (blob_key);

CREATE TRIGGER page_attachments_queue_blob AFTER DELETE ON page_attachments
  FOR EACH ROW EXECUTE FUNCTION deck_assets_queue_blob();

-- +goose Down
-- Lossy: attachments go (their trigger queues each blob for the sweep first),
-- and links to blocks, attachments, designs and Library items become ghosts.
DELETE FROM page_attachments;
DROP TABLE page_attachments;
DROP INDEX knowledge_links_attachment;
UPDATE knowledge_links SET target_kind = 'section' WHERE target_kind = 'block';
UPDATE knowledge_links SET target_kind = 'unresolved', target_id = NULL WHERE target_kind IN ('attachment', 'design', 'library');
ALTER TABLE knowledge_links DROP CONSTRAINT knowledge_links_target_kind_check;
ALTER TABLE knowledge_links ADD CONSTRAINT knowledge_links_target_kind_check CHECK (target_kind IN
  ('artifact', 'section', 'ticket', 'epic', 'agent', 'unresolved'));

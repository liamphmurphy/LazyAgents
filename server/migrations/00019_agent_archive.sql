-- +goose Up
-- Deleting an agent archives it: runs, chats and finished tickets keep pointing
-- at the row, while lists, assignment and delegation skip it. The handle is
-- only unique among live agents so it can be reused.
ALTER TABLE agents ADD COLUMN archived_at timestamptz;
ALTER TABLE agents DROP CONSTRAINT agents_workspace_id_handle_key;
CREATE UNIQUE INDEX agents_live_handle ON agents (workspace_id, handle) WHERE archived_at IS NULL;

-- +goose Down
DROP INDEX agents_live_handle;
ALTER TABLE agents ADD CONSTRAINT agents_workspace_id_handle_key UNIQUE (workspace_id, handle);
ALTER TABLE agents DROP COLUMN archived_at;

-- +goose Up
CREATE TABLE knowledge_asset_deletions (
 blob_key text PRIMARY KEY,created_at timestamptz NOT NULL DEFAULT now()
);
ALTER TABLE knowledge_imports ADD COLUMN archive_deleted boolean NOT NULL DEFAULT false;
-- +goose Down
ALTER TABLE knowledge_imports DROP COLUMN archive_deleted;
DROP TABLE knowledge_asset_deletions;

-- +goose Up
CREATE TABLE knowledge_assets (
 id uuid PRIMARY KEY DEFAULT uuid_v7(), workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
 owner_user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 content_hash text NOT NULL, blob_key text NOT NULL, filename text NOT NULL, content_type text NOT NULL,
 size_bytes bigint NOT NULL, created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(workspace_id,owner_user_id,content_hash)
);
CREATE TABLE page_asset_refs (
 version_id uuid NOT NULL REFERENCES artifact_versions(id) ON DELETE CASCADE,
 asset_id uuid NOT NULL REFERENCES knowledge_assets(id) ON DELETE CASCADE,
 PRIMARY KEY(version_id,asset_id)
);
-- +goose Down
DROP TABLE page_asset_refs;
DROP TABLE knowledge_assets;

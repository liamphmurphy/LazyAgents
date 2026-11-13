-- +goose Up
ALTER TABLE artifact_versions ADD COLUMN properties jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(properties) = 'object');
ALTER TABLE artifact_aliases ADD COLUMN kind text NOT NULL DEFAULT 'moved' CHECK (kind IN ('moved', 'alias'));
CREATE TABLE page_tags (
    artifact_id uuid NOT NULL REFERENCES artifacts(id) ON DELETE CASCADE,
    tag text NOT NULL,
    PRIMARY KEY (artifact_id, tag)
);
CREATE INDEX page_tags_tag_idx ON page_tags (tag, artifact_id);

-- +goose Down
DROP TABLE page_tags;
DELETE FROM artifact_aliases WHERE kind = 'alias';
ALTER TABLE artifact_aliases DROP COLUMN kind;
ALTER TABLE artifact_versions DROP COLUMN properties;

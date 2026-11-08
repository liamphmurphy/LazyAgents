-- name: GetLinkPreview :one
SELECT * FROM link_previews WHERE workspace_id = $1 AND url = $2;

-- name: GetLinkPreviewByID :one
SELECT * FROM link_previews WHERE id = $1 AND workspace_id = $2;

-- name: UpsertLinkPreview :one
-- Two people opening the same reply land on one row.
INSERT INTO link_previews (workspace_id, url, kind, title, description, site, video_id, blob_key, image_type)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (workspace_id, url) DO UPDATE SET kind = EXCLUDED.kind, title = EXCLUDED.title, description = EXCLUDED.description,
  site = EXCLUDED.site, video_id = EXCLUDED.video_id, blob_key = EXCLUDED.blob_key, image_type = EXCLUDED.image_type, fetched_at = now()
RETURNING *;

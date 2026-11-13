-- name: PageBlobInUse :one
-- Whether a page attachment already stores this content; call under LockDeckBlob.
SELECT EXISTS (SELECT 1 FROM page_attachments WHERE blob_key = @blob_key)::boolean;

-- name: GetPageAttachmentByName :one
-- The attachment of this name on a page the caller has already found visible.
SELECT * FROM page_attachments WHERE artifact_id = @artifact_id AND lower(name) = lower(@name);

-- name: InsertPageAttachment :one
INSERT INTO page_attachments (workspace_id, artifact_id, name, content_type, size_bytes, sha256, blob_key, created_by_type, created_by_id)
VALUES (@workspace_id, @artifact_id, @name, @content_type, @size_bytes, @sha256, @blob_key, @created_by_type, @created_by_id)
RETURNING *;

-- name: DeletePageAttachment :exec
DELETE FROM page_attachments WHERE id = @id;

-- name: GetVisiblePageAttachment :one
-- An attachment whose page the viewer may see.
SELECT pa.* FROM page_attachments pa JOIN artifacts a ON a.id = pa.artifact_id
WHERE pa.id = @id AND pa.workspace_id = @workspace_id AND artifact_visible(a.audience, a.owner_user_id, sqlc.narg(viewer)::uuid);

-- name: FindVisiblePageAttachment :one
-- ![[name]]: an attachment with this file name on a page the viewer may see,
-- preferring the page the link is written on, then the oldest.
SELECT pa.* FROM page_attachments pa JOIN artifacts a ON a.id = pa.artifact_id
WHERE pa.workspace_id = @workspace_id AND lower(pa.name) = lower(@name)
  AND artifact_visible(a.audience, a.owner_user_id, sqlc.narg(viewer)::uuid)
ORDER BY (pa.artifact_id = sqlc.narg(from_page)::uuid) IS TRUE DESC, pa.created_at, pa.id
LIMIT 1;

-- name: ListVisiblePageAttachments :many
SELECT pa.* FROM page_attachments pa JOIN artifacts a ON a.id = pa.artifact_id
WHERE pa.artifact_id = @artifact_id AND pa.workspace_id = @workspace_id
  AND artifact_visible(a.audience, a.owner_user_id, sqlc.narg(viewer)::uuid)
ORDER BY lower(pa.name);

-- name: SweepUnusedPageAttachments :many
-- Deletes up to row_limit attachments created before the cutoff that no link
-- embeds or names; their trigger queues each blob for the blob sweep.
DELETE FROM page_attachments pa WHERE pa.id IN (
  SELECT u.id FROM page_attachments u
  WHERE u.created_at < @before
    AND NOT EXISTS (SELECT 1 FROM knowledge_links l WHERE l.target_kind = 'attachment' AND l.target_id = u.id)
  ORDER BY u.created_at LIMIT @row_limit
) RETURNING pa.id;

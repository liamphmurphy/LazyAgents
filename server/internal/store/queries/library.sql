-- name: CreateLibraryItem :one
INSERT INTO library_items (workspace_id, user_id, kind, title, filename, content_type, size_bytes, blob_key, chat_id, message_id, persona_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11) RETURNING *;

-- name: ListLibraryItems :many
-- The caller's own items, newest first, with where each came from (the chat's
-- title and the persona's name, while they still exist).
SELECT l.*, c.title AS chat_title, p.name AS persona_name, pr.key AS project_key
FROM library_items l
LEFT JOIN chat_sessions c ON c.id = l.chat_id
LEFT JOIN personas p ON p.id = l.persona_id
LEFT JOIN projects pr ON pr.id = c.project_id
WHERE l.workspace_id = sqlc.arg(workspace_id) AND l.user_id = sqlc.arg(user_id)
  AND (sqlc.narg(kind)::text IS NULL OR l.kind = sqlc.narg(kind))
ORDER BY l.created_at DESC, l.id DESC
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: GetLibraryItem :one
SELECT * FROM library_items WHERE id = $1 AND workspace_id = $2 AND user_id = $3;

-- name: DeleteLibraryItem :one
-- Removes one of the caller's items. orphaned says no chat attachment or
-- other item still uses its object, so the caller may delete it.
WITH d AS (
  DELETE FROM library_items WHERE library_items.id = sqlc.arg(id) AND library_items.workspace_id = sqlc.arg(workspace_id) AND library_items.user_id = sqlc.arg(user_id)
  RETURNING blob_key
)
SELECT d.blob_key,
  (NOT EXISTS (SELECT 1 FROM chat_attachments a WHERE a.blob_key = d.blob_key)
   AND NOT EXISTS (SELECT 1 FROM library_items o WHERE o.blob_key = d.blob_key AND o.id <> sqlc.arg(id)))::boolean AS orphaned
FROM d;

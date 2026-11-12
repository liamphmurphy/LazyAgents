-- name: ListUserDecks :many
-- The person's decks with their current bodies, most recently changed first.
SELECT cv.id, cv.title, cv.updated_at, c.id AS chat_id, v.body
FROM chat_canvases cv
JOIN chat_sessions c ON c.id = cv.session_id
JOIN LATERAL (SELECT body FROM chat_canvas_versions WHERE canvas_id = cv.id ORDER BY seq DESC LIMIT 1) v ON true
WHERE c.workspace_id = @workspace_id AND c.created_by = @user_id AND cv.kind = 'deck'
ORDER BY cv.updated_at DESC, cv.id DESC
LIMIT 200;

-- name: RenameCanvas :exec
UPDATE chat_canvases SET title = @title, updated_at = now() WHERE id = @id;

-- name: CreateDeckAsset :one
INSERT INTO deck_assets (canvas_id, id, workspace_id, user_id, blob_key, mime, width, height, size_bytes)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING *;

-- name: GetDeckAsset :one
SELECT * FROM deck_assets WHERE canvas_id = $1 AND id = $2;

-- name: ListDeckAssets :many
SELECT * FROM deck_assets WHERE canvas_id = $1 ORDER BY created_at;

-- name: CopyDeckAssets :exec
INSERT INTO deck_assets (canvas_id, id, workspace_id, user_id, blob_key, mime, width, height, size_bytes)
SELECT @to_canvas::uuid, id, workspace_id, user_id, blob_key, mime, width, height, size_bytes FROM deck_assets WHERE canvas_id = @from_canvas::uuid;

-- name: DeckAssetBytes :one
-- Each stored image once per person, for the shared Library quota.
SELECT COALESCE(sum(size_bytes), 0)::bigint AS total FROM (
  SELECT DISTINCT blob_key, size_bytes FROM deck_assets WHERE workspace_id = $1 AND user_id = $2
) a;

-- name: DeleteDeckAsset :exec
DELETE FROM deck_assets WHERE canvas_id = $1 AND id = $2;

-- name: UserHasDeckBlob :one
-- Whether this person already stores this image (in any deck), so it doesn't count twice.
SELECT EXISTS (SELECT 1 FROM deck_assets WHERE workspace_id = $1 AND user_id = $2 AND blob_key = $3);

-- name: LockDeckBlob :exec
-- Serialises taking a reference to a deck image blob with the sweep deleting it.
SELECT pg_advisory_xact_lock(hashtextextended('deck-blob:' || @blob_key::text, 0));

-- name: DueDeckBlobSweeps :many
-- Released image blobs queued before the cutoff, oldest first.
SELECT blob_key FROM deck_blob_sweep WHERE queued_at < @before ORDER BY queued_at, blob_key LIMIT @row_limit;

-- name: TakeDeckBlobSweep :one
-- Dequeues a key that's still due, saying whether any deck or design still uses it.
DELETE FROM deck_blob_sweep s WHERE s.blob_key = @blob_key AND s.queued_at < @before
RETURNING (NOT EXISTS (SELECT 1 FROM deck_assets a WHERE a.blob_key = s.blob_key)
  AND NOT EXISTS (SELECT 1 FROM design_assets a WHERE a.blob_key = s.blob_key))::boolean AS unused;

-- name: RequeueDeckBlobSweep :exec
-- Sends a key whose blob couldn't be deleted to the back of the queue.
UPDATE deck_blob_sweep SET queued_at = now() WHERE blob_key = @blob_key;

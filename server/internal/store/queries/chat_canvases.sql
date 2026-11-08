-- name: CreateChatCanvas :one
INSERT INTO chat_canvases (session_id, kind, title) VALUES ($1, $2, $3) RETURNING *;

-- name: LockChatCanvas :one
SELECT * FROM chat_canvases WHERE id = $1 AND session_id = $2 FOR UPDATE;

-- name: GetChatCanvas :one
SELECT * FROM chat_canvases WHERE id = $1 AND session_id = $2;

-- name: TouchChatCanvas :exec
UPDATE chat_canvases SET updated_at = now() WHERE id = $1;

-- name: ListChatCanvases :many
-- The chat's canvases, oldest first, each with its current version number.
SELECT c.*, (SELECT max(v.seq) FROM chat_canvas_versions v WHERE v.canvas_id = c.id)::int AS seq
FROM chat_canvases c WHERE c.session_id = $1 ORDER BY c.created_at, c.id;

-- name: CreateChatCanvasVersion :one
INSERT INTO chat_canvas_versions (canvas_id, seq, body, note, author_type, author_id)
VALUES ($1, $2, $3, $4, $5, $6) RETURNING *;

-- name: LatestChatCanvasVersion :one
SELECT * FROM chat_canvas_versions WHERE canvas_id = $1 ORDER BY seq DESC LIMIT 1;

-- name: GetChatCanvasVersion :one
SELECT * FROM chat_canvas_versions WHERE canvas_id = $1 AND seq = $2;

-- name: ListChatCanvasVersions :many
-- Newest first, without bodies.
SELECT id, seq, note, author_type, created_at FROM chat_canvas_versions WHERE canvas_id = $1 ORDER BY seq DESC;

-- name: SetChatCanvasPromoted :exec
UPDATE chat_canvases SET promoted_artifact_id = $2 WHERE id = $1;

-- name: CreateChatCanvasComment :one
INSERT INTO chat_canvas_comments (canvas_id, parent_id, seq, author_type, author_id, body, anchor, status, suggestion)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING *;

-- name: GetChatCanvasComment :one
SELECT * FROM chat_canvas_comments WHERE id = $1 AND canvas_id = $2;

-- name: ListChatCanvasComments :many
SELECT * FROM chat_canvas_comments WHERE canvas_id = $1 ORDER BY created_at, id;

-- name: ReanchorChatCanvasComment :exec
UPDATE chat_canvas_comments SET anchor = $2, seq = $3, target_lost = $4, updated_at = now() WHERE id = $1;

-- name: SetChatCanvasCommentStatus :one
UPDATE chat_canvas_comments SET status = $2, updated_at = now() WHERE id = $1 RETURNING *;

-- name: ClearChatCanvasCommentSuggestion :exec
UPDATE chat_canvas_comments SET suggestion = NULL, updated_at = now() WHERE id = $1;

-- name: DeleteChatCanvasDraftComment :execrows
DELETE FROM chat_canvas_comments WHERE id = $1 AND canvas_id = $2 AND status = 'draft';

-- name: OpenChatCanvasDraftComments :many
-- The ids of the drafts among these that are now open.
UPDATE chat_canvas_comments SET status = 'open', updated_at = now()
WHERE canvas_id = sqlc.arg(canvas_id) AND id = ANY(sqlc.arg(ids)::uuid[]) AND status = 'draft' RETURNING id;

-- name: ReturnChatCanvasCommentsToDraft :exec
UPDATE chat_canvas_comments SET status = 'draft', updated_at = now() WHERE id = ANY(sqlc.arg(ids)::uuid[]);

-- name: SetChatCanvasCommentsSent :exec
UPDATE chat_canvas_comments SET sent_in_message_id = sqlc.arg(message_id) WHERE id = ANY(sqlc.arg(ids)::uuid[]);

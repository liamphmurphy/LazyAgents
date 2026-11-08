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

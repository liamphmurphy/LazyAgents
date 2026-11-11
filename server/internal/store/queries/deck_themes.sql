-- name: ListDeckThemes :many
SELECT * FROM deck_themes WHERE workspace_id = $1 AND user_id = $2 ORDER BY lower(name), id;

-- name: CountDeckThemes :one
SELECT count(*) FROM deck_themes WHERE workspace_id = $1 AND user_id = $2;

-- name: CreateDeckTheme :one
INSERT INTO deck_themes (workspace_id, user_id, name, fonts, colors)
VALUES ($1, $2, $3, $4, $5) RETURNING *;

-- name: GetDeckTheme :one
SELECT * FROM deck_themes WHERE id = $1 AND workspace_id = $2 AND user_id = $3;

-- name: UpdateDeckTheme :one
UPDATE deck_themes SET name = @name, fonts = @fonts, colors = @colors, updated_at = now()
WHERE id = @id AND workspace_id = @workspace_id AND user_id = @user_id RETURNING *;

-- name: DeleteDeckTheme :execrows
DELETE FROM deck_themes WHERE id = $1 AND workspace_id = $2 AND user_id = $3;

-- name: ChatOwnerDeckThemes :many
-- The saved themes of the person a chat belongs to, for its persona's set_theme.
SELECT t.* FROM deck_themes t
JOIN chat_sessions c ON c.workspace_id = t.workspace_id AND c.created_by = t.user_id
WHERE c.id = @chat_id ORDER BY lower(t.name), t.id;

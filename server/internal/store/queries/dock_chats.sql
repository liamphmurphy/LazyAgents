-- name: AddDockChat :exec
INSERT INTO dock_chats (workspace_id, user_id, scope, chat_id)
SELECT c.workspace_id, c.created_by, sqlc.arg(scope), c.id FROM chat_sessions c
WHERE c.id = sqlc.arg(chat_id) AND c.workspace_id = sqlc.arg(workspace_id) AND c.created_by = sqlc.arg(user_id)
ON CONFLICT (user_id, scope, chat_id) DO UPDATE SET attached_at = now();

-- name: ListDockChats :many
-- A dock's chats, the person's own and not archived, most recently used first.
SELECT c.* FROM dock_chats d JOIN chat_sessions c ON c.id = d.chat_id
WHERE d.workspace_id = sqlc.arg(workspace_id) AND d.user_id = sqlc.arg(user_id) AND d.scope = sqlc.arg(scope)
  AND c.created_by = sqlc.arg(user_id) AND NOT c.archived
ORDER BY COALESCE(c.last_message_at, d.attached_at) DESC, c.id DESC
LIMIT 50;

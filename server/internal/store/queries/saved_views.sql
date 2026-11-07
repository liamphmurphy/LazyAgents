-- name: ListSavedViews :many
SELECT * FROM saved_views WHERE workspace_id=$1 AND user_id=$2 ORDER BY lower(name), id;

-- name: CreateSavedView :one
INSERT INTO saved_views(workspace_id,user_id,name,query) VALUES($1,$2,$3,$4) RETURNING *;

-- name: UpdateSavedView :one
UPDATE saved_views SET name=coalesce(sqlc.narg(name),name), query=coalesce(sqlc.narg(query),query), updated_at=now()
WHERE id=$1 AND workspace_id=$2 AND user_id=$3 RETURNING *;

-- name: DeleteSavedView :execrows
DELETE FROM saved_views WHERE id=$1 AND workspace_id=$2 AND user_id=$3;

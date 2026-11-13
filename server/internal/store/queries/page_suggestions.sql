-- name: SetPagePersonaEdits :exec
UPDATE artifacts SET persona_direct = coalesce(sqlc.narg(direct)::boolean, persona_direct),
 persona_paused = coalesce(sqlc.narg(paused)::boolean, persona_paused) WHERE id = $1 AND workspace_id = $2;

-- name: SetPageVersionChanges :one
UPDATE artifact_versions SET changes = $2, persona_id = $3, source_chat_id = $4, source_run_id = $5,
 review_state = coalesce(sqlc.narg(review_state)::text, review_state) WHERE id = $1 RETURNING *;

-- name: SetPageChat :exec
UPDATE artifacts SET chat_session_id = $2 WHERE id = $1;

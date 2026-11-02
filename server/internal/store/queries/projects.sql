-- name: LockProject :one
SELECT * FROM projects WHERE id=$1 AND workspace_id=$2 FOR UPDATE;

-- name: LockProjectByKey :one
SELECT * FROM projects WHERE workspace_id=$1 AND key=$2 FOR UPDATE;

-- name: UpdateProject :one
UPDATE projects SET name=$2,color=$3,repo_url=$4,default_branch=$5,archived=$6,revision=revision+1 WHERE id=$1 RETURNING *;

-- name: ProjectHasLiveRuns :one
SELECT EXISTS(SELECT 1 FROM runs r LEFT JOIN tickets t ON t.id=r.ticket_id LEFT JOIN planning_sessions p ON p.run_id=r.id
WHERE coalesce(t.project_id,p.project_id)=$1 AND r.state NOT IN ('completed','failed','cancelled'))::boolean;

-- name: CreateConfiguredProject :one
INSERT INTO projects(workspace_id,key,name,color,repo_url,default_branch) VALUES($1,$2,$3,$4,$5,$6) RETURNING *;

-- name: FreezeLegacyProjectRepositories :exec
-- Capture legacy active runs' current repository before settings change.
-- New dispatches already contain this metadata in their immutable snapshot.
UPDATE runs r SET resolved_spec=jsonb_set(r.resolved_spec,'{repository}',jsonb_build_object('url',p.repo_url,'default_branch',p.default_branch))
FROM tickets t JOIN projects p ON p.id=t.project_id
WHERE r.ticket_id=t.id AND p.id=$1 AND r.state NOT IN ('completed','failed','cancelled') AND NOT (r.resolved_spec ? 'repository');

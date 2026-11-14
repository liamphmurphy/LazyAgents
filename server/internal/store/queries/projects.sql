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

-- name: GetProjectBySpace :one
-- The work a space tracks (LAZYAGENTS-213): its project, if it has one.
SELECT * FROM projects WHERE space_id = @space_id;

-- name: CreateSpaceForProject :one
-- Gives a project with no space its own, owned by user_id: the project's name and nearest
-- space color, and the project as where its new chats file tickets.
WITH made AS (
  INSERT INTO chat_spaces (workspace_id, user_id, name, color, project_id, space_memory)
  SELECT p.workspace_id, @user_id, left(btrim(p.name), 80), nearest_space_color(p.color), p.id, @space_memory
    FROM projects p WHERE p.id = @project_id AND p.space_id IS NULL
  RETURNING id, project_id
)
UPDATE projects SET space_id = made.id FROM made WHERE projects.id = made.project_id
RETURNING made.id::uuid AS space_id;

-- name: LinkProjectSpace :exec
-- Makes a space track work: the project is its, and its new chats file tickets there.
WITH linked AS (UPDATE projects SET space_id = @space_id WHERE projects.id = @project_id)
UPDATE chat_spaces SET project_id = @project_id, updated_at = now() WHERE chat_spaces.id = @space_id;

-- name: UnlinkProjectSpace :exec
-- A space stops tracking work. The project leaves it archived, with nothing to file into; a
-- space that filed its tickets there files none.
WITH done AS (
  UPDATE projects SET space_id = NULL, archived = true, revision = revision + 1 WHERE projects.id = @project_id RETURNING projects.id
)
UPDATE chat_spaces SET project_id = NULL, updated_at = now() WHERE chat_spaces.project_id IN (SELECT id FROM done);

-- name: ProjectHasTickets :one
SELECT EXISTS(SELECT 1 FROM tickets WHERE project_id = @project_id)::boolean;

-- name: SyncSpaceFromProject :exec
-- A project's name and color follow onto its space.
UPDATE chat_spaces SET name = left(btrim(@name::text), 80), color = nearest_space_color(@color::text), updated_at = now()
 WHERE id = (SELECT space_id FROM projects WHERE projects.id = @project_id)
   AND (name <> left(btrim(@name::text), 80) OR color <> nearest_space_color(@color::text));

-- name: SyncProjectFromSpace :exec
-- A space's name and color follow onto the project it tracks; a project's name is up to 100
-- characters, a space's 80, so a space name always fits.
UPDATE projects SET name = @name, color = @color, revision = revision + 1
 WHERE space_id = @space_id AND (name <> @name OR color <> @color);

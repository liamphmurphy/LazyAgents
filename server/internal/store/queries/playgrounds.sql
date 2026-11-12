-- name: GetChatPlayground :one
SELECT * FROM playgrounds WHERE chat_id = $1 AND workspace_id = $2;

-- name: StartChatPlayground :one
-- Records what a chat's playground runs and asks for it to start. One that
-- is already starting or running is restarted by the runner holding it with
-- the new spec, on the same port; one that ended starts afresh.
INSERT INTO playgrounds (workspace_id, user_id, chat_id, run_id, spec)
VALUES (sqlc.arg(workspace_id), sqlc.arg(user_id), sqlc.arg(chat_id), sqlc.arg(run_id), sqlc.arg(spec))
ON CONFLICT (chat_id) DO UPDATE SET
  spec = excluded.spec,
  user_id = excluded.user_id,
  run_id = CASE WHEN playgrounds.state IN ('starting', 'running') THEN playgrounds.run_id ELSE excluded.run_id END,
  action = CASE WHEN playgrounds.state IN ('starting', 'running') THEN 'restart' ELSE NULL END,
  state = CASE WHEN playgrounds.state IN ('starting', 'running') THEN playgrounds.state ELSE 'starting' END,
  error = NULL, error_log = '', stop_reason = '', step = '',
  updated_at = now()
RETURNING *;

-- name: RestartStoppedPlayground :one
-- The person starts an ended playground again with its last spec, in the
-- chat's current workspace.
UPDATE playgrounds SET state = 'starting', run_id = sqlc.arg(run_id), error = NULL, error_log = '', stop_reason = '', step = '', updated_at = now()
WHERE chat_id = sqlc.arg(chat_id) AND workspace_id = sqlc.arg(workspace_id) AND state IN ('stopped', 'failed')
RETURNING *;

-- name: RequestPlaygroundAction :one
-- Asks the runner holding a playground to reset its data, or stop it. A
-- reset needs it running; a stop wins over anything else asked.
UPDATE playgrounds SET action = sqlc.arg(action), updated_at = now()
WHERE chat_id = sqlc.arg(chat_id) AND workspace_id = sqlc.arg(workspace_id)
  AND CASE sqlc.arg(action)::text
    WHEN 'stop' THEN state IN ('starting', 'running')
    WHEN 'reset' THEN state = 'running' AND action IS NULL
    ELSE false END
RETURNING *;

-- name: StartingPlaygrounds :many
-- Playgrounds waiting for a runner, oldest first.
SELECT * FROM playgrounds WHERE state = 'starting' AND runner_id IS NULL ORDER BY updated_at, id LIMIT 20;

-- name: ClaimPlayground :one
-- A runner takes a waiting playground and the port it will serve it on.
UPDATE playgrounds SET runner_id = sqlc.arg(runner_id), stack = sqlc.arg(stack), port = sqlc.arg(port), idle_ttl_s = sqlc.arg(idle_ttl_s), action = NULL, step = 'Starting', updated_at = now()
WHERE id = sqlc.arg(id) AND state = 'starting' AND runner_id IS NULL
RETURNING *;

-- name: HeldPlaygrounds :many
-- What a runner process holds, to carry out what was asked of them.
SELECT * FROM playgrounds WHERE runner_id = $1 AND state IN ('starting', 'running') ORDER BY id;

-- name: SetPlaygroundStep :exec
UPDATE playgrounds SET step = sqlc.arg(step), updated_at = now()
WHERE id = sqlc.arg(id) AND runner_id = sqlc.arg(runner_id) AND state IN ('starting', 'running');

-- name: SetPlaygroundRunning :one
UPDATE playgrounds SET state = 'running', step = '', branch = sqlc.arg(branch), started_at = now(), data_reset_at = now(), last_used_at = now(), updated_at = now()
WHERE id = sqlc.arg(id) AND runner_id = sqlc.arg(runner_id) AND state IN ('starting', 'running')
RETURNING *;

-- name: SetPlaygroundDataReset :exec
UPDATE playgrounds SET step = '', data_reset_at = now(), updated_at = now()
WHERE id = sqlc.arg(id) AND runner_id = sqlc.arg(runner_id) AND state = 'running';

-- name: ClearPlaygroundAction :exec
-- Done with what was asked, unless something else was asked meanwhile.
UPDATE playgrounds SET action = NULL, updated_at = now()
WHERE id = sqlc.arg(id) AND runner_id = sqlc.arg(runner_id) AND action = sqlc.arg(action);

-- name: FailPlayground :exec
UPDATE playgrounds SET state = 'failed', error = sqlc.arg(error), error_log = sqlc.arg(error_log), step = '',
  runner_id = NULL, port = NULL, action = NULL, updated_at = now()
WHERE id = sqlc.arg(id) AND runner_id = sqlc.arg(runner_id) AND state IN ('starting', 'running');

-- name: StopPlayground :exec
UPDATE playgrounds SET state = 'stopped', stop_reason = sqlc.arg(reason), step = '',
  runner_id = NULL, port = NULL, action = NULL, updated_at = now()
WHERE id = sqlc.arg(id) AND runner_id = sqlc.arg(runner_id) AND state IN ('starting', 'running');

-- name: StopStackPlaygrounds :exec
-- A runner starting up removed its install's containers: what any earlier
-- runner process of the install held is gone.
UPDATE playgrounds SET state = 'stopped', stop_reason = sqlc.arg(reason), step = '',
  runner_id = NULL, port = NULL, action = NULL, updated_at = now()
WHERE stack = sqlc.arg(stack) AND state IN ('starting', 'running');

-- name: TouchPlaygrounds :exec
-- When the person last used them, as the gateway saw it; written once per
-- sweep, not per request.
UPDATE playgrounds SET last_used_at = sqlc.arg(used_at)
WHERE id = ANY(sqlc.arg(ids)::uuid[]) AND (last_used_at IS NULL OR last_used_at < sqlc.arg(used_at));

-- name: PlaygroundsForRun :many
SELECT * FROM playgrounds WHERE run_id = $1 AND state IN ('starting', 'running');

-- name: RefusePlayground :exec
-- A waiting playground no runner can take (every port is in use).
UPDATE playgrounds SET state = 'failed', error = sqlc.arg(error), step = '', updated_at = now()
WHERE id = sqlc.arg(id) AND state = 'starting' AND runner_id IS NULL;

-- name: SetPlaygroundSpec :one
-- A playground's spec changed without a restart (the document its API tab reads).
UPDATE playgrounds SET spec = sqlc.arg(spec), updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: SavePlaygroundSpec :exec
-- The last spec a repository's playground started with, for starting one
-- in any chat on the repository without an agent turn.
INSERT INTO playground_specs (workspace_id, repo, spec, saved_from_chat_id)
VALUES (sqlc.arg(workspace_id), sqlc.arg(repo), sqlc.arg(spec), sqlc.arg(chat_id))
ON CONFLICT (workspace_id, repo) DO UPDATE SET spec = excluded.spec, saved_from_chat_id = excluded.saved_from_chat_id, updated_at = now();

-- name: GetPlaygroundSpec :one
SELECT * FROM playground_specs WHERE workspace_id = $1 AND repo = $2;

-- name: RecentPlaygroundImages :many
-- The service images this install's playgrounds used lately, so a runner
-- can pull those it lacks before a start has to wait for them.
SELECT DISTINCT (s ->> 'image')::text AS image
FROM playgrounds p,
  jsonb_array_elements(CASE WHEN jsonb_typeof(p.spec -> 'services') = 'array' THEN p.spec -> 'services' ELSE '[]'::jsonb END) s
WHERE p.stack = sqlc.arg(stack)::text AND p.updated_at > now() - interval '30 days' AND coalesce(s ->> 'image', '') <> ''
ORDER BY image
LIMIT 20;

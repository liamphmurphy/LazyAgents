-- name: GetTestEnvironment :one
SELECT * FROM test_environments WHERE workspace_id = $1 AND repo = $2;

-- name: LockTestEnvironment :one
-- Learning merges into what's there; two runs on one repository finishing
-- turns together must not lose each other's packages.
SELECT * FROM test_environments WHERE workspace_id = $1 AND repo = $2 FOR UPDATE;

-- name: ListTestEnvironments :many
SELECT * FROM test_environments WHERE workspace_id = $1 ORDER BY repo;

-- name: CreateInferredTestEnvironment :exec
-- The first recipe for a repository. A recipe that's already there, learned
-- or edited, wins over a fresh inference.
INSERT INTO test_environments (workspace_id, repo, recipe, inferred_from)
VALUES ($1, $2, $3, $4)
ON CONFLICT (workspace_id, repo) DO NOTHING;

-- name: SaveLearnedTestEnvironment :exec
INSERT INTO test_environments (workspace_id, repo, recipe, learned_from_run_id)
VALUES ($1, $2, $3, $4)
ON CONFLICT (workspace_id, repo) DO UPDATE SET recipe = EXCLUDED.recipe,
  learned_from_run_id = EXCLUDED.learned_from_run_id, updated_at = now();

-- name: PutTestEnvironment :one
INSERT INTO test_environments (workspace_id, repo, recipe, edited_at)
VALUES ($1, $2, $3, now())
ON CONFLICT (workspace_id, repo) DO UPDATE SET recipe = EXCLUDED.recipe, edited_at = now(), updated_at = now()
RETURNING *;

-- name: DeleteTestEnvironment :execrows
DELETE FROM test_environments WHERE workspace_id = $1 AND repo = $2;

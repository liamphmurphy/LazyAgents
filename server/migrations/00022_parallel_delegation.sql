-- +goose Up
-- A parent may have several live delegated runs, each in its own git worktree.
-- The runs service caps them (MaxParallelDelegations) under the parent's row lock.
DROP INDEX runs_one_active_child;

-- +goose Down
-- Keep one live child per parent so the old unique index can come back.
UPDATE runs SET state = 'cancelled', outcome = coalesce(outcome, 'cancelled by migration rollback')
WHERE parent_run_id IS NOT NULL AND state NOT IN ('completed', 'failed', 'cancelled')
  AND id NOT IN (SELECT DISTINCT ON (parent_run_id) id FROM runs
                 WHERE parent_run_id IS NOT NULL AND state NOT IN ('completed', 'failed', 'cancelled')
                 ORDER BY parent_run_id, started_at);
CREATE UNIQUE INDEX runs_one_active_child ON runs (parent_run_id)
  WHERE parent_run_id IS NOT NULL AND state NOT IN ('completed', 'failed', 'cancelled');

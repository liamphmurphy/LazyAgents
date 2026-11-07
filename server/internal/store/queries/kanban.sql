-- Kanban board, ticket estimates and epic criteria (Architecture/ADR - Kanban board,
-- estimates and epic criteria). Service-level rules (the scale constant, people-win
-- estimates, the epic done gate) live in the tracker package.

-- name: SetTicketEstimate :one
-- Sets a ticket's point estimate and bumps its revision. NULL points clears the
-- estimate together with its rationale, author and time.
UPDATE tickets SET
  estimate_points = sqlc.narg(points),
  estimate_rationale = CASE WHEN sqlc.narg(points)::int IS NULL THEN NULL ELSE sqlc.narg(rationale) END,
  estimated_by_type = CASE WHEN sqlc.narg(points)::int IS NULL THEN NULL ELSE sqlc.narg(by_type) END,
  estimated_by_id = CASE WHEN sqlc.narg(points)::int IS NULL THEN NULL ELSE sqlc.narg(by_id)::uuid END,
  estimated_at = CASE WHEN sqlc.narg(points)::int IS NULL THEN NULL ELSE now() END,
  revision = revision + 1,
  updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: SetEpicStatus :one
UPDATE epics SET status = $2, updated_at = now() WHERE id = $1 AND workspace_id = $3 RETURNING *;

-- name: EpicRollups :many
-- Per-epic rollups for the board, bootstrap and PATCH /epics. Canceled tickets are
-- excluded from every ticket and point count.
SELECT
  e.id AS epic_id,
  (SELECT count(*) FROM epic_criteria c WHERE c.epic_id = e.id)::int AS criteria_total,
  (SELECT count(*) FROM epic_criteria c WHERE c.epic_id = e.id AND c.done)::int AS criteria_done,
  coalesce((SELECT sum(t.estimate_points) FROM tickets t WHERE t.epic_id = e.id AND t.status <> 'canceled'), 0)::int AS points_total,
  coalesce((SELECT sum(t.estimate_points) FROM tickets t WHERE t.epic_id = e.id AND t.status = 'done'), 0)::int AS points_done,
  (SELECT count(*) FROM tickets t WHERE t.epic_id = e.id AND t.status <> 'canceled')::int AS tickets_total,
  (SELECT count(*) FROM tickets t WHERE t.epic_id = e.id AND t.status NOT IN ('done', 'canceled'))::int AS tickets_open
FROM epics e
WHERE e.id = ANY(sqlc.arg(epic_ids)::uuid[]);

-- name: EpicOpenTicketKeys :many
-- What blocks the epic done gate on the ticket side.
SELECT key FROM tickets WHERE epic_id = $1 AND status NOT IN ('done', 'canceled') ORDER BY number;

-- name: ListEpicCriteria :many
SELECT * FROM epic_criteria WHERE epic_id = $1 ORDER BY position;

-- name: GetEpicCriterion :one
SELECT c.* FROM epic_criteria c JOIN epics e ON e.id = c.epic_id
WHERE c.id = $1 AND e.workspace_id = $2;

-- name: GetEpicCriterionByPosition :one
SELECT * FROM epic_criteria WHERE epic_id = $1 AND position = $2;

-- name: CreateEpicCriterion :one
-- Appends at the end. Concurrent appends collide on UNIQUE (epic_id, position): retry
-- on 23505. The constraint is deferrable, so it can't be an ON CONFLICT arbiter.
INSERT INTO epic_criteria (epic_id, position, text)
VALUES ($1, (SELECT coalesce(max(position), 0) + 1 FROM epic_criteria WHERE epic_id = $1), $2)
RETURNING *;

-- name: UpdateEpicCriterionText :one
UPDATE epic_criteria SET text = $2 WHERE id = $1 RETURNING *;

-- name: SetEpicCriterionDone :one
-- Records who checked it; unticking clears who and when.
UPDATE epic_criteria SET
  done = sqlc.arg(done),
  checked_by_type = CASE WHEN sqlc.arg(done) THEN sqlc.narg(by_type) ELSE NULL END,
  checked_by_id = CASE WHEN sqlc.arg(done) THEN sqlc.narg(by_id)::uuid ELSE NULL END,
  checked_at = CASE WHEN sqlc.arg(done) THEN now() ELSE NULL END
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: DeleteEpicCriterion :one
DELETE FROM epic_criteria WHERE id = $1 RETURNING epic_id, position;

-- name: CompactEpicCriteria :exec
-- Closes the gap after a delete so positions stay 1..n. The deferrable unique
-- constraint lets this shift rows in one statement.
UPDATE epic_criteria SET position = position - 1 WHERE epic_id = $1 AND position > $2;

-- name: ReorderEpicCriterion :exec
UPDATE epic_criteria SET position = CASE
 WHEN id = sqlc.arg(id) THEN sqlc.arg(new_position)::int
 WHEN position > sqlc.arg(old_position)::int AND position <= sqlc.arg(new_position)::int THEN position - 1
 WHEN position < sqlc.arg(old_position)::int AND position >= sqlc.arg(new_position)::int THEN position + 1
 ELSE position END
WHERE epic_id = sqlc.arg(epic_id) AND position BETWEEN least(sqlc.arg(old_position)::int, sqlc.arg(new_position)::int) AND greatest(sqlc.arg(old_position)::int, sqlc.arg(new_position)::int);

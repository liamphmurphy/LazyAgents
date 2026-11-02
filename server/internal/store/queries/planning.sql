-- name: CreatePlanningSession :one
INSERT INTO planning_sessions(workspace_id,project_id,epic_id,artifact_id,source_version_id,goal,run_id,created_by)
VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING *;

-- name: GetPlanningSession :one
SELECT * FROM planning_sessions WHERE id=$1 AND workspace_id=$2;

-- name: PlanningSessionForRun :one
SELECT * FROM planning_sessions WHERE run_id=$1;

-- name: LockPlanningSession :one
SELECT * FROM planning_sessions WHERE id=$1 AND workspace_id=$2 FOR UPDATE;

-- name: ListPlanningSessions :many
SELECT * FROM planning_sessions WHERE workspace_id=$1 ORDER BY created_at DESC,id DESC LIMIT 100;

-- name: UpdatePlanningSession :one
UPDATE planning_sessions SET proposals=$2,state=$3,ticket_keys=$4,revision=revision+1,updated_at=now() WHERE id=$1 RETURNING *;

-- name: PlanningEpicTickets :many
SELECT key,title,status FROM tickets WHERE epic_id=$1 ORDER BY key LIMIT 100;

-- name: ActivePlanningWork :many
SELECT p.id,p.run_id,p.goal,r.agent_id,r.state,r.harness,r.model FROM planning_sessions p JOIN runs r ON r.id=p.run_id WHERE p.workspace_id=$1 AND r.state NOT IN ('completed','failed','cancelled') ORDER BY p.created_at;

-- name: ReadyPlanningTickets :many
SELECT t.key FROM tickets t JOIN projects p ON p.id=t.project_id
WHERE t.workspace_id=$1 AND t.key=ANY(sqlc.arg(keys)::text[]) AND NOT p.archived
AND t.assignee_agent_id IS NOT NULL AND t.status<>'done'
AND NOT EXISTS(SELECT 1 FROM runs r WHERE r.ticket_id=t.id AND r.state NOT IN ('completed','failed','cancelled'))
AND NOT EXISTS(SELECT 1 FROM ticket_dependencies d JOIN tickets b ON b.id=d.blocker_id WHERE d.ticket_id=t.id AND b.status<>'done')
ORDER BY t.key;

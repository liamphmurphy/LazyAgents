-- name: OverviewTicketCounts :many
SELECT t.project_id,t.epic_id,t.status,count(*)::int AS count
FROM tickets t JOIN projects p ON p.id=t.project_id WHERE t.workspace_id=$1 AND NOT p.archived GROUP BY t.project_id,t.epic_id,t.status;

-- name: OverviewAgentTickets :many
-- Counts cover the full queue; only the ten highest-priority previews travel.
WITH ranked AS (
 SELECT t.id,t.key,t.title,t.status,t.priority,t.assignee_agent_id,
 count(*) OVER (PARTITION BY t.assignee_agent_id)::int AS queue_count,
 row_number() OVER (PARTITION BY t.assignee_agent_id ORDER BY
 CASE t.priority WHEN 'urgent' THEN 0 WHEN 'high' THEN 1 WHEN 'medium' THEN 2 WHEN 'low' THEN 3 ELSE 4 END,t.sort_order,t.key) AS pos
 FROM tickets t JOIN projects p ON p.id=t.project_id WHERE t.workspace_id=$1 AND NOT p.archived AND t.assignee_agent_id IS NOT NULL AND t.status NOT IN ('done', 'canceled')
 AND NOT EXISTS(SELECT 1 FROM runs r WHERE r.ticket_id=t.id AND r.state NOT IN ('completed','failed','cancelled'))
) SELECT id,key,title,status,priority,assignee_agent_id,queue_count FROM ranked WHERE pos<=10 ORDER BY assignee_agent_id,pos;

-- name: OverviewEpicTickets :many
SELECT id,key,title,status,priority,epic_id,assignee_agent_id,project_id,updated_at FROM tickets WHERE workspace_id=$1
ORDER BY CASE status WHEN 'in_progress' THEN 0 WHEN 'in_review' THEN 1 WHEN 'todo' THEN 2 WHEN 'backlog' THEN 3 ELSE 4 END,key;

-- name: OverviewRuns :many
SELECT r.id,r.agent_id,r.ticket_id,r.state,r.harness,r.model,r.started_at,
 t.key,t.title,t.status,t.priority,
 coalesce((SELECT ps.text FROM plan_steps ps WHERE ps.run_id=r.id AND ps.state IN ('current','needs_human','failed') ORDER BY ps.position LIMIT 1),'')::text AS step,
 coalesce((SELECT ps.position+1 FROM plan_steps ps WHERE ps.run_id=r.id AND ps.state IN ('current','needs_human','failed') ORDER BY ps.position LIMIT 1),0)::int AS step_position,
 (SELECT count(*) FROM plan_steps ps WHERE ps.run_id=r.id)::int AS steps_total,
 (SELECT count(*) FROM plan_steps ps WHERE ps.run_id=r.id AND ps.state='done')::int AS steps_done
FROM runs r JOIN tickets t ON t.id=r.ticket_id
WHERE r.workspace_id=$1 AND r.state NOT IN ('completed','failed','cancelled')
ORDER BY CASE r.state WHEN 'blocked' THEN 0 WHEN 'waiting' THEN 1 WHEN 'running' THEN 2 WHEN 'starting' THEN 3 WHEN 'paused' THEN 4 ELSE 5 END,r.started_at;

-- name: OverviewArtifacts :many
SELECT id,title,type,path,epic_id,updated_at FROM artifacts WHERE workspace_id=$1 AND current_version_id IS NOT NULL
ORDER BY updated_at DESC,id;

-- name: WeeklySpend :one
-- Reported cost of every run started since the week began (Monday 00:00 UTC).
SELECT count(*)::int AS runs,
 coalesce(sum(cost_usd_micros),0)::bigint AS cost_usd_micros,
 (count(*) FILTER (WHERE cost_source IS NULL OR cost_source IN ('unknown','partial')))::int AS unpriced_runs
FROM runs WHERE workspace_id=sqlc.arg(workspace_id) AND started_at>=sqlc.arg(since)::timestamptz;

-- name: ReviewOverDay :one
-- Measure the current review spell, not the last unrelated ticket edit.
SELECT count(*)::int FROM tickets t JOIN projects p ON p.id=t.project_id
WHERE t.workspace_id=$1 AND NOT p.archived AND t.status='in_review'
AND coalesce((SELECT max(e.created_at) FROM activity_events e WHERE e.ticket_id=t.id AND e.verb='status_changed' AND e.payload->>'to'='in_review'),t.created_at)<now()-interval '1 day';

-- name: EpicAgentTime :many
SELECT t.epic_id,
 sum(greatest(extract(epoch FROM (coalesce(rt.ended_at,now())-coalesce(rt.process_started_at,rt.started_at))),0))::float8 AS seconds
FROM run_turns rt JOIN runs r ON r.id=rt.run_id JOIN tickets t ON t.id=r.ticket_id
WHERE r.workspace_id=$1 AND t.epic_id IS NOT NULL AND (r.mode='unmanaged' OR rt.process_started_at IS NOT NULL)
GROUP BY t.epic_id;

-- name: CompletedTicketMetrics :many
WITH completed AS (
 SELECT e.ticket_id,max(e.created_at) AS completed_at FROM activity_events e
 WHERE e.workspace_id=sqlc.arg(workspace_id) AND e.verb='status_changed' AND e.payload->>'to'='done'
 AND e.created_at<=sqlc.arg(as_of)::timestamptz GROUP BY e.ticket_id
)
SELECT t.id,t.assignee_agent_id,c.completed_at::timestamptz AS completed_at,
 greatest(extract(epoch FROM (c.completed_at-coalesce((SELECT min(e.created_at) FROM activity_events e WHERE e.ticket_id=t.id AND e.verb='status_changed' AND e.payload->>'to'='in_progress' AND e.created_at<=c.completed_at),t.created_at))),0)::float8 AS cycle_seconds
FROM completed c JOIN tickets t ON t.id=c.ticket_id
-- Throughput counts tickets that are done now: one reopened or canceled after
-- reaching done is not a completion (Kanban board ADR).
WHERE t.status='done' AND c.completed_at>=sqlc.arg(as_of)::timestamptz-interval '30 days' ORDER BY c.completed_at;

-- name: RunCostMetrics :many
-- Spend is the reported cost of runs started in the selected window.
SELECT agent_id,started_at,(cost_usd_micros::float8/1000000)::float8 AS cost_usd,cost_source
FROM runs WHERE workspace_id=sqlc.arg(workspace_id) AND started_at>=sqlc.arg(as_of)::timestamptz-interval '30 days' AND started_at<=sqlc.arg(as_of)::timestamptz;

-- name: PRMetrics :many
SELECT t.assignee_agent_id,t.pr_state,
 max(e.created_at)::timestamptz AS closed_at
FROM tickets t JOIN activity_events e ON e.ticket_id=t.id AND e.verb IN ('pr_merged','pr_closed')
WHERE t.workspace_id=sqlc.arg(workspace_id) AND t.pr_state IN ('merged','closed') AND e.created_at<=sqlc.arg(as_of)::timestamptz
GROUP BY t.id HAVING max(e.created_at)>=sqlc.arg(as_of)::timestamptz-interval '30 days';

-- name: MetricWorkspaces :many
SELECT id FROM workspaces;

-- name: SaveMetricSnapshot :exec
INSERT INTO metric_snapshots(workspace_id,range,payload,generated_at) VALUES($1,$2,$3,$4)
ON CONFLICT(workspace_id,range) DO UPDATE SET payload=EXCLUDED.payload,generated_at=EXCLUDED.generated_at;

-- name: GetMetricSnapshot :one
SELECT * FROM metric_snapshots WHERE workspace_id=$1 AND range=$2;

-- name: AgentRunHistory :many
SELECT r.id,r.state,r.harness,r.model,r.started_at,r.ended_at,r.cost_usd_micros,r.cost_source,r.tool_calls,r.input_tokens,
 (r.input_tokens+r.output_tokens)::bigint AS tokens,t.id AS ticket_id,t.key,t.title,t.status,t.priority
FROM runs r JOIN tickets t ON t.id=r.ticket_id
WHERE r.workspace_id=$1 AND r.agent_id=$2 ORDER BY r.started_at DESC,r.id DESC LIMIT 51 OFFSET sqlc.arg(skip)::int;

-- name: LiveAgentRunStates :many
SELECT agent_id,state FROM runs WHERE workspace_id=$1 AND state NOT IN ('completed','failed','cancelled');

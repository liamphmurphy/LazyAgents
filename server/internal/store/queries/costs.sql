-- name: CostUnits :many
-- Spend units for the Costs page (LAZYAGENTS-60). Every run contributes its
-- authoritative counters (runs.cost_usd_micros, input+output tokens), so any
-- grouping of the units adds up to the sum of the run counters.
--
-- Chat runs can switch model mid-run (same harness), so their spend is split per
-- reply: each non-user message with usage carries the turn's usage, and its
-- micros are int64(cost_usd * 1e6) in float64, exactly how runner/chat.go adds
-- it to the run counter (float8 multiply, then trunc toward zero). Turns that
-- ended without a message (paused or cancelled) are the remainder, attributed
-- to the run's current model at its start. If the replies claim more than the
-- run counter (legacy repaired history) the replies are ignored and the whole
-- run is one unit, so totals never exceed the counters.
--
-- A unit's day is its UTC date: the reply's time, or the run's start.
WITH base AS (
  SELECT r.id, r.agent_id, r.purpose, r.ticket_id, r.parent_run_id, r.harness, r.model, r.state,
    r.started_at, r.cost_usd_micros, r.input_tokens + r.output_tokens AS tokens, r.cost_source,
    coalesce(t.project_id, ps.project_id, cs.project_id) AS project_id, cs.id AS chat_id
  FROM runs r
  LEFT JOIN tickets t ON t.id = r.ticket_id
  LEFT JOIN planning_sessions ps ON ps.run_id = r.id
  LEFT JOIN LATERAL (
    SELECT c.id, c.project_id FROM chat_sessions c
    WHERE r.purpose = 'chat' AND (c.current_run_id = r.id OR c.id IN (SELECT m.session_id FROM chat_messages m WHERE m.run_id = r.id))
    LIMIT 1
  ) cs ON true
  WHERE r.workspace_id = sqlc.arg(workspace_id)
    AND (sqlc.narg(project_id)::uuid IS NULL OR coalesce(t.project_id, ps.project_id, cs.project_id) = sqlc.narg(project_id))
    AND (sqlc.narg(ticket_id)::uuid IS NULL OR r.ticket_id = sqlc.narg(ticket_id))
    AND (sqlc.narg(chat_id)::uuid IS NULL OR cs.id = sqlc.narg(chat_id))
    AND (sqlc.narg(epic_id)::uuid IS NULL OR t.epic_id = sqlc.narg(epic_id))
),
replies AS (
  SELECT m.run_id, m.created_at, coalesce(m.harness, b.harness) AS harness, coalesce(m.model, b.model) AS model,
    trunc(coalesce((m.usage->>'cost_usd')::float8, 0) * 1000000::float8)::bigint AS cost,
    coalesce((m.usage->>'input_tokens')::bigint, 0) + coalesce((m.usage->>'output_tokens')::bigint, 0) AS tokens
  FROM chat_messages m JOIN base b ON b.id = m.run_id
  WHERE b.purpose = 'chat' AND m.role <> 'user' AND m.usage IS NOT NULL
),
split AS (
  SELECT b.id AS run_id, (rt.run_id IS NOT NULL AND rt.cost <= b.cost_usd_micros AND rt.tokens <= b.tokens) AS ok,
    coalesce(rt.cost, 0)::bigint AS cost, coalesce(rt.tokens, 0)::bigint AS tokens
  FROM base b LEFT JOIN (SELECT run_id, sum(cost) AS cost, sum(tokens) AS tokens FROM replies GROUP BY run_id) rt ON rt.run_id = b.id
),
units AS (
  SELECT rp.run_id, rp.created_at AS at, rp.harness, rp.model, rp.cost, rp.tokens
  FROM replies rp JOIN split s ON s.run_id = rp.run_id AND s.ok
  UNION ALL
  SELECT b.id, b.started_at, b.harness, b.model,
    CASE WHEN s.ok THEN b.cost_usd_micros - s.cost ELSE b.cost_usd_micros END,
    CASE WHEN s.ok THEN b.tokens - s.tokens ELSE b.tokens END
  FROM base b JOIN split s ON s.run_id = b.id
  -- A fully split run needs no empty remainder (it would count the run on its start day).
  WHERE NOT (s.ok AND b.cost_usd_micros = s.cost AND b.tokens = s.tokens)
)
SELECT u.run_id, (u.at AT TIME ZONE 'UTC')::date AS day, u.harness::text AS harness, u.model::text AS model,
  u.cost::bigint AS cost_micros, u.tokens::bigint AS tokens,
  b.agent_id, b.purpose, b.ticket_id, b.parent_run_id, coalesce(b.project_id::text, '')::text AS project_id, b.state, b.cost_source, b.started_at,
  coalesce(b.cost_source IN ('unknown', 'partial'), b.tokens > 0)::boolean AS unpriced
FROM units u JOIN base b ON b.id = u.run_id
WHERE (sqlc.narg(from_day)::date IS NULL OR (u.at AT TIME ZONE 'UTC')::date >= sqlc.narg(from_day))
  AND (sqlc.narg(to_day)::date IS NULL OR (u.at AT TIME ZONE 'UTC')::date <= sqlc.narg(to_day))
ORDER BY u.at, u.run_id;

-- name: CostTickets :many
-- Display fields for the Costs page's most expensive tickets.
SELECT t.id, t.key, t.title, t.estimate_points, p.key AS project_key
FROM tickets t JOIN projects p ON p.id = t.project_id
WHERE t.workspace_id = sqlc.arg(workspace_id) AND t.id = ANY(sqlc.arg(ids)::uuid[]);

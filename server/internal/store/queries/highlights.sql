-- name: HighlightsBuckets :many
-- A turn may produce several assistant messages. Use its first reply once;
-- legacy messages without a turn remain individual, unclassified samples.
-- Attribute to the recorded speaker, never the chat's current host or model.
WITH replies AS (
  SELECT DISTINCT ON (coalesce(m.run_turn_id, m.id))
    m.created_at,
    coalesce(p.id::text, '')::text AS persona_id,
    coalesce(p.name, 'Unknown persona')::text AS persona,
    coalesce(m.harness, '')::text AS harness, coalesce(m.model, '')::text AS model,
    coalesce(t.effort, '')::text AS effort, coalesce(t.effort_level, '')::text AS effort_level,
    coalesce(t.effort_reasons, '{}'::text[])::text[] AS reasons,
    CASE WHEN t.status = 'done' AND t.ended_at >= t.process_started_at
      THEN (extract(epoch FROM (t.ended_at - t.process_started_at)) * 1000)::bigint END AS duration_ms
  FROM chat_messages m
  JOIN chat_sessions c ON c.id = m.session_id
  JOIN runs r ON r.id = m.run_id AND r.workspace_id = c.workspace_id AND r.purpose = 'chat'
  LEFT JOIN run_turns t ON t.id = m.run_turn_id AND t.run_id = r.id
  LEFT JOIN personas p ON p.id = m.persona_id AND p.workspace_id = c.workspace_id AND p.user_id = c.created_by
  WHERE c.workspace_id = sqlc.arg(workspace_id) AND c.created_by = sqlc.arg(user_id)
    AND m.role = 'assistant'
    AND m.created_at < sqlc.arg(until_time)::timestamptz
    AND (sqlc.narg(project_key)::text IS NULL OR c.project_id IN (
      SELECT id FROM projects WHERE workspace_id = c.workspace_id AND key = sqlc.narg(project_key)))
  ORDER BY coalesce(m.run_turn_id, m.id), m.seq
)
SELECT persona_id, persona, harness, model, effort, effort_level, reasons,
  count(*)::bigint AS turns, coalesce(sum(duration_ms), 0)::bigint AS duration_ms,
  count(duration_ms)::bigint AS timed_turns
FROM replies
-- Apply the lower bound after choosing the first reply, so later replies
-- cannot count the same turn again in another date range.
WHERE created_at >= sqlc.arg(from_time)::timestamptz
GROUP BY persona_id, persona, harness, model, effort, effort_level, reasons
ORDER BY persona_id, harness, model, effort, effort_level, reasons;

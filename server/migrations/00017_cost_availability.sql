-- +goose Up
ALTER TABLE runs DROP CONSTRAINT runs_cost_source_check;
ALTER TABLE runs ADD CONSTRAINT runs_cost_source_check
  CHECK (cost_source IN ('harness', 'computed', 'reported', 'unknown', 'partial'));
-- Codex exec reports cumulative session usage on resume. Older runners added
-- those totals again. Repair only turns with an explicit Codex session event;
-- retain the original JSON so downgrades can restore it.
CREATE TEMP TABLE la_usage_repairs ON COMMIT DROP AS
WITH sessions AS (
 SELECT DISTINCT ON (e.run_id, (s.payload->>'turn')::int)
   e.run_id, (s.payload->>'turn')::int AS turn_seq,
   s.seq AS start_seq,
   (SELECT min(n.seq) FROM run_events n WHERE n.run_id=e.run_id AND n.seq>s.seq AND n.type='lifecycle' AND n.payload->>'state'='turn_started') AS end_seq,
   e.payload->>'session_id' AS session_id,coalesce(e.payload->>'model','') AS model
 FROM run_events e
 JOIN LATERAL (
  SELECT p.payload,p.seq FROM run_events p WHERE p.run_id=e.run_id AND p.seq<e.seq
   AND p.type='lifecycle' AND p.payload->>'state'='turn_started' AND p.payload->>'turn' ~ '^[0-9]+$' ORDER BY p.seq DESC LIMIT 1
 ) s ON true
 WHERE e.type='lifecycle' AND e.payload->>'state'='harness_started'
  AND e.payload->>'harness'='codex' AND coalesce(e.payload->>'session_id','')<>''
 ORDER BY e.run_id,(s.payload->>'turn')::int,e.seq DESC
), legacy AS (
 SELECT t.id,t.run_id,t.usage,s.session_id,s.model,s.start_seq,s.end_seq,
  lag(s.model) OVER(PARTITION BY t.run_id,s.session_id ORDER BY t.seq) AS previous_model,
  lag(t.usage) OVER(PARTITION BY t.run_id,s.session_id ORDER BY t.seq) AS previous
 FROM run_turns t JOIN sessions s ON s.run_id=t.run_id AND s.turn_seq=t.seq
 WHERE NOT (t.usage ? 'session_input_tokens')
  AND coalesce((t.usage->>'input_tokens')::bigint,0)>0
), deltas AS (
 SELECT *,
  CASE WHEN (usage->>'input_tokens')::bigint < coalesce((previous->>'input_tokens')::bigint,0) OR coalesce((usage->>'output_tokens')::bigint,0)<coalesce((previous->>'output_tokens')::bigint,0) OR coalesce((usage->>'cached_tokens')::bigint,0)<coalesce((previous->>'cached_tokens')::bigint,0)
   THEN '{}'::jsonb ELSE coalesce(previous,'{}'::jsonb) END AS baseline
 FROM legacy
)
SELECT id,run_id,start_seq,end_seq,
 usage || jsonb_build_object(
  'legacy_cumulative_usage',usage,
  'session_id',session_id,
  'session_input_tokens',coalesce((usage->>'input_tokens')::bigint,0),
  'session_output_tokens',coalesce((usage->>'output_tokens')::bigint,0),
  'session_cached_tokens',coalesce((usage->>'cached_tokens')::bigint,0),
  'input_tokens',greatest(coalesce((usage->>'input_tokens')::bigint,0)-coalesce((baseline->>'input_tokens')::bigint,0),0),
  'output_tokens',greatest(coalesce((usage->>'output_tokens')::bigint,0)-coalesce((baseline->>'output_tokens')::bigint,0),0),
  'cached_tokens',greatest(coalesce((usage->>'cached_tokens')::bigint,0)-coalesce((baseline->>'cached_tokens')::bigint,0),0),
  'cost_usd',CASE WHEN previous IS NOT NULL AND model<>previous_model THEN 0 ELSE greatest(coalesce((usage->>'cost_usd')::numeric,0)-coalesce((baseline->>'cost_usd')::numeric,0),0) END,
  'cost_source',CASE WHEN previous IS NOT NULL AND model<>previous_model THEN 'unknown' ELSE coalesce(usage->>'cost_source','') END
 ) AS corrected
FROM deltas;
UPDATE run_turns t SET usage=f.corrected FROM la_usage_repairs f WHERE t.id=f.id;
UPDATE run_events e SET payload=e.payload || jsonb_build_object(
 'input_tokens',f.corrected->'input_tokens','output_tokens',f.corrected->'output_tokens',
 'cached_tokens',f.corrected->'cached_tokens','cost_usd',f.corrected->'cost_usd','cost_source',f.corrected->'cost_source','cost_known',f.corrected->>'cost_source' IN ('harness','computed','reported'))
FROM la_usage_repairs f WHERE e.run_id=f.run_id AND e.seq>f.start_seq AND (f.end_seq IS NULL OR e.seq<f.end_seq) AND e.type='usage';
UPDATE runs r SET input_tokens=u.input_tokens,output_tokens=u.output_tokens,cost_usd_micros=u.cost_usd_micros
FROM (
 SELECT run_id,sum(coalesce((usage->>'input_tokens')::bigint,0)) AS input_tokens,
 sum(coalesce((usage->>'output_tokens')::bigint,0)) AS output_tokens,
 sum(trunc(coalesce((usage->>'cost_usd')::numeric,0)*1000000))::bigint AS cost_usd_micros
 FROM run_turns WHERE run_id IN (SELECT run_id FROM la_usage_repairs) GROUP BY run_id
) u WHERE r.id=u.run_id AND r.mode='managed';

-- Older Codex turns recorded token usage with an empty cost source when the
-- model had no price. Preserve known spend but mark the total incomplete.
UPDATE runs r SET cost_source = CASE WHEN r.cost_usd_micros > 0 THEN 'partial' ELSE 'unknown' END
WHERE EXISTS (
 SELECT 1 FROM run_turns t WHERE t.run_id=r.id
 AND coalesce(t.usage->>'cost_source','') IN ('','unknown')
 AND (coalesce((t.usage->>'input_tokens')::bigint,0)>0 OR coalesce((t.usage->>'output_tokens')::bigint,0)>0)
);

-- Snapshots are derived and will be recomputed with cost completeness.
DELETE FROM metric_snapshots;

-- +goose Down
UPDATE runs SET cost_source=NULL WHERE cost_source IN ('unknown','partial');
ALTER TABLE runs DROP CONSTRAINT runs_cost_source_check;
ALTER TABLE runs ADD CONSTRAINT runs_cost_source_check CHECK (cost_source IN ('harness','computed','reported'));

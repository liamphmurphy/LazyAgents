import type { components } from "./api/schema";

type Bucket = components["schemas"]["HighlightsBucket"];
export type HighlightRow = { key: string; label: string; turns: number; automatic: number };
const effortNames: Record<string, string> = { low: "Low", medium: "Medium", high: "High", xhigh: "Extra high", max: "Max" };
export const effortName = (s: string) => effortNames[s] ?? (s || "Model default / unrecorded");
const reasonNames: Record<string, string> = { confirmation: "Go-ahead", effort_mismatch: "Retry or edit" };

/** Buckets contain counts, never message content. Unknown history stays unknown. */
export function routingHighlights(buckets: Bucket[]) {
  let turns = 0, automatic = 0, chosen = 0, raised = 0, adjusted = 0, duration = 0, timed = 0;
  const personas = new Map<string, HighlightRow>(), models = new Map<string, HighlightRow>(), efforts = new Map<string, HighlightRow>(), levels = new Map<string, HighlightRow>(), reasons = new Map<string, HighlightRow>();
  const add = (map: Map<string, HighlightRow>, key: string, label: string, b: Bucket) => {
    const row = map.get(key) ?? { key, label, turns: 0, automatic: 0 };
    row.turns += b.turns;
    row.automatic += b.effort_level ? b.turns : 0;
    map.set(key, row);
  };
  for (const b of buckets) {
    turns += b.turns;
    duration += b.duration_ms;
    timed += b.timed_turns;
    if (b.effort_level) {
      automatic += b.turns;
      if (b.reasons.length) raised += b.turns;
      if (b.effort && b.effort !== b.effort_level) adjusted += b.turns;
      add(levels, b.effort_level, effortName(b.effort_level), b);
      for (const reason of new Set(b.reasons)) add(reasons, reason, reasonNames[reason] ?? reason, b);
    } else if (b.effort) chosen += b.turns;
    add(personas, b.persona_id, b.persona, b);
    add(models, `${b.harness}/${b.model}`, b.model ? `${b.model} · ${b.harness || "Unknown harness"}` : "Unknown model", b);
    add(efforts, b.effort, effortName(b.effort), b);
  }
  const ranked = (map: Map<string, HighlightRow>) => [...map.values()].sort((a, b) => b.turns - a.turns || a.key.localeCompare(b.key));
  const orderedEfforts = (map: Map<string, HighlightRow>) => [...map.values()].sort((a, b) => ["low", "medium", "high", "xhigh", "max", ""].indexOf(a.key) - ["low", "medium", "high", "xhigh", "max", ""].indexOf(b.key));
  return { turns, automatic, chosen, unknown: turns - automatic - chosen, raised, adjusted, timed, averageMs: timed ? duration / timed : undefined,
    personas: ranked(personas), models: ranked(models), efforts: orderedEfforts(efforts), levels: orderedEfforts(levels), reasons: ranked(reasons) };
}

// Costs page and cost chips (LAZYAGENTS-60). Every figure comes from
// GET /costs, which sums run counters server-side in integer micro-dollars, so
// these helpers only format, pick ranges and shape the chart.

export type CostPart = { key: string; cost_micros: number };
export type CostDay = { date: string; runs: number; cost_micros: number; agents: CostPart[]; models: CostPart[] };

export const RANGE_PRESETS = [
  { value: "7", label: "7 days" },
  { value: "30", label: "30 days" },
  { value: "90", label: "90 days" },
  { value: "custom", label: "Custom" },
] as const;
export type RangePreset = (typeof RANGE_PRESETS)[number]["value"];

/** The server accepts at most 366 days (inclusive). */
export const MAX_RANGE_DAYS = 366;

/** YYYY-MM-DD of a Date in UTC. */
export function utcDay(d: Date): string {
  return d.toISOString().slice(0, 10);
}

/** The last `days` UTC days, ending today. */
export function presetRange(days: number, now: Date): { from: string; to: string } {
  const to = new Date(Date.UTC(now.getUTCFullYear(), now.getUTCMonth(), now.getUTCDate()));
  const from = new Date(to.getTime() - (days - 1) * 86_400_000);
  return { from: utcDay(from), to: utcDay(to) };
}

const DAY_RE = /^\d{4}-\d{2}-\d{2}$/;

/** Why a custom range can't be queried, or undefined when it can. */
export function rangeError(from: string, to: string): string | undefined {
  if (!DAY_RE.test(from) || !DAY_RE.test(to) || Number.isNaN(Date.parse(from)) || Number.isNaN(Date.parse(to))) return "Pick a start and end day.";
  const span = (Date.parse(to) - Date.parse(from)) / 86_400_000 + 1;
  if (span < 1) return "The end day must not be before the start day.";
  if (span > MAX_RANGE_DAYS) return `A range is at most ${MAX_RANGE_DAYS} days.`;
  return undefined;
}

/** 412_370_000 → "$412.37"; spend under a cent keeps three decimals so it never reads $0.00. */
export function usd(micros: number): string {
  const v = micros / 1e6;
  if (v > 0 && v < 0.01) return v < 0.001 ? "< $0.001" : `$${v.toFixed(3)}`;
  return `$${v.toLocaleString("en-US", { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`;
}

/** Spend with unpriced runs: a lower bound, or Unknown when nothing was priced. Never reads as free. */
export function spendLabel(micros: number, unpricedRuns: number): string {
  if (unpricedRuns > 0) return micros > 0 ? `≥ ${usd(micros)}` : "Unknown";
  return usd(micros);
}

/** "40.8%" of the total; an empty total shows a dash. */
export function share(part: number, total: number): string {
  if (total <= 0) return "–";
  return `${((part / total) * 100).toFixed(1)}%`;
}

/** Cost per point, or undefined when there are no points. */
export function costPerPoint(micros: number, points: number | undefined | null): string | undefined {
  if (!points || points <= 0) return undefined;
  return usd(Math.round(micros / points));
}

export const OTHER = "Other";

/**
 * The chart's series: the `top` most expensive keys over the range, the rest
 * folded into Other. Each bar's segments add up to its day total.
 */
export function stackSeries(days: CostDay[], by: "agents" | "models", top = 6) {
  const totals = new Map<string, number>();
  for (const d of days) for (const p of d[by]) totals.set(p.key, (totals.get(p.key) ?? 0) + p.cost_micros);
  const ranked = [...totals.entries()].filter(([, v]) => v > 0).sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0]));
  const keep = ranked.length > top ? ranked.slice(0, top - 1) : ranked;
  const kept = new Set(keep.map(([k]) => k));
  const keys = keep.map(([k]) => k);
  const hasOther = ranked.length > keep.length;
  if (hasOther) keys.push(OTHER);
  const bars = days.map((d) => {
    const parts = new Map<string, number>();
    for (const p of d[by]) {
      const k = kept.has(p.key) ? p.key : OTHER;
      parts.set(k, (parts.get(k) ?? 0) + p.cost_micros);
    }
    return { date: d.date, runs: d.runs, total: d.cost_micros, segments: keys.map((k) => ({ key: k, cost_micros: parts.get(k) ?? 0 })) };
  });
  const seriesTotals = keys.map((k) => ({ key: k, cost_micros: k === OTHER ? ranked.slice(keep.length).reduce((s, [, v]) => s + v, 0) : (totals.get(k) ?? 0) }));
  return { keys, bars, totals: seriesTotals };
}

/** A round axis maximum (1, 2, 2.5 or 5 × 10ⁿ dollars) at or above the tallest bar. */
export function axisMax(maxMicros: number): number {
  if (maxMicros <= 0) return 1_000_000;
  const v = maxMicros / 1e6;
  const mag = 10 ** Math.floor(Math.log10(v));
  const step = [1, 2, 2.5, 5, 10].find((s) => s * mag >= v) ?? 10;
  return step * mag * 1e6;
}

/** "harness/model" → model, for chart legends keyed like the server's model slices. */
export function seriesLabel(key: string, by: "agents" | "models"): string {
  if (by === "agents" || key === OTHER) return key;
  const i = key.indexOf("/");
  return i < 0 ? key : key.slice(i + 1);
}

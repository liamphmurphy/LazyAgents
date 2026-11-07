import type { components } from "./api/schema";

type S = components["schemas"];
export type EpicOverview = S["EpicOverview"];
export type EpicHealth = S["Epic"]["health"];
export type EpicStatusFilter = "open" | "done" | "all";
export type EpicHealthFilter = "any" | EpicHealth;
export type EpicSort = "attention" | "target" | "progress" | "name";
export type EpicFilters = { status: EpicStatusFilter; health: EpicHealthFilter; sort: EpicSort };

export const EPIC_SORTS: Record<EpicSort, string> = {
  attention: "Needs attention",
  target: "Target date",
  progress: "Points progress",
  name: "Name",
};
export const HEALTH_LABEL: Record<EpicHealth, string> = { on_track: "On track", at_risk: "At risk", off_track: "Off track" };
const HEALTH_RANK: Record<EpicHealth, number> = { off_track: 0, at_risk: 1, on_track: 2 };
const pick = <T extends string>(value: string | null, allowed: readonly T[], fallback: T): T =>
  allowed.includes(value as T) ? (value as T) : fallback;

/** Overview filters live in the URL (?status=&health=&sort=) so reloads and shared links keep them. */
export function parseEpicFilters(params: { get(name: string): string | null }): EpicFilters {
  return {
    status: pick(params.get("status"), ["open", "done", "all"], "open"),
    health: pick(params.get("health"), ["any", "on_track", "at_risk", "off_track"], "any"),
    sort: pick(params.get("sort"), ["attention", "target", "progress", "name"], "attention"),
  };
}

/** Query string for the overview; defaults are left out so the plain URL stays /epics. */
export function epicFilterQuery(f: EpicFilters, project?: string): string {
  const q = new URLSearchParams();
  if (project) q.set("project", project);
  if (f.status !== "open") q.set("status", f.status);
  if (f.health !== "any") q.set("health", f.health);
  if (f.sort !== "attention") q.set("sort", f.sort);
  const s = q.toString();
  return s ? `?${s}` : "";
}

export function statusCounts(epics: EpicOverview[]): Record<EpicStatusFilter, number> {
  const done = epics.filter((e) => e.epic.status === "done").length;
  return { open: epics.length - done, done, all: epics.length };
}

export function filterEpics(epics: EpicOverview[], f: Pick<EpicFilters, "status" | "health">): EpicOverview[] {
  return epics.filter((e) => (f.status === "all" || e.epic.status === f.status) && (f.health === "any" || e.epic.health === f.health));
}

/** Done share in [0, 1]; 0 when nothing is estimated. */
export function ratio(done: number, total: number): number {
  return total > 0 ? Math.min(1, Math.max(0, done / total)) : 0;
}

const byTarget = (a: EpicOverview, b: EpicOverview) =>
  a.target_date && b.target_date ? a.target_date.localeCompare(b.target_date) : a.target_date ? -1 : b.target_date ? 1 : 0;
const byName = (a: EpicOverview, b: EpicOverview) => a.epic.name.localeCompare(b.epic.name);

/** Needs attention: off track, at risk, on track, then the nearest target date (none last), then name. */
export function sortEpics(epics: EpicOverview[], sort: EpicSort): EpicOverview[] {
  const out = [...epics];
  const cmp: Record<EpicSort, (a: EpicOverview, b: EpicOverview) => number> = {
    attention: (a, b) => HEALTH_RANK[a.epic.health] - HEALTH_RANK[b.epic.health] || byTarget(a, b) || byName(a, b),
    target: (a, b) => byTarget(a, b) || byName(a, b),
    progress: (a, b) => ratio(b.epic.points_done, b.epic.points_total) - ratio(a.epic.points_done, a.epic.points_total) || byName(a, b),
    name: byName,
  };
  return out.sort(cmp[sort]);
}

/** Local calendar day as YYYY-MM-DD. */
export function ymd(d: Date): string {
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
}

/** An open epic past its target date. */
export function isOverdue(e: Pick<EpicOverview, "target_date"> & { epic: { status: S["EpicStatus"] } }, today: string): boolean {
  return e.epic.status === "open" && !!e.target_date && e.target_date < today;
}

export type BurnUpPoint = { date: string; done: number };
export type BurnUp = { scope: number; done: number; points: BurnUpPoint[] };

/**
 * Points burn-up from the epic's tickets. A done ticket counts on the day it
 * was last updated (the API has no status history), and scope is today's
 * estimated total, canceled excluded. The series starts at the epic's start
 * date (or the first completion) at 0 and ends today.
 */
export function burnUp(
  tickets: Array<Pick<S["TicketSummary"], "status" | "estimate_points" | "updated_at">>,
  { start, today }: { start?: string; today: string },
): BurnUp {
  const live = tickets.filter((t) => t.status !== "canceled");
  const scope = live.reduce((n, t) => n + (t.estimate_points ?? 0), 0);
  const perDay = new Map<string, number>();
  for (const t of live) {
    if (t.status !== "done" || !t.estimate_points) continue;
    const day = ymd(new Date(t.updated_at));
    perDay.set(day, (perDay.get(day) ?? 0) + t.estimate_points);
  }
  const days = [...perDay.keys()].sort();
  const first = start && (!days.length || start <= days[0]) ? start : days[0] ?? today;
  const points: BurnUpPoint[] = [{ date: first, done: 0 }];
  let done = 0;
  for (const day of days) {
    done += perDay.get(day)!;
    if (points.at(-1)!.date === day) points.at(-1)!.done = done;
    else points.push({ date: day, done });
  }
  if (points.at(-1)!.date < today) points.push({ date: today, done });
  return { scope, done, points };
}

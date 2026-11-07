// Pure logic for the tickets list: grouping, local filters, saved-view queries,
// keyboard selection and per-ticket bulk results. Type-only imports so node
// can run it directly in unit tests.
import type { components } from "./api/schema";

type TicketSummary = components["schemas"]["TicketSummary"];
type TicketStatus = components["schemas"]["TicketStatus"];
type Priority = components["schemas"]["Priority"];

export const STATUS_GROUP_ORDER: TicketStatus[] = ["in_progress", "in_review", "todo", "backlog", "done", "canceled"];
export const PRIORITY_GROUP_ORDER: Priority[] = ["urgent", "high", "medium", "low", "none"];

export const GROUPS = ["status", "epic", "assignee", "priority"] as const;
export type GroupBy = (typeof GROUPS)[number];
export const GROUP_NAME: Record<GroupBy, string> = { status: "Status", epic: "Epic", assignee: "Assignee", priority: "Priority" };

/** ?group= value; unknown or missing means the default, status. */
export function parseGroup(v: string | null | undefined): GroupBy {
  return (GROUPS as readonly string[]).includes(v ?? "") ? (v as GroupBy) : "status";
}

export type TicketGroup = {
  /** Stable id: the status, priority, epic or agent id, or "none". */
  id: string;
  by: GroupBy;
  /** Epic or assignee name; empty for status/priority (the screen names those). */
  name: string;
  tickets: TicketSummary[];
  criteriaDone: number;
  criteriaTotal: number;
  points: number;
};

function summarize(id: string, by: GroupBy, name: string, tickets: TicketSummary[]): TicketGroup {
  let criteriaDone = 0;
  let criteriaTotal = 0;
  let points = 0;
  for (const t of tickets) {
    criteriaDone += t.criteria_done;
    criteriaTotal += t.criteria_total;
    if (t.status !== "canceled") points += t.estimate_points ?? 0;
  }
  return { id, by, name, tickets, criteriaDone, criteriaTotal, points };
}

/**
 * Groups keep the server's order within each group. Status and priority use
 * their fixed order; epic and assignee groups sort A–Z with "none" last.
 */
export function groupTickets(tickets: TicketSummary[], by: GroupBy): TicketGroup[] {
  const buckets = new Map<string, { name: string; tickets: TicketSummary[] }>();
  const add = (id: string, name: string, t: TicketSummary) => {
    const b = buckets.get(id);
    if (b) b.tickets.push(t);
    else buckets.set(id, { name, tickets: [t] });
  };
  for (const t of tickets) {
    if (by === "status") add(t.status, "", t);
    else if (by === "priority") add(t.priority, "", t);
    else if (by === "epic") add(t.epic?.id ?? "none", t.epic?.name ?? "", t);
    else add(t.assignee?.id ?? "none", t.assignee?.name ?? "", t);
  }
  let ids: string[];
  if (by === "status") ids = STATUS_GROUP_ORDER.filter((s) => buckets.has(s));
  else if (by === "priority") ids = PRIORITY_GROUP_ORDER.filter((p) => buckets.has(p));
  else {
    ids = [...buckets.keys()]
      .filter((id) => id !== "none")
      .sort((a, b) => buckets.get(a)!.name.localeCompare(buckets.get(b)!.name, undefined, { sensitivity: "base" }) || a.localeCompare(b));
    if (buckets.has("none")) ids.push("none");
  }
  return ids.map((id) => summarize(id, by, buckets.get(id)!.name, buckets.get(id)!.tickets));
}

/** Blocked means an open ticket with a blocker that isn't done. */
export function isBlocked(t: TicketSummary) {
  return t.status !== "done" && t.status !== "canceled" && t.unmet_dependencies > 0;
}

/** Filters the API can't apply: ?blocked=true and ?unestimated=true. */
export function applyLocalFilters(tickets: TicketSummary[], f: { blocked?: boolean; unestimated?: boolean }) {
  return tickets.filter(
    (t) =>
      (!f.blocked || isBlocked(t)) &&
      (!f.unestimated || (t.estimate_points == null && t.status !== "done" && t.status !== "canceled")),
  );
}

/** The URL params a saved view keeps, in a fixed order. project and sv never are. */
export const VIEW_PARAMS = ["view", "status", "group", "assignee", "epic", "q", "ready", "blocked", "unestimated"] as const;

/** Normalized saved-view query from the list's URL params; defaults are dropped. */
export function viewQuery(params: URLSearchParams | string): string {
  const p = typeof params === "string" ? new URLSearchParams(params.replace(/^\?/, "")) : params;
  const out = new URLSearchParams();
  for (const k of VIEW_PARAMS) {
    const v = p.get(k);
    if (!v) continue;
    if (k === "view" && v === "all") continue;
    if (k === "group" && v === "status") continue;
    if ((k === "ready" || k === "blocked" || k === "unestimated") && v !== "true") continue;
    out.set(k, v);
  }
  return out.toString();
}

/** The /tickets URL that opens a saved view in a project. */
export function savedViewHref(view: { id: string; query: string }, project?: string | null) {
  const p = new URLSearchParams();
  if (project) p.set("project", project);
  p.set("sv", view.id);
  for (const [k, v] of new URLSearchParams(viewQuery(view.query))) p.set(k, v);
  return `/tickets?${p}`;
}

/** Human summary of a saved-view query, e.g. "View Active · Group Epic · Agent any". */
export function describeQuery(query: string, names: { view?: (v: string) => string; epic?: (id: string) => string } = {}) {
  const p = new URLSearchParams(viewQuery(query));
  const parts = [`View ${names.view?.(p.get("view") ?? "all") ?? p.get("view") ?? "all"}`];
  parts.push(`Group ${GROUP_NAME[parseGroup(p.get("group"))]}`);
  parts.push(`Agent ${p.get("assignee") ?? "any"}`);
  if (p.get("status")) parts.push(`Status ${p.get("status")!.replace("_", " ")}`);
  if (p.get("epic")) parts.push(`Epic ${names.epic?.(p.get("epic")!) ?? "selected"}`);
  if (p.get("ready")) parts.push("Ready to dispatch");
  if (p.get("blocked")) parts.push("Blocked");
  if (p.get("unestimated")) parts.push("Unestimated");
  parts.push(p.get("q") ? `Search “${p.get("q")}”` : "Search none");
  return parts.join(" · ");
}

/** Suggested saved views: names and filters that fill the save dialog. */
export const SUGGESTED_VIEWS: ReadonlyArray<{ name: string; query: string; hint: string }> = [
  { name: "My review queue", query: "status=in_review", hint: "In review assigned to anyone" },
  { name: "Blocked", query: "blocked=true", hint: "Blocked by an open dependency" },
  { name: "Unestimated", query: "unestimated=true", hint: "No estimate yet (done and canceled excluded)" },
];

/** Next focused key for j/k; starts at the first (or last) row when nothing is focused. */
export function moveFocus(keys: string[], current: string | null, delta: 1 | -1): string | null {
  if (keys.length === 0) return null;
  const i = current ? keys.indexOf(current) : -1;
  if (i < 0) return delta > 0 ? keys[0] : keys[keys.length - 1];
  return keys[Math.min(keys.length - 1, Math.max(0, i + delta))];
}

/** Keys between a and b inclusive, in list order; just b when a isn't listed. */
export function rangeKeys(keys: string[], a: string | null, b: string): string[] {
  const i = a ? keys.indexOf(a) : -1;
  const j = keys.indexOf(b);
  if (j < 0) return [];
  if (i < 0) return [b];
  return keys.slice(Math.min(i, j), Math.max(i, j) + 1);
}

export type BulkFailure = { key: string; status: number; message: string };
export type BulkResult = { label: string; updated: string[]; failed: BulkFailure[] };

/**
 * Applies fn to every key on its own, a few at a time. Successes stay applied;
 * each failure keeps its HTTP status and message.
 */
export async function runEach(
  keys: string[],
  fn: (key: string) => Promise<unknown>,
  concurrency = 6,
): Promise<{ updated: string[]; failed: BulkFailure[] }> {
  const outcome = new Map<string, BulkFailure | null>();
  let next = 0;
  async function worker() {
    while (next < keys.length) {
      const key = keys[next++];
      try {
        await fn(key);
        outcome.set(key, null);
      } catch (e) {
        const err = e as { status?: number; message?: string };
        outcome.set(key, { key, status: err.status ?? 0, message: err.message || "Request failed" });
      }
    }
  }
  await Promise.all(Array.from({ length: Math.min(concurrency, keys.length) }, worker));
  const updated: string[] = [];
  const failed: BulkFailure[] = [];
  for (const k of keys) {
    const f = outcome.get(k);
    if (f) failed.push(f);
    else updated.push(k);
  }
  return { updated, failed };
}

/** Friendlier reason for well-known per-ticket failures. */
export function failureReason(f: BulkFailure) {
  if (f.status === 409 && /cancel the run/i.test(f.message)) return "Cancel the run first: a run is live on this ticket.";
  return f.message.charAt(0).toUpperCase() + f.message.slice(1);
}

// Pure view logic for the run page and the runs index.

export type Ev = { seq: number; type: string; at: string; payload: unknown };
type P = Record<string, unknown>;

export const str = (v: unknown) => (v == null ? "" : typeof v === "string" ? v : JSON.stringify(v));

// ------------------------------------------------------------------ tools

/** "lazyagents.claim_ticket" for MCP tools, the tool name otherwise. */
export function toolName(t: string) {
  const m = /^mcp__([^_]+(?:_[^_]+)*?)__(.+)$/.exec(t);
  return m ? `${m[1]}.${m[2]}` : t;
}

/** A compact argument summary: `(ticket: "PAY-214")`, or the command for shells. */
export function argSummary(input: unknown) {
  if (!input || typeof input !== "object") return "";
  const o = input as P;
  if (typeof o.command === "string") return o.command;
  if (typeof o.file_path === "string") return o.file_path;
  const parts = Object.entries(o)
    .slice(0, 3)
    .map(([k, v]) => `${k}: ${typeof v === "string" ? JSON.stringify(v.length > 40 ? v.slice(0, 40) + "…" : v) : Array.isArray(v) ? `[${v.length}]` : JSON.stringify(v)}`);
  return `(${parts.join(", ")}${Object.keys(o).length > 3 ? ", …" : ""})`;
}

export type ToolRow<E extends Ev = Ev> = { kind: "tool"; call: E; result?: E };
export type EventRow<E extends Ev = Ev> = { kind: "event"; e: E };
export type Row<E extends Ev = Ev> = EventRow<E> | ToolRow<E>;

/** Pairs each tool call with its result so they read as one row. */
export function rowsOf<E extends Ev>(events: E[]): Row<E>[] {
  const out: Row<E>[] = [];
  const open = new Map<string, ToolRow<E>>();
  for (const e of events) {
    const p = e.payload as P;
    if (e.type === "tool_call") {
      const row: ToolRow<E> = { kind: "tool", call: e };
      if (p.id) open.set(str(p.id), row);
      out.push(row);
    } else if (e.type === "tool_result" && p.id && open.has(str(p.id))) {
      open.get(str(p.id))!.result = e;
    } else {
      out.push({ kind: "event", e });
    }
  }
  return out;
}

export function rowEvent<E extends Ev>(r: Row<E>): E {
  return r.kind === "tool" ? r.call : r.e;
}

/** Link target for tools that read a wiki page or an artifact. */
export function toolLink(tool: string, input: unknown): { href: string; label: string } | null {
  const o = (input && typeof input === "object" ? input : {}) as P;
  const name = toolName(tool);
  if (name === "lazyagents.read_page" && typeof o.ref === "string" && o.ref) {
    return { href: `/wiki/${o.ref.split("/").map(encodeURIComponent).join("/")}`, label: o.ref };
  }
  if (name === "lazyagents.read_artifact" && typeof o.artifact === "string" && o.artifact) {
    return { href: `/artifacts/${encodeURIComponent(o.artifact)}`, label: o.artifact };
  }
  return null;
}

type ChildRef = { id: string; started_at: string; agent: { handle: string } };

/** Best effort: the child run with that agent handle that started closest after the call. */
export function delegateChild<C extends ChildRef>(call: Ev, children: C[], calls: Ev[] = [call]): C | undefined {
  if (toolName(str((call.payload as P).tool)) !== "lazyagents.delegate") return undefined;
  const handle = str(((call.payload as P).input as P | undefined)?.agent).replace(/^@/, "");
  if (!handle) return undefined;
  const used = new Set<string>();
  for (const prior of [...calls].sort((a, b) => a.seq - b.seq)) {
    if (prior.seq >= call.seq) break;
    const child = delegateChild(prior, children.filter((c) => !used.has(c.id)));
    if (child) used.add(child.id);
  }
  const at = Date.parse(call.at);
  const after = children
    .filter((c) => !used.has(c.id) && c.agent.handle === handle && Date.parse(c.started_at) >= at - 1000)
    .sort((a, b) => Date.parse(a.started_at) - Date.parse(b.started_at));
  return after[0];
}

export function isFailedRow(r: Row): boolean {
  return r.kind === "tool" && (!!(r.call.payload as P).denied_by_harness || !!(r.result?.payload as P | undefined)?.is_error);
}

// ------------------------------------------------------------------ turns

/** Turn number of each turn_started marker, with its time. */
export function turnStarts(events: Ev[]): { turn: number; at: string }[] {
  return events
    .filter((e) => e.type === "lifecycle" && (e.payload as P).state === "turn_started")
    .map((e) => ({ turn: Number((e.payload as P).turn), at: e.at }))
    .filter((m) => Number.isFinite(m.turn));
}

/** The turn an event of the run itself belongs to, by sequence order. 0 is setup (before the first marker). */
export function turnByEvent(events: Ev[]): Map<number, number> {
  const out = new Map<number, number>();
  let turn = 0;
  for (const e of [...events].sort((a, b) => a.seq - b.seq)) {
    if (e.type === "lifecycle" && (e.payload as P).state === "turn_started" && Number.isFinite(Number((e.payload as P).turn))) {
      turn = Number((e.payload as P).turn);
    }
    out.set(e.seq, turn);
  }
  return out;
}

/** The turn a delegated run's event falls in, by time. */
export function turnAt(starts: { turn: number; at: string }[], at: string): number {
  const t = Date.parse(at);
  let turn = 0;
  for (const s of starts) if (Date.parse(s.at) <= t && s.turn >= turn) turn = s.turn;
  return turn;
}

export type TurnGroup<T> = { turn: number; items: T[] };

/** Groups items by turn, ascending; items keep their order. */
export function groupByTurn<T>(items: T[], turnOf: (item: T) => number): TurnGroup<T>[] {
  const groups = new Map<number, T[]>();
  for (const it of items) {
    const n = turnOf(it);
    groups.set(n, [...(groups.get(n) ?? []), it]);
  }
  return [...groups.entries()].sort((a, b) => a[0] - b[0]).map(([turn, list]) => ({ turn, items: list }));
}

export function triggerLine(trigger: string, input: string | undefined, labels: Record<string, string>) {
  const first = (input ?? "").split("\n")[0];
  return `${labels[trigger] ?? trigger}${trigger !== "start" && first ? `: ${first}` : ""}`;
}

// ------------------------------------------------------------------ usage

type UsageRun = { model: string; cost_usd: number; cost_source?: string | null; input_tokens: number; output_tokens: number };

export type ModelUsage = { model: string; runs: number; cost: number; unpriced: boolean };

/** This run and its children grouped by model. */
export function usageByModel(runs: UsageRun[]): ModelUsage[] {
  const m = new Map<string, ModelUsage>();
  for (const r of runs) {
    const cur = m.get(r.model) ?? { model: r.model, runs: 0, cost: 0, unpriced: false };
    cur.runs += 1;
    cur.cost += r.cost_usd || 0;
    if (!r.cost_source || r.cost_source === "unknown" || r.cost_source === "partial") cur.unpriced = true;
    m.set(r.model, cur);
  }
  return [...m.values()].sort((a, b) => b.cost - a.cost || a.model.localeCompare(b.model));
}

export function subAgentTotals(children: UsageRun[]) {
  return children.reduce(
    (t, c) => ({ cost: t.cost + (c.cost_usd || 0), input: t.input + c.input_tokens, output: t.output + c.output_tokens, unpriced: t.unpriced || !c.cost_source || c.cost_source === "unknown" }),
    { cost: 0, input: 0, output: 0, unpriced: false },
  );
}

/** 0..1 share of input served from cache; null when unknown. */
export function cacheShare(input: number, cached: number | undefined): number | null {
  if (cached === undefined || !(input > 0)) return null;
  return Math.max(0, Math.min(1, cached / input));
}

// --------------------------------------------------------------- filters

export const RUN_STATES = ["live", "all", "waiting", "running", "queued", "starting", "blocked", "completed", "failed", "cancelled", "paused"] as const;
export const RUN_PURPOSES = ["ticket", "planning", "chat"] as const;

export type RunFilters = { state: string; agent?: string; project?: string; purpose?: "ticket" | "planning" | "chat" };

type Params = { get(name: string): string | null };

export function parseRunFilters(params: Params): RunFilters {
  const state = params.get("state");
  const purpose = params.get("purpose");
  return {
    state: state && (RUN_STATES as readonly string[]).includes(state) ? state : "live",
    agent: params.get("agent") || undefined,
    project: params.get("project") || undefined,
    purpose: (RUN_PURPOSES as readonly string[]).includes(purpose ?? "") ? (purpose as RunFilters["purpose"]) : undefined,
  };
}

/** Defaults (live, nothing else) are left out so URLs stay short. */
export function runFilterQuery(f: RunFilters): string {
  const p = new URLSearchParams();
  if (f.state && f.state !== "live") p.set("state", f.state);
  if (f.agent) p.set("agent", f.agent);
  if (f.project) p.set("project", f.project);
  if (f.purpose) p.set("purpose", f.purpose);
  return p.toString();
}

export function hasActiveFilters(f: RunFilters): boolean {
  return f.state !== "live" || !!f.agent || !!f.project || !!f.purpose;
}

// ------------------------------------------------------------ work links

type WorkRun = { purpose: string; ticket_key: string; planning_id?: string; chat_id?: string; project_key?: string };

/** Where the work of a run lives; never a /tickets/ link without a key. */
export function workHref(r: WorkRun): string | null {
  if (r.purpose === "ticket" && r.ticket_key) return `/tickets/${encodeURIComponent(r.ticket_key)}`;
  if (r.purpose === "planning" && r.planning_id) return `/plans?id=${r.planning_id}`;
  if (r.purpose === "chat" && r.chat_id) return `/chat/${r.chat_id}${r.project_key ? `?project=${encodeURIComponent(r.project_key)}` : ""}`;
  return null;
}

export const PURPOSE_LABEL: Record<string, string> = { ticket: "Ticket run", planning: "Planning run", chat: "Chat run" };

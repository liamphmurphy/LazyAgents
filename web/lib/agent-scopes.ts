/**
 * Pure helpers for the agent pages: effective scope levels, change
 * classification, collapsed diffs and directory counts. No runtime imports,
 * so node's test runner loads this file directly (lib/agent-scopes.test.mjs).
 */

export type Mode = "allowed" | "requires_approval" | "denied";
export type ScopeMap = Partial<Record<string, Mode>>;

const RANK: Record<Mode, number> = { allowed: 0, requires_approval: 1, denied: 2 };

export function stricter(a: Mode, b: Mode): Mode {
  return RANK[b] > RANK[a] ? b : a;
}

/** "db:*" covers every db: scope; anything else matches only itself. */
export function matches(pattern: string, scope: string): boolean {
  if (pattern === scope) return true;
  return pattern.endsWith(":*") && scope.startsWith(pattern.slice(0, -1));
}

/** The strictest matching entry (exact or wildcard). Unknown scopes are denied. */
export function modeFor(scopes: ScopeMap | undefined, scope: string): { mode: Mode; found: boolean } {
  let mode: Mode | undefined;
  for (const [pattern, m] of Object.entries(scopes ?? {})) {
    if (m && matches(pattern, scope)) mode = mode ? stricter(mode, m) : m;
  }
  return mode ? { mode, found: true } : { mode: "denied", found: false };
}

export type Effective = { mode: Mode; source: "agent" | "project" };

/**
 * One scope's level in a project, merged the way the server does
 * (server/internal/agentdef/resolve.go Resolve): a project can only tighten,
 * and a scope the agent doesn't grant is denied. The server stays
 * authoritative; this lets unsaved edits preview correctly.
 */
export function effectiveMode(agent: ScopeMap, override: ScopeMap | undefined, scope: string): Effective {
  const base = modeFor(agent, scope);
  const p = modeFor(override, scope);
  if (p.found) {
    const t = stricter(base.mode, p.mode);
    if (t !== base.mode || !base.found) return { mode: t, source: "project" };
  }
  return { mode: base.mode, source: "agent" };
}

export type ScopeRow = {
  scope: string;
  /** The definition's own entry for exactly this scope; undefined = not granted by name. */
  definition?: Mode;
  /** The agent grants it through another entry, e.g. a wildcard. */
  granted: boolean;
  effective: Effective;
  /** Named only by the project override. */
  projectOnly: boolean;
};

/** Matrix rows: catalog scopes in catalog order, then the agent's custom scopes, then ones only the project names. */
export function scopeRows(catalog: string[], agent: ScopeMap, override: ScopeMap | undefined): ScopeRow[] {
  const seen = new Set<string>();
  const names: string[] = [];
  const add = (s: string) => {
    if (!seen.has(s)) {
      seen.add(s);
      names.push(s);
    }
  };
  catalog.forEach(add);
  Object.keys(agent).filter((s) => agent[s]).sort().forEach(add);
  Object.keys(override ?? {}).filter((s) => override?.[s]).sort().forEach(add);
  return names.map((scope) => ({
    scope,
    definition: agent[scope],
    granted: modeFor(agent, scope).found,
    effective: effectiveMode(agent, override, scope),
    projectOnly: !(scope in agent) && !catalog.includes(scope) && !!override?.[scope],
  }));
}

export type ScopeChangeKind = "approval" | "widens" | "tightens";

/**
 * How a definition change reads at run time. Not granted counts as denied.
 * A change to Approval always says so, since every use will pause the run.
 */
export function classifyChange(from: Mode | undefined, to: Mode | undefined): ScopeChangeKind {
  const a = from ?? "denied";
  const b = to ?? "denied";
  if (b === "requires_approval") return "approval";
  return RANK[b] < RANK[a] ? "widens" : "tightens";
}

export type ScopeChange = { scope: string; from?: Mode; to?: Mode; kind: ScopeChangeKind };

export function scopeChanges(before: ScopeMap, after: ScopeMap): ScopeChange[] {
  const names = [...new Set([...Object.keys(before), ...Object.keys(after)])].sort();
  return names
    .filter((s) => before[s] !== after[s])
    .map((s) => ({ scope: s, from: before[s], to: after[s], kind: classifyChange(before[s], after[s]) }));
}

export function countModes(scopes: Array<{ mode: Mode }>): Record<Mode, number> {
  const out: Record<Mode, number> = { allowed: 0, requires_approval: 0, denied: 0 };
  for (const s of scopes) out[s.mode]++;
  return out;
}

type DiffLine = { op: "same" | "add" | "del"; text: string };
export type DiffRow =
  | { kind: "line"; op: DiffLine["op"]; text: string; oldNo?: number; newNo?: number }
  | { kind: "gap"; count: number };

/** Numbers diff lines and folds unchanged runs longer than 2×context into a gap. */
export function collapseDiff(lines: DiffLine[], context = 2): DiffRow[] {
  let o = 0;
  let n = 0;
  const numbered = lines.map((l) => {
    if (l.op !== "add") o++;
    if (l.op !== "del") n++;
    return { kind: "line" as const, op: l.op, text: l.text, oldNo: l.op === "add" ? undefined : o, newNo: l.op === "del" ? undefined : n };
  });
  const keep = numbered.map((l, i) => {
    if (l.op !== "same") return true;
    for (let d = -context; d <= context; d++) {
      const x = numbered[i + d];
      if (x && x.op !== "same") return true;
    }
    return false;
  });
  const out: DiffRow[] = [];
  for (let i = 0; i < numbered.length; i++) {
    if (keep[i]) {
      out.push(numbered[i]);
      continue;
    }
    let j = i;
    while (j < numbered.length && !keep[j]) j++;
    out.push({ kind: "gap", count: j - i });
    i = j - 1;
  }
  return out;
}

/** Directory counts. Live runs include planning runs; open tickets are queued assignments plus tickets with a live run. */
export function liveCounts(o: {
  assignments: Array<{ ticket: { key: string } }>;
  planning_runs: unknown[];
  queue_count: number;
}): { live: number; open: number } {
  return {
    live: o.assignments.length + o.planning_runs.length,
    open: o.queue_count + new Set(o.assignments.map((a) => a.ticket.key)).size,
  };
}

export type SkillLike = { name: string; description: string; body: string };
export type SkillView<S extends SkillLike> = { skill: S; origin: "agent" | "removed" | "added" };

/**
 * The agent's (draft) skills with project removals marked, then the
 * project's additions. saved = skill names in the saved definition, which is
 * what the server resolved against.
 */
export function skillViews<S extends SkillLike>(draft: S[], saved: string[], resolved: S[] | undefined): SkillView<S>[] {
  if (!resolved) return draft.map((skill) => ({ skill, origin: "agent" }));
  const res = new Set(resolved.map((k) => k.name));
  const ours = new Set(saved);
  return [
    ...draft.map((skill) => ({ skill, origin: ours.has(skill.name) && !res.has(skill.name) ? ("removed" as const) : ("agent" as const) })),
    ...resolved.filter((k) => !ours.has(k.name)).map((skill) => ({ skill, origin: "added" as const })),
  ];
}

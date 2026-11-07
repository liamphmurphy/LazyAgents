import type { Ticket } from "./api/client";

export type MapTicket = Pick<Ticket, "key" | "title" | "status" | "estimate_points" | "blocked_by"> & { external?: boolean };

/** Stable topological columns and a points-weighted unfinished path. Cycles are reported, never hidden. */
export function dependencyMap(tickets: MapTicket[]) {
  const byKey = new Map(tickets.map((t) => [t.key, t]));
  const depth = new Map<string, number>();
  const paths = new Map<string, { keys: string[]; points: number }>();
  const remaining = new Set(byKey.keys());
  const edges = tickets.flatMap((t) => t.blocked_by.filter((b) => byKey.has(b.key)).map((b) => ({ from: b.key, to: t.key })));
  while (remaining.size) {
    let advanced = false;
    for (const key of remaining) {
      const ticket = byKey.get(key)!;
      const parents = edges.filter((e) => e.to === key).map((e) => e.from);
      if (parents.some((p) => !depth.has(p))) continue;
      depth.set(key, parents.length ? Math.max(...parents.map((p) => depth.get(p)!)) + 1 : 0);
      const unfinished = ticket.status !== "done" && ticket.status !== "canceled";
      const best = parents.map((p) => paths.get(p)!).sort((a, b) => b.points - a.points || b.keys.length - a.keys.length)[0];
      // Completed/canceled nodes break the unfinished chain.
      paths.set(key, unfinished ? { keys: [...(best?.keys ?? []), key], points: (best?.points ?? 0) + (ticket.estimate_points ?? 0) } : { keys: [], points: 0 });
      remaining.delete(key);
      advanced = true;
    }
    if (!advanced) break;
  }
  const cyclic = [...remaining];
  for (const key of cyclic) depth.set(key, 0);
  const chain = [...paths.values()].sort((a, b) => b.points - a.points || b.keys.length - a.keys.length)[0] ?? { keys: [], points: 0 };
  const nodes = [...tickets].sort((a, b) => depth.get(a.key)! - depth.get(b.key)! || a.key.localeCompare(b.key, undefined, { numeric: true }));
  const columns: MapTicket[][] = [];
  nodes.forEach((t) => (columns[depth.get(t.key)!] ??= []).push(t));
  const warnings = edges.filter((e) => byKey.get(e.from)?.status === "canceled" && !["done", "canceled"].includes(byKey.get(e.to)!.status));
  return { nodes, columns, edges, depth, chain, warnings, cyclic };
}

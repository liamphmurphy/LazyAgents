"use client";

import { useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, unwrap, type Ticket } from "@/lib/api/client";
import { useBootstrap } from "@/lib/api/hooks";
import { useTicketEditorDetail } from "@/lib/api/board-hooks";
import { dependencyMap, type MapTicket } from "@/lib/dependency-map";
import { TicketEditorDialog } from "./ticket-editor-dialog";
import { STATUS_NAME } from "./meta";
import { StatusGlyph } from "./glyphs";

export function DependencyMap({ project, epic, assignee, includeCanceled = true, onOpen }: {
  project: string; epic?: string; assignee?: string; includeCanceled?: boolean; onOpen?: (key: string) => void;
}) {
  const [selected, setSelected] = useState<string>();
  const [list, setList] = useState(false);
  const refs = useRef(new Map<string, HTMLButtonElement>());
  const bootstrap = useBootstrap();
  const editor = useTicketEditorDetail(selected);
  const qc = useQueryClient();
  const query = useQuery({
    queryKey: ["tickets", "dependency-map", project, epic, assignee, includeCanceled],
    queryFn: async ({ signal }) => {
      const summary = await unwrap(api.GET("/tickets", { params: { query: { project, epic, assignee, view: "all" } }, signal }));
      const primary = summary.tickets.filter((t) => includeCanceled || t.status !== "canceled");
      const all = new Map<string, MapTicket>();
      // Bounded batches avoid flooding the API for large projects; recurse through external blockers.
      const pending = primary.map((t) => t.key);
      const inside = new Set(pending);
      const seen = new Set<string>();
      while (pending.length) {
        const batch = pending.splice(0, 12).filter((key) => !seen.has(key));
        batch.forEach((key) => seen.add(key));
        const details = await Promise.all(batch.map((key) => unwrap(api.GET("/tickets/{key}", { params: { path: { key } }, signal }))));
        for (const t of details) {
          all.set(t.key, { ...t, external: !inside.has(t.key) });
          pending.push(...t.blocked_by.map((b) => b.key).filter((key) => !seen.has(key)));
        }
      }
      return [...all.values()];
    },
  });
  const remove = useMutation({
    mutationFn: async ({ from, to }: { from: string; to: string }) => {
      const t = await unwrap(api.GET("/tickets/{key}", { params: { path: { key: to } } }));
      return unwrap(api.PUT("/tickets/{key}/dependencies", { params: { path: { key: to } }, body: {
        revision: t.dependency_revision, blocked_by: t.blocked_by.filter((b) => b.key !== from).map((b) => b.key),
      } }));
    },
    onSettled: async () => { await Promise.all(["tickets", "boards", "bootstrap", "epics", "planning", "inbox", "dashboard", "agent-overviews"].map((key) => qc.invalidateQueries({ queryKey: [key] }))); },
  });
  const open = (key: string) => onOpen ? onOpen(key) : setSelected(key);
  if (query.isPending) return <p role="status" className="p-5">Loading dependency map…</p>;
  if (query.isError) return <p role="alert" className="p-5">Couldn&apos;t load dependency map: {query.error.message} <button onClick={() => void query.refetch()} className="underline">Retry</button></p>;
  const graph = dependencyMap(query.data);
  const positions = new Map(graph.columns.flatMap((column, d) => column.map((t, row) => [t.key, { x: d * 270 + 20, y: row * 108 + 44 }] as const)));
  const criticalEdge = (from: string, to: string) => graph.chain.keys.indexOf(to) > 0 && graph.chain.keys[graph.chain.keys.indexOf(to) - 1] === from;
  const width = Math.max(1, graph.columns.length) * 270;
  const height = Math.max(1, ...graph.columns.map((c) => c.length)) * 108 + 50;
  return <section aria-label="Dependency map" className="space-y-4">
    <div className="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-border bg-surface p-4">
      <div><p className="font-semibold">Longest unfinished chain: {graph.chain.keys.length} tickets · {graph.chain.points} pts</p>
        <p className="mt-1 text-xs text-fg-secondary">{graph.chain.keys.join(" → ") || "No unfinished tickets"}</p>
        <p className="text-xs text-fg-tertiary">Largest remaining point total; done and canceled tickets excluded. Unestimated tickets count as 0 pts.</p></div>
      <div className="flex gap-3"><button aria-pressed={!list} onClick={() => setList(false)}>Graph</button><button aria-pressed={list} onClick={() => setList(true)}>List</button></div>
    </div>
    {!!graph.cyclic.length && <p role="alert">Dependency cycle detected. Review these tickets: {graph.cyclic.join(", ")}. The unfinished chain excludes cycles and their dependents.</p>}
    {!!graph.warnings.length && <div className="rounded-lg border border-state-blocked p-3 text-sm">
      <p className="font-semibold text-state-blocked">⚠ Canceled blockers still block work</p>
      {graph.warnings.map((e) => <div key={`${e.from}-${e.to}`} className="mt-2 flex flex-wrap items-center gap-3">
        <span>{e.from} still blocks {e.to}</span><button disabled={remove.isPending} className="underline disabled:opacity-50" onClick={() => remove.mutate(e)}>Remove {e.from} from {e.to}</button>
        <button className="underline" onClick={() => open(e.from)}>Open {e.from} to reopen</button>
      </div>)}
    </div>}
    {remove.isError && <p role="alert">Couldn&apos;t remove dependency: {remove.error.message}. The map has refreshed; review and try again.</p>}
    {remove.isSuccess && <p role="status" className="text-xs">Dependency removed.</p>}
    {!graph.nodes.length && <p>No tickets match these filters.</p>}
    {!list && <>
      <p className="text-xs text-fg-secondary">Click or Enter opens a ticket. Tab traverses nodes; ←/→ follow dependencies; ↑/↓ move within a column. Purple: longest unfinished chain. Dashed red: canceled blocker. Faded: outside filter.</p>
      <div className="overflow-auto rounded-lg border border-border bg-surface" aria-label="Dependency graph">
        <div className="relative" style={{ width, height }}>
          <svg width={width} height={height} className="absolute inset-0" aria-hidden="true">
            {graph.edges.map((e) => {
              const a = positions.get(e.from)!, b = positions.get(e.to)!;
              const canceled = graph.warnings.some((w) => w.from === e.from && w.to === e.to);
              return <path key={`${e.from}-${e.to}`} d={`M${a.x + 230},${a.y + 38} C${a.x + 254},${a.y + 38} ${b.x - 24},${b.y + 38} ${b.x},${b.y + 38} l-7,-4 m7,4 l-7,4`} fill="none" stroke={canceled ? "var(--state-blocked, #c33)" : criticalEdge(e.from, e.to) ? "var(--accent)" : "#9aa1ad"} strokeWidth={criticalEdge(e.from, e.to) ? 3 : 1.5} strokeDasharray={canceled ? "5 4" : undefined} />;
            })}
          </svg>
          {graph.columns.map((_, d) => <span key={d} className="absolute text-xs text-fg-tertiary" style={{ left: d * 270 + 20, top: 12 }}>DEPTH {d}</span>)}
          {graph.nodes.map((t) => {
            const p = positions.get(t.key)!;
            return <button key={t.key} ref={(el) => { if (el) refs.current.set(t.key, el); else refs.current.delete(t.key); }} onClick={() => open(t.key)}
              onKeyDown={(e) => {
                const column = graph.columns[graph.depth.get(t.key)!];
                const row = column.indexOf(t);
                const target = e.key === "ArrowDown" ? column[row + 1]?.key : e.key === "ArrowUp" ? column[row - 1]?.key : e.key === "ArrowLeft" ? graph.edges.find((edge) => edge.to === t.key)?.from : e.key === "ArrowRight" ? graph.edges.find((edge) => edge.from === t.key)?.to : undefined;
                if (target) { e.preventDefault(); refs.current.get(target)?.focus(); }
              }}
              className={`absolute rounded-lg border p-3 text-left text-xs focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-accent ${graph.chain.keys.includes(t.key) ? "border-accent ring-1 ring-accent" : t.status === "canceled" ? "border-state-blocked border-dashed" : "border-border-strong"} ${t.status === "done" ? "bg-pill-done-bg" : t.status === "in_progress" ? "bg-accent-soft" : t.status === "in_review" ? "bg-pill-review-bg" : "bg-surface"} ${t.external ? "opacity-60" : ""}`}
              style={{ left: p.x, top: p.y, width: 230, height: 80 }}>
              <span className="flex items-center justify-between gap-2"><span className="font-mono">{t.key}</span><span className="flex items-center gap-1"><StatusGlyph state={t.status} label={false} />{STATUS_NAME[t.status]}</span></span>
              <span className="mt-2 block truncate font-medium">{t.title}</span><span>{t.estimate_points ?? "—"} pts{t.external ? " · outside filter" : ""}</span>
            </button>;
          })}
        </div>
      </div>
    </>}
    <div className={list ? "overflow-auto" : "sr-only focus-within:not-sr-only"}>
      <table className="w-full text-left text-xs"><caption>Dependency list</caption><thead><tr>{["Depth", "Ticket", "Status", "Points", "Blocked by"].map((h) => <th key={h} className="p-2">{h}</th>)}</tr></thead>
        <tbody>{graph.nodes.map((t) => <tr key={t.key}><td className="p-2">{graph.depth.get(t.key)}</td><td className="p-2"><button className="underline" onClick={() => open(t.key)}>{t.key} {t.title}</button>{graph.chain.keys.includes(t.key) && " · critical path"}{t.external && " · outside filter"}</td><td className="p-2">{STATUS_NAME[t.status]}</td><td className="p-2">{t.estimate_points ?? "—"}</td><td className="p-2">{t.blocked_by.map((b) => `${b.key} (${STATUS_NAME[b.status]})`).join(", ") || "—"}</td></tr>)}</tbody>
      </table>
    </div>
    {selected && editor.isError && <p role="alert">Couldn&apos;t open {selected}: {editor.error.message} <button onClick={() => setSelected(undefined)}>Dismiss</button></p>}
    {selected && !editor.isError && bootstrap.data && <TicketEditorDialog key={selected} ticketKey={selected} detail={editor.data as Ticket | undefined} bootstrap={bootstrap.data} onClose={() => setSelected(undefined)} onReload={async () => { const r = await editor.refetch(); return r.isError ? undefined : r.data; }} />}
  </section>;
}

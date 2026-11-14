"use client";

import { useRouter } from "next/navigation";
import type { GraphData, LinkTarget, NodeKind } from "@/lib/api/client";
import { useGraph } from "@/lib/api/hooks";
import { Skeleton } from "@/components/ui/primitives";
import { agentHue } from "./avatars";

const STATE_COLOR: Record<string, string> = {
  backlog: "var(--state-backlog)",
  todo: "var(--state-todo)",
  in_progress: "var(--state-progress)",
  in_review: "var(--state-review)",
  done: "var(--state-done)",
};

/** Node color by type: doc-type hues, agent identity hues, ticket state. */
function colorOf(t: LinkTarget) {
  switch (t.kind) {
    case "ticket":
      return STATE_COLOR[t.status ?? "todo"] ?? "var(--state-todo)";
    case "agent":
      return `var(--agent-${agentHue(t.color_index ?? 0)})`;
    case "epic":
      return "var(--text-tertiary)";
    default:
      return `var(--doc-${t.artifact_type ?? "note"})`;
  }
}

function labelOf(t: LinkTarget) {
  if (t.kind === "ticket" || t.kind === "agent") return t.display_id ?? t.title;
  return t.display_id ?? t.title;
}

type Placed = { key: string; x: number; y: number; hop: number; t: LinkTarget };

/**
 * Radial layout: the centre, its direct links on an inner ring, second-hop
 * nodes on an outer ring near the first neighbour that reaches them.
 */
function layout(g: GraphData, w: number, h: number): Placed[] {
  const cx = w / 2;
  const cy = h / 2;
  const r1 = Math.min(w, h) * 0.3;
  const r2 = Math.min(w, h) * 0.44;
  const centre = g.nodes.find((n) => n.hop === 0);
  const hop1 = g.nodes.filter((n) => n.hop === 1).sort((a, b) => a.key.localeCompare(b.key));
  const hop2 = g.nodes.filter((n) => n.hop === 2);
  const out: Placed[] = [];
  if (centre) out.push({ key: centre.key, x: cx, y: cy, hop: 0, t: centre.target });
  const angle = new Map<string, number>();
  hop1.forEach((n, i) => {
    const a = (i / Math.max(hop1.length, 1)) * Math.PI * 2 - Math.PI / 2;
    angle.set(n.key, a);
    out.push({ key: n.key, x: cx + r1 * Math.cos(a), y: cy + r1 * Math.sin(a), hop: 1, t: n.target });
  });
  // Group second-hop nodes under a parent, then fan them out around it.
  const kids = new Map<string, string[]>();
  for (const n of hop2) {
    const e = g.edges.find((e) => (e.from === n.key && angle.has(e.to)) || (e.to === n.key && angle.has(e.from)));
    const parent = e ? (e.from === n.key ? e.to : e.from) : (hop1[0]?.key ?? "");
    kids.set(parent, [...(kids.get(parent) ?? []), n.key]);
  }
  const spread = (Math.PI * 2) / Math.max(hop1.length, 1);
  for (const [parent, ks] of kids) {
    const base = angle.get(parent) ?? 0;
    ks.forEach((k, i) => {
      const a = base + (ks.length === 1 ? 0 : (i / (ks.length - 1) - 0.5) * spread * 0.8);
      const n = hop2.find((x) => x.key === k)!;
      out.push({ key: k, x: cx + r2 * Math.cos(a), y: cy + r2 * Math.sin(a), hop: 2, t: n.target });
    });
  }
  return out;
}

const W = 300;
const H = 240;

/** The local graph (1–2 hops) in a page's side panel (docs/dev/plan.md §12.2). */
export function LocalGraph({ kind, id }: { kind: NodeKind; id: string | undefined }) {
  const router = useRouter();
  const { data } = useGraph(kind, id, 2);
  if (!data) return <Skeleton className="h-[200px] w-full" />;
  if (data.nodes.length <= 1) return <p className="text-12 text-fg-tertiary">No links yet. Linked pages, documents and tickets appear here.</p>;
  const placed = layout(data, W, H);
  const at = new Map(placed.map((p) => [p.key, p]));
  const n1 = placed.filter((p) => p.hop === 1).length;
  return (
    <svg
      viewBox={`0 0 ${W} ${H}`}
      className="h-auto w-full overflow-visible"
      aria-label={`Local graph: ${n1} direct links, ${placed.length - n1 - 1} more one step further`}
    >
      <g stroke="var(--border-strong)" strokeWidth={1}>
        {data.edges.map((e) => {
          const a = at.get(e.from);
          const b = at.get(e.to);
          if (!a || !b) return null;
          const strong = a.hop === 0 || b.hop === 0;
          return <line key={`${e.from}>${e.to}`} x1={a.x} y1={a.y} x2={b.x} y2={b.y} opacity={strong ? 0.9 : 0.45} />;
        })}
      </g>
      {placed.map((p) => {
        const r = p.hop === 0 ? 7 : p.hop === 1 ? 5 : 3.5;
        const label = labelOf(p.t);
        // Labels must fit inside the panel: shorter on the sides.
        const max = p.hop === 0 ? 26 : Math.abs(p.x - W / 2) > W / 6 ? 13 : 20;
        const short = label.length > max ? `${label.slice(0, max - 1)}…` : label;
        const anchor = p.x < W / 2 - 4 ? "end" : p.x > W / 2 + 4 ? "start" : "middle";
        const dx = anchor === "end" ? -r - 4 : anchor === "start" ? r + 4 : 0;
        const dy = anchor === "middle" ? (p.y < H / 2 ? -r - 5 : r + 12) : 4;
        return (
          <a
            key={p.key}
            href={p.t.href}
            onClick={(e) => {
              if (!p.t.href || e.metaKey || e.ctrlKey) return;
              e.preventDefault();
              router.push(p.t.href);
            }}
            className="group cursor-pointer focus:outline-none"
          >
            <title>{[p.t.display_id, p.t.title].filter(Boolean).join(" · ")}</title>
            <circle cx={p.x} cy={p.y} r={r + 6} fill="transparent" />
            <circle
              cx={p.x}
              cy={p.y}
              r={r}
              fill={colorOf(p.t)}
              stroke={p.hop === 0 ? "var(--accent)" : "var(--bg-panel)"}
              strokeWidth={p.hop === 0 ? 2 : 1.5}
              className="group-focus-visible:stroke-[var(--accent)]"
            />
            {p.hop < 2 && (
              <text
                x={p.x + dx}
                y={p.y + dy}
                textAnchor={anchor}
                fontSize={p.hop === 0 ? 11.5 : 10.5}
                fontWeight={p.hop === 0 ? 600 : 400}
                fill={p.hop === 0 ? "var(--text)" : "var(--text-secondary)"}
                className="group-hover:fill-[var(--text)]"
              >
                {short}
              </text>
            )}
          </a>
        );
      })}
    </svg>
  );
}

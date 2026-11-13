"use client";

import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { chartLayout, parseChart, withUnit, type ChartSpec } from "@/lib/chat-chart";
import { useVizPalette } from "@/lib/viz-theme";
import { CodeFence } from "./code-fence";
import { VisualFrame } from "./visual-frame";

const fullNumber = (v: number, unit: string | undefined) => withUnit(v.toLocaleString("en-US", { maximumFractionDigits: 2 }), unit);

function useWidth() {
  const ref = useRef<HTMLDivElement>(null);
  const [width, setWidth] = useState(0);
  useEffect(() => {
    if (!ref.current) return;
    const observer = new ResizeObserver(([entry]) => setWidth(Math.round(entry.contentRect.width)));
    observer.observe(ref.current);
    return () => observer.disconnect();
  }, []);
  return [ref, width] as const;
}

type Row = { color: string; value: string; name: string };

function tooltipFor(spec: ChartSpec, position: number, series: readonly string[]): { title: string; rows: Row[] } {
  const { unit } = spec.y;
  if (spec.type !== "scatter") {
    return {
      title: spec.categories[position],
      rows: spec.series.flatMap((s, i): Row[] => (s.values[position] === null ? [] : [{ color: series[i], value: fullNumber(s.values[position]!, unit), name: s.name }])),
    };
  }
  let left = position;
  for (const [i, s] of spec.series.entries()) {
    if (left >= s.points.length) {
      left -= s.points.length;
      continue;
    }
    const [x, y] = s.points[left];
    return { title: s.name, rows: [{ color: series[i], value: fullNumber(x, undefined), name: spec.x.label ?? "x" }, { color: series[i], value: fullNumber(y, unit), name: spec.y.label ?? "y" }] };
  }
  return { title: "", rows: [] };
}

function ChartPlot({ spec, alt, tall }: { spec: ChartSpec; alt: string; tall?: boolean }) {
  const palette = useVizPalette();
  const [ref, width] = useWidth();
  const [active, setActive] = useState<number | null>(null);
  const layout = useMemo(() => (width > 0 ? chartLayout(spec, width, tall) : null), [spec, width, tall]);
  const { plot } = layout ?? {};
  const pointed = (e: { clientX: number; clientY: number; currentTarget: HTMLElement }) => {
    const r = e.currentTarget.getBoundingClientRect();
    setActive(layout?.nearest(e.clientX - r.left, e.clientY - r.top) ?? null);
  };
  const move = (delta: number) => layout && setActive(Math.max(0, Math.min(layout.positions - 1, (active ?? (delta > 0 ? -1 : layout.positions)) + delta)));
  const text = { fontSize: 11, fontFamily: "inherit", fontVariantNumeric: "tabular-nums" } as const;
  const point = active !== null && layout ? layout.anchor(active) : null;
  const tip = active !== null && layout ? tooltipFor(spec, active, palette.series) : null;
  const multi = spec.series.length > 1;

  return (
    <div className="flex flex-col gap-2 bg-surface px-3 py-3">
      {(spec.title || spec.subtitle) && (
        <div>
          {spec.title && <div className="text-13 font-semibold text-fg">{spec.title}</div>}
          {spec.subtitle && <div className="text-12 text-fg-secondary">{spec.subtitle}</div>}
        </div>
      )}
      {multi && (
        <ul className="flex flex-wrap gap-x-3 gap-y-1 text-12 text-fg-secondary">
          {spec.series.map((s, i) => (
            <li key={s.name} className="flex items-center gap-1.5">
              <span aria-hidden className={spec.type === "line" ? "h-0.5 w-3 rounded-full" : spec.type === "scatter" ? "size-2 rounded-full" : "size-2 rounded-xs"} style={{ background: palette.series[i] }} />
              {s.name}
            </li>
          ))}
        </ul>
      )}
      {spec.y.label && !layout?.horizontal && <div className="text-11 text-fg-tertiary">{spec.y.label}</div>}
      {/* Arrow keys walk the same positions as the pointer. */}
      <div
        ref={ref}
        tabIndex={0}
        role="group"
        aria-label={`${alt}. Arrow keys read each value.`}
        className="relative w-full rounded outline-none focus-visible:ring-1 focus-visible:ring-accent"
        onPointerMove={pointed}
        onPointerLeave={() => setActive(null)}
        onBlur={() => setActive(null)}
        onKeyDown={(e) => {
          if (e.key === "ArrowRight" || e.key === "ArrowDown") {
            e.preventDefault();
            move(1);
          } else if (e.key === "ArrowLeft" || e.key === "ArrowUp") {
            e.preventDefault();
            move(-1);
          } else if (e.key === "Escape") setActive(null);
        }}
      >
        {layout && plot ? (
          <svg width={layout.width} height={layout.height} role="img" aria-label={alt} className="block">
            {layout.valueTicks.map((t) => (
              <g key={t.pos}>
                {layout.horizontal ? (
                  <>
                    <line x1={t.pos} x2={t.pos} y1={plot.top} y2={plot.bottom} stroke={palette.grid} />
                    <text x={t.pos} y={plot.bottom + 16} textAnchor="middle" fill={palette.muted} style={text}>{t.label}</text>
                  </>
                ) : (
                  <>
                    <line x1={plot.left} x2={plot.right} y1={t.pos} y2={t.pos} stroke={palette.grid} />
                    <text x={plot.left - 8} y={t.pos + 4} textAnchor="end" fill={palette.muted} style={text}>{t.label}</text>
                  </>
                )}
              </g>
            ))}
            {layout.xTicks.map((t) => (
              <g key={t.pos}>
                <line x1={t.pos} x2={t.pos} y1={plot.top} y2={plot.bottom} stroke={palette.grid} />
                <text x={t.pos} y={plot.bottom + 16} textAnchor="middle" fill={palette.muted} style={text}>{t.label}</text>
              </g>
            ))}
            {layout.baseline !== null && (layout.horizontal ? <line x1={layout.baseline} x2={layout.baseline} y1={plot.top} y2={plot.bottom} stroke={palette.line} /> : <line x1={plot.left} x2={plot.right} y1={layout.baseline} y2={layout.baseline} stroke={palette.line} />)}
            {layout.categoryLabels.map((c) => <text key={`${c.x}-${c.y}`} x={c.x} y={c.y} textAnchor={c.anchor} fill={palette.muted} style={text}>{c.label}</text>)}
            {layout.bars.map((b) => <path key={`${b.series}-${b.index}`} d={b.d} fill={palette.series[b.series]} opacity={active !== null && active !== b.index ? 0.45 : 1} />)}
            {spec.type === "line" && point && <line x1={point.x} x2={point.x} y1={plot.top} y2={plot.bottom} stroke={palette.line} />}
            {layout.lines.map((l) => (
              <g key={l.series}>
                <path d={l.d} fill="none" stroke={palette.series[l.series]} strokeWidth={2} strokeLinejoin="round" strokeLinecap="round" />
                {l.dots.map((d) => <circle key={d.index} cx={d.x} cy={d.y} r={4} fill={palette.series[l.series]} stroke={palette.surface} strokeWidth={2} />)}
              </g>
            ))}
            {layout.points.map((p, i) => <circle key={i} cx={p.x} cy={p.y} r={active === i ? 6 : 4} fill={palette.series[p.series]} stroke={palette.surface} strokeWidth={2} />)}
            {layout.labels.map((l) => <text key={`${l.x}-${l.y}-${l.label}`} x={l.x} y={l.y} textAnchor={l.anchor} fill={palette.ink} style={text}>{l.label}</text>)}
          </svg>
        ) : (
          <div className="h-[220px]" />
        )}
        {tip && point && layout && (
          <div role="status" className="pointer-events-none absolute z-10 w-max max-w-[220px] rounded-md border border-border-float bg-surface-raised px-2 py-1.5 text-12" style={{ left: point.x < layout.width / 2 ? point.x + 12 : point.x - 12, top: Math.max(0, point.y - 8), transform: point.x < layout.width / 2 ? undefined : "translateX(-100%)" }}>
            <div className="mb-0.5 text-fg-tertiary">{tip.title}</div>
            {tip.rows.map((r) => (
              <div key={r.name} className="flex items-center gap-1.5">
                <span aria-hidden className="h-0.5 w-2.5 rounded-full" style={{ background: r.color }} />
                <span className="font-semibold text-fg">{r.value}</span>
                {r.name && <span className="text-fg-secondary">{r.name}</span>}
              </div>
            ))}
          </div>
        )}
      </div>
      {spec.x.label && !layout?.horizontal && <div className="text-center text-11 text-fg-tertiary">{spec.x.label}</div>}
      {spec.y.label && layout?.horizontal && <div className="text-center text-11 text-fg-tertiary">{spec.y.label}</div>}
    </div>
  );
}

function ChartData({ spec }: { spec: ChartSpec }) {
  const cell = "px-3 py-1.5 text-left font-normal";
  const num = "px-3 py-1.5 text-right font-mono tabular-nums";
  return (
    <div className="max-h-[420px] overflow-auto bg-surface">
      <table className="w-full border-collapse text-12 text-fg-2">
        {spec.type === "scatter" ? (
          <>
            <thead><tr className="border-b border-border text-fg-tertiary"><th className={cell}>Series</th><th className={num}>{spec.x.label ?? "x"}</th><th className={num}>{spec.y.label ?? "y"}</th></tr></thead>
            <tbody>
              {spec.series.flatMap((s) => s.points.map((p, i) => <tr key={`${s.name}-${i}`} className="border-b border-border-row last:border-b-0"><td className={cell}>{s.name}</td><td className={num}>{fullNumber(p[0], undefined)}</td><td className={num}>{fullNumber(p[1], spec.y.unit)}</td></tr>))}
            </tbody>
          </>
        ) : (
          <>
            <thead><tr className="border-b border-border text-fg-tertiary"><th className={cell}>{spec.x.label ?? ""}</th>{spec.series.map((s) => <th key={s.name} className={num}>{s.name || spec.y.label || "Value"}</th>)}</tr></thead>
            <tbody>
              {spec.categories.map((c, i) => <tr key={`${c}-${i}`} className="border-b border-border-row last:border-b-0"><td className={cell}>{c}</td>{spec.series.map((s) => <td key={s.name} className={num}>{s.values[i] === null ? "–" : fullNumber(s.values[i]!, spec.y.unit)}</td>)}</tr>)}
            </tbody>
          </>
        )}
      </table>
    </div>
  );
}

/** A chart, in Chat or a lesson, drawn from its spec with a data table alongside. */
export function ChartVisual({ spec, alt, caption }: { spec: ChartSpec; alt?: string; caption?: ReactNode }) {
  const [view, setView] = useState<"chart" | "data">("chart");
  const name = alt ?? spec.title ?? "Chart";
  return (
    <VisualFrame testId="chart" icon="dashboard" label="Chart" views={[{ value: "chart", label: "Chart" }, { value: "data", label: "Data" }]} view={view} onView={setView} caption={caption} zoom={view === "chart" ? <ChartPlot spec={spec} alt={name} tall /> : undefined}>
      {view === "chart" ? <ChartPlot spec={spec} alt={name} /> : <ChartData spec={spec} />}
    </VisualFrame>
  );
}

/** A ```chart fence; a spec that can't be drawn shows as code, saying why. */
export function ChartFence({ code, info }: { code: string; info?: string }) {
  const parsed = useMemo(() => parseChart(code), [code]);
  if ("spec" in parsed) return <ChartVisual spec={parsed.spec} />;
  return (
    <div className="flex flex-col gap-1">
      <p role="alert" className="text-12 text-pill-blocked-fg">{parsed.error}</p>
      <CodeFence code={code} info={info?.replace(/^\s*\S+/, "json")} />
    </div>
  );
}

"use client";

import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { Fragment, useState, type ReactNode } from "react";
import type { Schemas } from "@/lib/api/client";
import { useCosts } from "@/lib/api/hooks";
import { compact } from "@/lib/format";
import {
  OTHER,
  RANGE_PRESETS,
  axisMax,
  costPerPoint,
  presetRange,
  rangeError,
  seriesLabel,
  share,
  spendLabel,
  stackSeries,
  usd,
  type RangePreset,
} from "@/lib/costs";
import { AGENT_HUES } from "@/components/domain/avatars";
import { Button } from "@/components/ui/button";
import { Segmented } from "@/components/ui/controls";
import { Banner, EmptyState, Skeleton } from "@/components/ui/primitives";
import { ProjectScopeState, projectCrumbs, useProjectScope } from "@/components/shell/project-scope";
import { Content, TopBar } from "@/components/shell/topbar";
import { ProjectPicker } from "@/components/shell/project-picker";
import { ReportsTabNav } from "./reports-tabs";

type Report = Schemas["CostReport"];
type Slice = Schemas["CostSlice"];
type By = "agents" | "models";

const BY_OPTIONS = [
  { value: "agents", label: "By agent" },
  { value: "models", label: "By model" },
] as const;
const PRESETS = new Set<string>(RANGE_PRESETS.map((p) => p.value));
const dayFmt = new Intl.DateTimeFormat("en-US", { month: "short", day: "numeric", timeZone: "UTC" });
const weekdayFmt = new Intl.DateTimeFormat("en-US", { weekday: "short", month: "short", day: "numeric", timeZone: "UTC" });
const yearFmt = new Intl.DateTimeFormat("en-US", { month: "short", day: "numeric", year: "numeric", timeZone: "UTC" });
const dateInput = "h-8 rounded-md border border-border-strong bg-surface px-2 font-mono text-xs text-fg";

function seriesColor(i: number, key: string): string {
  return key === OTHER ? "var(--text-tertiary)" : `var(--agent-${AGENT_HUES[i % AGENT_HUES.length]})`;
}

function Kpi({ label, value, sub, tone, action }: { label: string; value: ReactNode; sub: ReactNode; tone?: "warn"; action?: ReactNode }) {
  return (
    <div className="flex flex-col gap-1.5 rounded-xl border border-border bg-surface px-[18px] py-4">
      <div className="flex items-center justify-between gap-2 text-[13px] text-fg-secondary">
        {label}
        {action}
      </div>
      <div className={tone === "warn" ? "text-[26px] font-semibold text-pill-human-fg" : "text-[26px] font-semibold"}>{value}</div>
      <div className="text-xs text-fg-tertiary">{sub}</div>
    </div>
  );
}

function SpendChart({ report, by, setBy, agentNames }: { report: Report; by: By; setBy: (b: By) => void; agentNames: Map<string, string> }) {
  const { keys, bars, totals } = stackSeries(report.days, by);
  const [hover, setHover] = useState<number | undefined>(undefined);
  const max = axisMax(Math.max(0, ...bars.map((b) => b.total)));
  const W = 1000, H = 220, left = 44, bottom = 22, top = 8;
  const plotH = H - bottom - top;
  const slot = (W - left) / Math.max(1, bars.length);
  const barW = Math.max(2, Math.min(28, slot * 0.7));
  const label = (k: string) => (by === "agents" ? (agentNames.get(k) ?? k) : seriesLabel(k, by));
  const ticks = [0, 0.25, 0.5, 0.75, 1];
  const every = Math.max(1, Math.ceil(bars.length / 6));
  const shown = hover !== undefined ? bars[hover] : undefined;
  return (
    <section aria-labelledby="spend-over-time" className="rounded-xl border border-border bg-surface px-[18px] py-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h2 id="spend-over-time" className="type-heading">
          Spend over time <span className="ml-2 text-xs font-normal text-fg-tertiary">daily, stacked</span>
        </h2>
        <Segmented label="Group the chart" small options={BY_OPTIONS} value={by} onChange={setBy} />
      </div>
      <p aria-live="polite" className="mt-2 min-h-5 text-xs text-fg-secondary">
        {shown ? (
          <>
            <span className="font-medium text-fg">{weekdayFmt.format(new Date(shown.date))}</span> · {usd(shown.total)} · {shown.runs} {shown.runs === 1 ? "run" : "runs"}
            {shown.segments.filter((s) => s.cost_micros > 0).map((s) => (
              <span key={s.key}> · {label(s.key)} {usd(s.cost_micros)}</span>
            ))}
          </>
        ) : (
          <span className="text-fg-tertiary">Hover or focus a bar for its split · top 6 series, the rest as Other</span>
        )}
      </p>
      <svg viewBox={`0 0 ${W} ${H}`} className="mt-1 h-auto w-full" role="group" aria-label={`Daily spend ${by === "agents" ? "by agent" : "by model"}`}>
        {ticks.map((t) => {
          const y = top + plotH * (1 - t);
          return (
            <g key={t}>
              <line x1={left} x2={W} y1={y} y2={y} stroke="var(--border-subtle)" />
              <text x={left - 6} y={y + 3} textAnchor="end" className="fill-fg-tertiary font-mono text-[10px]">{usd(max * t).replace(/\.00$/, "")}</text>
            </g>
          );
        })}
        {bars.map((b, i) => {
          const x = left + slot * i + (slot - barW) / 2;
          let y = top + plotH;
          return (
            <g
              key={b.date}
              tabIndex={0}
              role="img"
              aria-label={`${dayFmt.format(new Date(b.date))}: ${usd(b.total)}`}
              onMouseEnter={() => setHover(i)}
              onMouseLeave={() => setHover(undefined)}
              onFocus={() => setHover(i)}
              onBlur={() => setHover(undefined)}
              className="outline-none"
            >
              <rect x={left + slot * i} y={top} width={slot} height={plotH} fill={hover === i ? "var(--hover)" : "transparent"} />
              {b.segments.map((s, k) => {
                const h = (s.cost_micros / max) * plotH;
                if (h <= 0) return null;
                y -= h;
                return <rect key={s.key} x={x} y={y} width={barW} height={h} fill={seriesColor(k, s.key)} opacity={hover === undefined || hover === i ? 0.9 : 0.5} />;
              })}
              {i % every === 0 && (
                <text x={x + barW / 2} y={H - 6} textAnchor="middle" className="fill-fg-tertiary text-[10px]">{dayFmt.format(new Date(b.date))}</text>
              )}
            </g>
          );
        })}
      </svg>
      {keys.length > 0 && (
        <ul className="mt-3 flex flex-wrap gap-x-5 gap-y-1.5 text-xs" aria-label="Series totals">
          {totals.map((s, i) => (
            <li key={s.key} className="flex items-center gap-1.5">
              <span aria-hidden className="size-2.5 rounded-[3px]" style={{ background: seriesColor(i, s.key) }} />
              <span className="text-fg-secondary">{label(s.key)}</span>
              <span className="font-mono text-fg">{usd(s.cost_micros)}</span>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}

function Breakdown({
  title,
  hint,
  rows,
  total,
  metric,
  href,
  name,
}: {
  title: string;
  hint?: string;
  rows: Slice[];
  total: Slice;
  metric: "runs" | "tokens";
  href?: (s: Slice) => string | undefined;
  name?: (s: Slice) => ReactNode;
}) {
  return (
    <section aria-label={title} className="rounded-xl border border-border bg-surface">
      <div className="flex items-baseline justify-between gap-3 border-b border-border-subtle px-[18px] py-3">
        <h2 className="type-heading">{title}</h2>
        {hint && <span className="text-xs text-fg-tertiary">{hint}</span>}
      </div>
      <table className="w-full text-left text-[13px]">
        <thead className="text-xs text-fg-tertiary">
          <tr>
            <th scope="col" className="px-[18px] py-2 font-medium">{title.replace(/^By /, "").replace(/^./, (c) => c.toUpperCase())}</th>
            <th scope="col" className="w-20 px-2 py-2 text-right font-medium">Share</th>
            <th scope="col" className="w-20 px-2 py-2 text-right font-medium">{metric === "runs" ? "Runs" : "Tokens"}</th>
            <th scope="col" className="w-28 px-[18px] py-2 text-right font-medium">Spend</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((s) => {
            const link = href?.(s);
            const label = name ? name(s) : s.label;
            return (
              <tr key={s.key} className="border-t border-border-row hover:bg-hover">
                <td className="max-w-0 truncate px-[18px] py-2">
                  {link ? <Link href={link} className="hover:underline">{label}</Link> : label}
                  {s.detail && <span className="ml-2 font-mono text-[11.5px] text-fg-tertiary">{s.detail}</span>}
                </td>
                <td className="px-2 py-2 text-right font-mono text-xs text-fg-secondary">{share(s.cost_micros, total.cost_micros)}</td>
                <td className="px-2 py-2 text-right font-mono text-xs text-fg-secondary">{metric === "runs" ? s.runs : compact(s.tokens)}</td>
                <td className="px-[18px] py-2 text-right font-mono text-xs">{spendLabel(s.cost_micros, s.unpriced_runs)}</td>
              </tr>
            );
          })}
        </tbody>
        <tfoot>
          <tr className="border-t border-border font-medium">
            <td className="px-[18px] py-2">Total</td>
            <td />
            <td className="px-2 py-2 text-right font-mono text-xs">{metric === "runs" ? total.runs : compact(total.tokens)}</td>
            <td className="px-[18px] py-2 text-right font-mono text-xs">{spendLabel(total.cost_micros, total.unpriced_runs)}</td>
          </tr>
        </tfoot>
      </table>
    </section>
  );
}

function TopTickets({ tickets, projectNames, agentNames }: { tickets: Schemas["CostTicket"][]; projectNames: Map<string, string>; agentNames: Map<string, string> }) {
  const [open, setOpen] = useState<string | undefined>(undefined);
  return (
    <section aria-labelledby="top-tickets" className="rounded-xl border border-border bg-surface">
      <div className="flex flex-wrap items-baseline justify-between gap-3 border-b border-border-subtle px-[18px] py-3">
        <h2 id="top-tickets" className="type-heading">
          Most expensive tickets <span className="ml-2 text-xs font-normal text-fg-tertiary">top 10 in range, including sub agent runs</span>
        </h2>
        <span className="text-xs text-fg-tertiary">Points come from the ticket estimate</span>
      </div>
      {tickets.length === 0 ? (
        <p className="px-[18px] py-6 text-center text-[13px] text-fg-tertiary">No ticket spend in this range.</p>
      ) : (
        <table className="w-full text-left text-[13px]">
          <thead className="text-xs text-fg-tertiary">
            <tr>
              <th scope="col" className="w-8 px-[18px] py-2 font-medium">#</th>
              <th scope="col" className="px-2 py-2 font-medium">Ticket</th>
              <th scope="col" className="w-44 px-2 py-2 font-medium max-md:hidden">Project</th>
              <th scope="col" className="w-16 px-2 py-2 text-right font-medium">Points</th>
              <th scope="col" className="w-24 px-2 py-2 text-right font-medium">Spend</th>
              <th scope="col" className="w-28 px-2 py-2 text-right font-medium">Cost / point</th>
              <th scope="col" className="w-24 px-[18px] py-2 text-right font-medium">Runs</th>
            </tr>
          </thead>
          <tbody>
            {tickets.map((t, i) => {
              const expanded = open === t.key;
              const cpp = costPerPoint(t.cost_micros, t.points);
              return (
                <Fragment key={t.key}>
                  <tr className="border-t border-border-row hover:bg-hover">
                    <td className="px-[18px] py-2 font-mono text-xs text-fg-tertiary">{i + 1}</td>
                    <td className="max-w-0 px-2 py-2">
                      <span className="flex min-w-0 items-baseline gap-2">
                        <Link href={`/tickets/${t.key}`} className="flex-none font-mono text-xs text-fg-secondary hover:underline">{t.key}</Link>
                        <span className="min-w-0 truncate">{t.title}</span>
                      </span>
                    </td>
                    <td className="truncate px-2 py-2 text-fg-secondary max-md:hidden">{projectNames.get(t.project_key) ?? t.project_key}</td>
                    <td className="px-2 py-2 text-right font-mono text-xs">{t.points ?? "–"}</td>
                    <td className="px-2 py-2 text-right font-mono text-xs">{usd(t.cost_micros)}</td>
                    <td className="px-2 py-2 text-right font-mono text-xs text-fg-secondary">{cpp ?? <span className="font-sans text-fg-tertiary">Not estimated</span>}</td>
                    <td className="px-[18px] py-2 text-right">
                      <button
                        type="button"
                        aria-expanded={expanded}
                        aria-controls={`runs-${t.key}`}
                        onClick={() => setOpen(expanded ? undefined : t.key)}
                        className="text-xs text-accent hover:underline"
                      >
                        {t.runs.length} {t.runs.length === 1 ? "run" : "runs"} {expanded ? "▾" : "▸"}
                      </button>
                    </td>
                  </tr>
                  {expanded && (
                    <tr id={`runs-${t.key}`} className="bg-surface-sunken">
                      <td />
                      <td colSpan={6} className="px-2 pb-3 pt-1">
                        <p className="py-1 text-xs text-fg-tertiary">Runs behind {t.key} · each opens its run log</p>
                        <ul className="flex flex-col">
                          {t.runs.map((r) => (
                            <li key={r.id} className="flex items-center gap-3 border-t border-border-row py-1.5 text-xs first:border-t-0">
                              <span className="w-40 truncate">{r.parent_run_id ? "↳ " : ""}{agentNames.get(r.agent) ?? r.agent}</span>
                              <span className="w-36 truncate font-mono text-fg-secondary">{r.model}</span>
                              <span className="w-24 text-fg-secondary">{r.state}</span>
                              <span className="w-20 text-right font-mono">{usd(r.cost_micros)}</span>
                              <Link href={`/runs/${r.id}`} className="ml-auto text-accent hover:underline">Open log →</Link>
                            </li>
                          ))}
                        </ul>
                      </td>
                    </tr>
                  )}
                </Fragment>
              );
            })}
          </tbody>
        </table>
      )}
      <p className="border-t border-border-subtle px-[18px] py-2.5 text-xs text-fg-tertiary">
        Ticket spend sums every run on the ticket in this range, sub agent runs included. The ticket header&apos;s cost chip shows all-time spend.
      </p>
    </section>
  );
}

/** Spend across runs, from GET /costs. Run and chat SSE events refresh it. */
export function CostsScreen() {
  const params = useSearchParams();
  const router = useRouter();
  const scope = useProjectScope("optional");
  const rawPreset = params.get("range") ?? "30";
  const preset = (PRESETS.has(rawPreset) ? rawPreset : "30") as RangePreset;
  const by: By = params.get("by") === "models" ? "models" : "agents";
  const [now] = useState(() => new Date());
  const fallback = presetRange(30, now);
  const range = preset === "custom" ? { from: params.get("from") ?? fallback.from, to: params.get("to") ?? fallback.to } : presetRange(Number(preset), now);
  const invalid = preset === "custom" ? rangeError(range.from, range.to) : undefined;
  const project = scope.status === "ready" ? scope.project : undefined;
  const report = useCosts({ ...range, project: project?.key }, scope.status === "ready" && !invalid);

  const set = (patch: Record<string, string | undefined>) => {
    const q = new URLSearchParams(params.toString());
    for (const [k, v] of Object.entries(patch)) {
      if (v === undefined) q.delete(k);
      else q.set(k, v);
    }
    router.replace(`/reports?${q}`);
  };
  const pickPreset = (p: RangePreset) => set(p === "custom" ? { range: p, from: range.from, to: range.to } : { range: p, from: undefined, to: undefined });

  const data = report.data;
  const agentNames = new Map((data?.agents ?? []).map((a) => [a.key, a.label]));
  const projectNames = new Map((data?.projects ?? []).map((p) => [p.key, p.label]));
  const days = data?.days.length ?? 0;
  const peak = data?.days.reduce<Schemas["CostDay"] | undefined>((m, d) => (d.cost_micros > (m?.cost_micros ?? 0) ? d : m), undefined);
  const rangeLabel = invalid ? "" : `${yearFmt.format(new Date(range.from))} – ${yearFmt.format(new Date(range.to))} · UTC days`;

  let body: ReactNode;
  if (scope.status !== "ready") body = <ProjectScopeState base="/reports?tab=costs" scope={scope} />;
  else if (invalid) body = <Banner tone="blocked" title="Pick a valid range">{invalid}</Banner>;
  else if (report.isError)
    body = (
      <Banner tone="blocked" title="Couldn’t load costs" actions={<Button onClick={() => report.refetch()}>Retry</Button>}>
        {report.error.message}
      </Banner>
    );
  else if (!data)
    body = (
      <div aria-busy="true" className="flex flex-col gap-4">
        <div className="grid grid-cols-4 gap-4 max-lg:grid-cols-2">{[0, 1, 2, 3].map((i) => <Skeleton key={i} className="h-[104px]" />)}</div>
        <Skeleton className="h-[320px]" />
      </div>
    );
  else if (data.total.runs === 0)
    body = (
      <EmptyState icon="dollar" title="No spend in this range">
        Runs record their spend as they work. Try a longer range{project ? " or All projects" : ""}.
      </EmptyState>
    );
  else
    body = (
      <div className="flex flex-col gap-5">
        <div className="grid grid-cols-4 gap-4 max-lg:grid-cols-2 max-sm:grid-cols-1">
          <Kpi label="Total spend" value={spendLabel(data.total.cost_micros, data.unpriced_runs)} sub={`${data.total.runs} ${data.total.runs === 1 ? "run" : "runs"} · ${compact(data.total.tokens)} tokens`} />
          <Kpi
            label="Daily average"
            value={usd(Math.round(data.total.cost_micros / Math.max(1, days)))}
            sub={peak ? `Peak ${usd(peak.cost_micros)} on ${weekdayFmt.format(new Date(peak.date))}` : "No spend yet"}
          />
          <Kpi
            label="Cost per point"
            value={costPerPoint(data.estimated.cost_micros, data.estimated.points) ?? "–"}
            sub={data.estimated.points > 0 ? `Over ${data.estimated.points} points on ${data.estimated.tickets} estimated ${data.estimated.tickets === 1 ? "ticket" : "tickets"}` : "No estimated tickets in range"}
          />
          <Kpi
            label="Unpriced runs"
            value={data.unpriced_runs}
            tone={data.unpriced_runs > 0 ? "warn" : undefined}
            sub={data.unpriced_runs > 0 ? "Unknown or partial cost; totals are lower bounds" : "Every run is priced"}
            action={data.unpriced_runs > 0 ? <Link href="/runs" className="text-xs text-accent hover:underline">Show runs</Link> : undefined}
          />
        </div>
        <SpendChart report={data} by={by} setBy={(b) => set({ by: b === "agents" ? undefined : b })} agentNames={agentNames} />
        <div className="grid grid-cols-2 gap-5 max-lg:grid-cols-1">
          <Breakdown title="By agent" hint="Rows open the agent’s run history" rows={data.agents} total={data.total} metric="runs" href={(s) => `/runs?agent=${encodeURIComponent(s.key)}`} />
          <Breakdown
            title="By project"
            hint={project ? undefined : "Rows scope the page to the project"}
            rows={data.projects}
            total={data.total}
            metric="runs"
            href={(s) => (s.key && !project ? `/reports?${new URLSearchParams({ ...Object.fromEntries(params), project: s.key })}` : undefined)}
            name={(s) => s.label}
          />
          <Breakdown title="By model" hint="Chat turns split by the model that ran them" rows={data.models} total={data.total} metric="tokens" />
          <Breakdown title="By purpose" rows={data.purposes} total={data.total} metric="runs" />
        </div>
        <TopTickets tickets={data.top_tickets} projectNames={projectNames} agentNames={agentNames} />
      </div>
    );

  return (
    <>
      <TopBar crumbs={[...projectCrumbs(project, "Reports").slice(0, -1), { label: "Reports", href: "/reports" }, { label: "Costs" }]} />
      <Content>
        <div className="flex flex-col gap-5">
          <div className="flex flex-wrap items-start justify-between gap-4">
            <div className="flex flex-col gap-3">
              <div className="space-y-1">
                <h1 className="type-display">Reports</h1>
                <p className="text-sm text-fg-secondary">
                  Spend recorded on every run: ticket work, sub agents, planning and chat.
                </p>
              </div>
              <div className="flex flex-wrap items-center gap-3">
                <ReportsTabNav tab="costs" />
                <ProjectPicker />
              </div>
            </div>
            <div className="flex flex-col items-end gap-2">
              <Segmented label="Date range" options={RANGE_PRESETS} value={preset} onChange={pickPreset} />
              {preset === "custom" && (
                <div className="flex items-center gap-2 text-xs text-fg-tertiary">
                  <label className="flex items-center gap-1.5">From<input type="date" className={dateInput} value={range.from} onChange={(e) => set({ from: e.target.value })} /></label>
                  <label className="flex items-center gap-1.5">To<input type="date" className={dateInput} value={range.to} onChange={(e) => set({ to: e.target.value })} /></label>
                </div>
              )}
              {!invalid && <p className="text-xs text-fg-tertiary">{rangeLabel}</p>}
            </div>
          </div>
          {body}
        </div>
      </Content>
    </>
  );
}

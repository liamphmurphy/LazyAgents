"use client";

import { useRouter, useSearchParams } from "next/navigation";
import { useHighlights } from "@/lib/api/hooks";
import { presetRange, rangeError, share } from "@/lib/costs";
import { routingHighlights, type HighlightRow } from "@/lib/highlights";
import { Button } from "@/components/ui/button";
import { Segmented } from "@/components/ui/controls";
import { Banner, EmptyState, Skeleton } from "@/components/ui/primitives";
import { ProjectScopeState, projectCrumbs, useProjectScope } from "@/components/shell/project-scope";
import { Content, TopBar } from "@/components/shell/topbar";
import { ProjectPicker } from "@/components/shell/project-picker";
import { ReportsTabNav } from "./reports-tabs";
import { PageHeader } from "@/components/ui/page-header";

const ranges = [{ value: "7", label: "7 days" }, { value: "30", label: "30 days" }, { value: "90", label: "90 days" }, { value: "custom", label: "Custom" }] as const;
const field = "rounded-md border border-border-strong bg-surface px-2 py-1 text-14 text-fg";

function Metric({ label, value, note }: { label: string; value: string | number; note: string }) {
  return <section aria-label={label} className="rounded-xl border border-border bg-surface p-4">
    <h2 className="text-14 text-fg-secondary">{label}</h2>
    <p className="my-2 text-26 font-semibold">{value}</p>
    <p className="text-12 text-fg-tertiary">{note}</p>
  </section>;
}

function Distribution({ title, rows, total, note }: { title: string; rows: HighlightRow[]; total: number; note: string }) {
  return <section aria-label={title} className="rounded-xl border border-border bg-surface p-4">
    <h2 className="type-heading">{title}</h2>
    <p className="mt-1 mb-3 text-12 text-fg-tertiary">{note}</p>
    {rows.length ? <div className="overflow-x-auto"><table className="w-full text-left text-14">
      <thead className="text-12 text-fg-secondary"><tr><th scope="col" className="py-2 font-normal">Choice</th><th scope="col" className="px-3 text-right font-normal">Turns</th><th scope="col" className="text-right font-normal">Share</th><th scope="col" className="pl-3 text-right font-normal">Auto effort</th></tr></thead>
      <tbody>{rows.map((r) => <tr key={r.key} className="border-t border-border-subtle">
        <th scope="row" className="max-w-64 py-3 pr-3 text-14 font-normal"><span className="break-words">{r.label}</span><div aria-hidden className="mt-1 h-1 rounded bg-hover"><div className="h-1 rounded bg-accent" style={{ width: `${total ? r.turns / total * 100 : 0}%` }} /></div></th>
        <td className="px-3 text-right font-mono">{r.turns}</td><td className="text-right font-mono">{share(r.turns, total)}</td><td className="pl-3 text-right font-mono">{r.automatic}</td>
      </tr>)}</tbody>
    </table></div> : <p className="py-4 text-14 text-fg-secondary">No recorded samples.</p>}
  </section>;
}

export function HighlightsScreen() {
  const params = useSearchParams();
  const router = useRouter();
  const scope = useProjectScope("optional");
  const project = scope.status === "ready" ? scope.project : undefined;
  const rawRange = params.get("range") ?? "30";
  const range = ranges.some((r) => r.value === rawRange) ? rawRange : "30";
  const fallback = presetRange(range === "custom" ? 30 : Number(range), new Date());
  const dates = range === "custom" ? { from: params.get("from") ?? fallback.from, to: params.get("to") ?? fallback.to } : fallback;
  const invalid = rangeError(dates.from, dates.to);
  const query = useHighlights({ ...dates, project: project?.key }, scope.status === "ready" && !invalid);
  const buckets = query.data?.buckets ?? [];
  const persona = params.get("persona") ?? "all";
  const mode = params.get("selection") === "auto" ? "auto" : "all";
  const all = routingHighlights(buckets);
  const selected = buckets.filter((b) => (persona === "all" || b.persona_id === persona) && (mode === "all" || b.effort_level));
  const report = routingHighlights(selected);
  const set = (values: Record<string, string | undefined>) => {
    const next = new URLSearchParams(params.toString());
    next.set("tab", "highlights");
    for (const [key, value] of Object.entries(values)) { if (value === undefined) next.delete(key); else next.set(key, value); }
    router.replace(`/reports?${next}`);
  };

  let body;
  if (scope.status !== "ready") body = <ProjectScopeState base="/reports?tab=highlights" scope={scope} />;
  else if (invalid) body = <Banner tone="human" title="Invalid date range">{invalid}</Banner>;
  else if (query.isError) body = <Banner tone="blocked" title="Couldn’t load highlights" actions={<Button onClick={() => query.refetch()}>Retry</Button>}>{query.error.message}</Banner>;
  else if (!query.data) body = <Skeleton className="h-80" />;
  else if (!report.turns) body = <EmptyState icon="compass" title="No reply turns in this selection">Try a longer range, another persona, or All effort choices. Only your chats are included.</EmptyState>;
  else body = <>
    <div className="grid grid-cols-4 gap-4 max-lg:grid-cols-2 max-sm:grid-cols-1">
      <Metric label="Reply turns" value={report.turns} note={`${report.chosen} chosen effort · ${report.unknown} unclassified`} />
      <Metric label="Automatic effort" value={share(report.automatic, report.turns)} note={`${report.automatic} of ${report.turns} reply turns`} />
      <Metric label="Effort raised" value={share(report.raised, report.automatic)} note={`${report.raised} of ${report.automatic} Auto turns · go-ahead, retry or edit`} />
      <Metric label="Average turn time" value={report.averageMs === undefined ? "Unknown" : `${(report.averageMs / 1000).toFixed(1)}s`} note={`${report.timed} timed turns · includes tools, excludes queue time`} />
    </div>
    <p className="text-14 text-fg-secondary">{report.adjusted} Auto turns used a different recorded effort than requested. Auto turns without an actual effort remain model default / unrecorded.</p>
    <div className="grid grid-cols-2 gap-4 max-lg:grid-cols-1">
      <Distribution title="Requested Auto effort" rows={report.levels} total={report.automatic} note="What the message called for, before model support limited the effort." />
      <Distribution title="Actual effort" rows={report.efforts} total={report.turns} note="What ran, across the selected reply turns." />
      <Distribution title="By persona" rows={report.personas} total={report.turns} note="The recorded replying persona, including guests in group chats." />
      <Distribution title="Model usage" rows={report.models} total={report.turns} note="Models that ran. Historical records do not identify automatic model selection." />
      <Distribution title="Reasons effort was raised" rows={report.reasons} total={report.automatic} note="Share of Auto turns. A turn can have more than one reason." />
    </div>
  </>;

  return <>
    <TopBar crumbs={[...projectCrumbs(project, "Reports").slice(0, -1), { label: "Reports", href: "/reports" }, { label: "Highlights" }]} />
    <Content><div className="flex flex-col gap-5">
      <PageHeader title="Reports" description="Routing decisions across your persona chats."><ReportsTabNav tab="highlights" /><ProjectPicker /></PageHeader>
      <div className="flex flex-wrap items-center gap-3">
        <Segmented label="Date range" options={ranges} value={range} onChange={(value) => set({ range: value, from: undefined, to: undefined })} />
        {range === "custom" && <><label className="text-14">From <input aria-label="From" type="date" className={field} value={dates.from} onChange={(e) => set({ from: e.target.value })} /></label><label className="text-14">To <input aria-label="To" type="date" className={field} value={dates.to} onChange={(e) => set({ to: e.target.value })} /></label></>}
        <label className="text-14">Persona <select className={field} value={persona} onChange={(e) => set({ persona: e.target.value })}><option value="all">All personas</option>{all.personas.map((p) => <option key={p.key} value={p.key}>{p.label}</option>)}</select></label>
        <Segmented label="Effort choices" options={[{ value: "all", label: "All effort choices" }, { value: "auto", label: "Auto only" }]} value={mode} onChange={(value) => set({ selection: value })} />
      </div>
      <p className="text-12 text-fg-tertiary">{dates.from} – {dates.to} UTC · One sample per replying turn. Older replies without effort metadata stay unclassified. This view includes your chats only; Costs also includes other run types.</p>
      {body}
    </div></Content>
  </>;
}

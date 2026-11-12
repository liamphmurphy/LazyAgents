"use client";

// The compact Runs list (DESIGN.md §3 Responsive, frame "7 Runs"): live runs as
// cards, then one card of runs that finished today. The desktop table is
// runs-screen.tsx; RunsTabs picks this one below COMPACT.

import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { useState } from "react";
import type { Schemas } from "@/lib/api/client";
import { useRunPages } from "@/lib/api/hooks";
import { duration } from "@/lib/format";
import { PURPOSE_LABEL } from "@/lib/run-view";
import { finishedLabel, finishedToday, livePill, stepProgress } from "@/lib/run-mobile";
import { AgentAvatar } from "@/components/domain/avatars";
import { Pill } from "@/components/domain/labels";
import { Button } from "@/components/ui/button";
import { Banner, EmptyState, Skeleton } from "@/components/ui/primitives";
import { useProjectScope } from "@/components/shell/project-scope";
import { RunsTabNav } from "./runs-tabs";

type Run = Schemas["Run"];

const titleOf = (run: Run) => run.work_title || run.ticket_key || PURPOSE_LABEL[run.purpose];

function LiveCard({ run }: { run: Run }) {
  const pill = livePill(run);
  const p = stepProgress(run);
  const pct = p.total ? Math.round((p.done / p.total) * 100) : 0;
  return (
    <li className="relative flex flex-col gap-2.5 rounded-xl border border-border bg-surface p-3.5">
      <div className="flex items-center gap-2">
        <AgentAvatar name={run.agent.name} colorIndex={run.agent.color_index} label={false} />
        <span className="min-w-0 flex-1 truncate text-[13px] font-medium">{run.agent.name}</span>
        <span className="font-mono text-xs text-fg-tertiary">{duration(run.started_at, run.ended_at)}</span>
        <Pill tone={pill.tone}>{pill.label}</Pill>
      </div>
      {/* The title link's ::after covers the card, so the whole card opens the run. */}
      <Link href={`/runs/${run.id}`} className="flex min-w-0 flex-col gap-0.5 after:absolute after:inset-0 after:rounded-xl after:content-['']">
        <span className="line-clamp-2 text-[15px] font-semibold leading-snug text-fg">{titleOf(run)}</span>
        {run.ticket_key && <span className="font-mono text-xs text-fg-secondary">{run.ticket_key}</span>}
      </Link>
      {p.total > 0 && (
        <div className="flex flex-col gap-1.5">
          <p className="flex items-baseline gap-2 text-[13px]">
            <span className="flex-none font-mono text-xs text-fg-tertiary">Step {p.current?.position ?? p.done} of {p.total}</span>
            <span className="min-w-0 truncate text-fg-2">{p.current?.text ?? (p.done === p.total ? "All steps done" : "")}</span>
          </p>
          <div
            role="progressbar"
            aria-label={`${run.agent.name} progress`}
            aria-valuemin={0}
            aria-valuemax={p.total}
            aria-valuenow={p.done}
            aria-valuetext={`${p.done} of ${p.total} steps done`}
            className="h-1.5 overflow-hidden rounded-full bg-surface-sunken"
          >
            <div className={pill.tone === "human" ? "h-full rounded-full bg-state-human" : "h-full rounded-full bg-state-done"} style={{ width: `${pct}%` }} />
          </div>
        </div>
      )}
    </li>
  );
}

function FinishedRow({ run }: { run: Run }) {
  const s = finishedLabel(run.state);
  return (
    <li className="relative flex min-h-[44px] items-center gap-2.5 border-b border-border-row px-3.5 py-2.5 last:border-b-0">
      <AgentAvatar name={run.agent.name} colorIndex={run.agent.color_index} label={false} />
      <Link href={`/runs/${run.id}`} className="flex min-w-0 flex-1 flex-col after:absolute after:inset-0 after:content-['']">
        <span className="truncate text-[13.5px] text-fg">{titleOf(run)}</span>
        <span className="truncate text-xs text-fg-tertiary">
          {run.agent.name}
          {run.ticket_key && <> · <span className="font-mono">{run.ticket_key}</span></>}
          {" · "}{duration(run.started_at, run.ended_at)}
        </span>
      </Link>
      <Pill tone={s.tone}>{s.label}</Pill>
    </li>
  );
}

/** Compact Runs: live cards over a "Finished today" card. SSE invalidates ["runs"]. */
export function CompactRunsScreen() {
  const params = useSearchParams();
  const scope = useProjectScope("optional");
  const project = params.has("project") ? params.get("project") || undefined : scope.status === "ready" ? scope.project?.key : undefined;
  const [live] = useRunPages({ state: "live", project }, 1);
  const [recent] = useRunPages({ state: "all", project }, 1);
  const [now] = useState(() => new Date());
  const liveRuns = live.data?.runs ?? [];
  const done = finishedToday(recent.data?.runs ?? [], now);
  const loading = live.isPending || scope.status === "loading";
  return (
    <div className="flex flex-col gap-4 px-4 pb-6 pt-safe">
      <div className="flex flex-col gap-3 pt-4">
        <h1 className="type-display">Runs</h1>
        <RunsTabNav tab="runs" />
      </div>
      {live.isError ? (
        <Banner tone="blocked" title="Couldn’t load runs" actions={<Button onClick={() => live.refetch()}>Retry</Button>}>
          {live.error.message}
        </Banner>
      ) : loading ? (
        <div aria-busy="true" className="flex flex-col gap-3">
          {[0, 1].map((i) => <Skeleton key={i} className="h-32 rounded-xl" />)}
        </div>
      ) : (
        <section aria-label="Live runs" className="flex flex-col gap-2">
          <h2 className="text-xs font-semibold uppercase tracking-wide text-fg-tertiary">Live · {liveRuns.length}</h2>
          {liveRuns.length ? (
            <ul className="flex flex-col gap-3">{liveRuns.map((r) => <LiveCard key={r.id} run={r} />)}</ul>
          ) : (
            <EmptyState icon="play" title="Nothing running">Dispatch a ticket from the Inbox to start a run.</EmptyState>
          )}
        </section>
      )}
      {done.length > 0 && (
        <section aria-label="Finished today" className="overflow-hidden rounded-xl border border-border bg-surface">
          <h2 className="border-b border-border-subtle px-3.5 py-2.5 text-sm font-semibold">Finished today</h2>
          <ul>{done.map((r) => <FinishedRow key={r.id} run={r} />)}</ul>
        </section>
      )}
    </div>
  );
}

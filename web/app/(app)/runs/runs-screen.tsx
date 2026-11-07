"use client";

import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { Popover } from "radix-ui";
import { useState, useSyncExternalStore } from "react";
import type { Schemas } from "@/lib/api/client";
import { useBootstrap, useRunnerStatus, useRunPages } from "@/lib/api/hooks";
import { absoluteTime, ago, duration, runCost } from "@/lib/format";
import { cx } from "@/lib/cx";
import {
  PURPOSE_LABEL, RUN_COLUMNS_KEY, advancedFilterCount, hasActiveFilters, parseRunFilters, runColumns, runFilterQuery, workHref,
  type RunColumns, type RunFilters,
} from "@/lib/run-view";
import { AgentAvatar } from "@/components/domain/avatars";
import { floatPanel } from "@/components/domain/pickers";
import { StatusPill } from "@/components/domain/run-status";
import { Button, ButtonLink } from "@/components/ui/button";
import { Segmented, Toggle } from "@/components/ui/controls";
import { Banner, EmptyState, Skeleton } from "@/components/ui/primitives";
import { useProjectScope } from "@/components/shell/project-scope";
import { Content, TopBar } from "@/components/shell/topbar";

type Run = Schemas["Run"];

const STATUS: { value: string; label: string }[] = [
  { value: "live", label: "Live" },
  { value: "all", label: "All" },
  { value: "waiting", label: "Waiting" },
  { value: "running", label: "Running" },
  { value: "queued", label: "Queued" },
  { value: "completed", label: "Completed" },
  { value: "failed", label: "Failed" },
  { value: "cancelled", label: "Cancelled" },
  { value: "paused", label: "Paused" },
];
const PURPOSES = [
  { value: "all", label: "All" },
  { value: "ticket", label: "Ticket" },
  { value: "planning", label: "Planning" },
  { value: "chat", label: "Chat" },
] as const;

const selectClass = "h-8 rounded-md border border-border-strong bg-surface px-2.5 text-[13px] text-fg";

/** The More columns choice: remembered in localStorage, overridden by ?columns=. */
const COLUMNS_CHANGED = "lazyagents:runs-columns";
function subscribeColumns(onChange: () => void) {
  window.addEventListener("storage", onChange);
  window.addEventListener(COLUMNS_CHANGED, onChange);
  return () => {
    window.removeEventListener("storage", onChange);
    window.removeEventListener(COLUMNS_CHANGED, onChange);
  };
}
function readColumns(): string | null {
  try {
    return window.localStorage.getItem(RUN_COLUMNS_KEY);
  } catch {
    return null;
  }
}
function rememberColumns(c: RunColumns) {
  try {
    if (c === "more") window.localStorage.setItem(RUN_COLUMNS_KEY, c);
    else window.localStorage.removeItem(RUN_COLUMNS_KEY);
  } catch {
    /* Storage unavailable: the URL still carries the choice. */
  }
  window.dispatchEvent(new Event(COLUMNS_CHANGED));
}

function RunRow({ run, more }: { run: Run; more: boolean }) {
  const href = workHref(run);
  const title = run.work_title || (run.purpose === "ticket" ? "" : PURPOSE_LABEL[run.purpose]);
  // In the default set the Work cell carries the purpose: the ticket key, or "Planning" / "Chat".
  const prefix = run.ticket_key || (more || !run.work_title ? "" : PURPOSE_LABEL[run.purpose]);
  return (
    <tr className="relative border-b border-border-row last:border-b-0 hover:bg-hover">
      <td className="px-3 py-2.5">
        <StatusPill run={run} />
        {/* Under 768px the Agent column is hidden, so this link covers the row instead. */}
        <Link href={`/runs/${run.id}`} className="after:absolute after:inset-0 after:content-[''] md:hidden">
          <span className="sr-only">{run.agent.name}</span>
        </Link>
      </td>
      <td className="px-3 py-2.5 max-md:hidden">
        <span className="flex items-center gap-2">
          <AgentAvatar name={run.agent.name} colorIndex={run.agent.color_index} label={false} />
          {/* The row link lives on the agent name; its ::after covers the whole row. */}
          <Link href={`/runs/${run.id}`} className="truncate font-medium after:absolute after:inset-0 after:content-['']">{run.agent.name}</Link>
        </span>
      </td>
      {more && <td className="px-3 py-2.5 text-fg-secondary max-md:hidden">{PURPOSE_LABEL[run.purpose]}</td>}
      <td className="max-w-0 px-3 py-2.5">
        <span className="flex min-w-0 items-baseline gap-2">
          {prefix && <span className={cx("flex-none text-fg-secondary", run.ticket_key ? "font-mono text-xs" : "text-xs")}>{run.ticket_key || `${prefix} ·`}</span>}
          {href ? (
            <Link href={href} className="relative z-10 min-w-0 truncate text-fg-2 hover:underline">{title || run.ticket_key}</Link>
          ) : (
            <span className="min-w-0 truncate text-fg-2">{title}</span>
          )}
        </span>
      </td>
      {more && <td className="px-3 py-2.5 max-md:hidden"><span title={run.harness} className="font-mono text-xs text-fg-secondary">{run.model}</span></td>}
      {more && <td className="whitespace-nowrap px-3 py-2.5 text-xs text-fg-tertiary max-md:hidden" title={absoluteTime(run.started_at)}>{ago(run.started_at)}</td>}
      <td className="whitespace-nowrap px-3 py-2.5 font-mono text-xs text-fg-secondary">{duration(run.started_at, run.ended_at)}</td>
      <td className="whitespace-nowrap px-3 py-2.5 font-mono text-xs text-fg-secondary max-md:hidden">{runCost(run.cost_usd, run.cost_source)}</td>
      {more && <td className="px-3 py-2.5 text-right font-mono text-xs text-fg-tertiary max-md:hidden">{run.child_count || "–"}</td>}
    </tr>
  );
}

function Filters({ filters, set, clear }: { filters: RunFilters; set: (patch: Partial<RunFilters>) => void; clear: () => void }) {
  const boot = useBootstrap();
  const agents = boot.data?.agents ?? [];
  const projects = (boot.data?.projects ?? []).filter((p) => !p.archived || p.key === filters.project);
  const count = advancedFilterCount(filters);
  // ?state=starting or ?state=blocked from older links: keep it selectable so the select shows it.
  const states = STATUS.some((s) => s.value === filters.state) ? STATUS : [...STATUS, { value: filters.state, label: filters.state[0].toUpperCase() + filters.state.slice(1) }];
  return (
    <div className="flex flex-wrap items-end gap-x-4 gap-y-3">
      <label className="flex flex-col gap-1 text-xs text-fg-tertiary">
        Status
        <select className={selectClass} value={filters.state} onChange={(e) => set({ state: e.target.value })}>
          {states.map((s) => <option key={s.value} value={s.value}>{s.label}</option>)}
        </select>
      </label>
      <label className="flex flex-col gap-1 text-xs text-fg-tertiary">
        Project
        <select className={selectClass} value={filters.project ?? ""} onChange={(e) => set({ project: e.target.value || undefined })}>
          <option value="">All projects</option>
          {projects.map((p) => <option key={p.key} value={p.key}>{p.name}</option>)}
        </select>
      </label>
      <Popover.Root>
        <Popover.Trigger asChild>
          <Button variant="outline" icon="sliders" aria-label={count ? `Filters, ${count} active` : "Filters"}>
            Filters
            {count > 0 && <span aria-hidden className="ml-0.5 rounded-full bg-accent px-1.5 text-[11px] font-medium leading-4 text-on-accent">{count}</span>}
          </Button>
        </Popover.Trigger>
        <Popover.Portal>
          <Popover.Content align="start" sideOffset={6} aria-label="Filters" className={cx(floatPanel, "flex w-[280px] flex-col gap-3 p-3")}>
            <label className="flex flex-col gap-1 text-xs text-fg-tertiary">
              Agent
              <select className={selectClass} value={filters.agent ?? ""} onChange={(e) => set({ agent: e.target.value || undefined })}>
                <option value="">All agents</option>
                {agents.map((a) => <option key={a.handle} value={a.handle}>{a.name}</option>)}
              </select>
            </label>
            <div className="flex flex-col gap-1 text-xs text-fg-tertiary">
              Purpose
              <Segmented
                label="Purpose"
                options={PURPOSES}
                value={filters.purpose ?? "all"}
                onChange={(v) => set({ purpose: v === "all" ? undefined : v })}
              />
            </div>
            <div className="flex items-center justify-between border-t border-border pt-3">
              <Button variant="ghost" disabled={!hasActiveFilters(filters)} onClick={clear}>Clear filters</Button>
              <Popover.Close asChild><Button variant="outline">Done</Button></Popover.Close>
            </div>
          </Popover.Content>
        </Popover.Portal>
      </Popover.Root>
      {hasActiveFilters(filters) && <Button variant="ghost" onClick={clear}>Clear filters</Button>}
    </div>
  );
}

/** Runs across agents. SSE invalidates ["runs"] and ["runner"]. */
export function RunsScreen() {
  const params = useSearchParams();
  const router = useRouter();
  const scope = useProjectScope("optional");
  const runner = useRunnerStatus();
  const urlFilters = parseRunFilters(params);
  // A bare /runs starts in the shell's project; an explicit ?project= (or "All projects") wins.
  const scoped = scope.status === "ready" ? scope.project?.key : undefined;
  const filters: RunFilters = { ...urlFilters, project: urlFilters.project ?? (params.has("project") ? undefined : scoped) };
  const query = runFilterQuery(filters);
  const [loaded, setLoaded] = useState({ query, pages: 1 });
  const pages = loaded.query === query ? loaded.pages : 1;
  const results = useRunPages(filters, pages);
  const stored = useSyncExternalStore(subscribeColumns, readColumns, () => undefined);
  const columns = runColumns(params.get("columns"), stored);
  const more = columns === "more";

  // ?columns= is a view setting, not a filter: carry it over when the filters change.
  const go = (q: URLSearchParams) => {
    const c = params.get("columns");
    if (c) q.set("columns", c);
    router.replace(`/runs${q.size ? `?${q}` : ""}`);
  };
  const set = (patch: Partial<RunFilters>) => {
    const next = { ...filters, ...patch };
    // Picking "All projects" must stay explicit, so it is not re-defaulted from the shell scope.
    const q = new URLSearchParams(runFilterQuery(next));
    if (!next.project && "project" in patch) q.set("project", "");
    go(q);
  };
  const clear = () => go(new URLSearchParams("project="));
  const setColumns = (on: boolean) => {
    const c: RunColumns = on ? "more" : "default";
    rememberColumns(c);
    const q = new URLSearchParams(params);
    if (on) q.set("columns", c);
    else q.delete("columns");
    router.replace(`/runs${q.size ? `?${q}` : ""}`);
  };

  const first = results[0];
  const runs = [...new Map(results.flatMap((r) => r.data?.runs ?? []).map((run) => [run.id, run])).values()];
  const last = results[results.length - 1];
  const r = runner.data;
  const scopeLoading = scope.status === "loading";
  return (
    <>
      <TopBar crumbs={[{ label: "Runs" }]} actions={<ButtonLink href="/inbox" variant="outline">Dispatch from Inbox</ButtonLink>} />
      <Content>
        <div className="flex flex-col gap-5">
          <div className="flex flex-wrap items-end justify-between gap-3">
            <div className="space-y-1">
              <h1 className="type-display">Runs</h1>
              <p className="text-sm text-fg-secondary">Ticket, planning and chat runs across agents.</p>
            </div>
            <p role="status" className="flex items-center gap-3 font-mono text-xs text-fg-tertiary">
              {r && (r.online ? `${r.busy} running · ${r.queued} queued · ${r.slots} slots` : `Runner offline · ${r.queued} queued`)}
              <span className="flex items-center gap-1.5 text-pill-done-fg"><span aria-hidden className="size-1.5 rounded-full bg-state-done" />Live</span>
            </p>
          </div>
          <div className="flex flex-wrap items-end justify-between gap-3">
            <Filters filters={filters} set={set} clear={clear} />
            <Toggle checked={more} onChange={setColumns} label="More columns" />
          </div>
          {first.isError ? (
            <Banner tone="blocked" title="Couldn’t load runs" actions={<Button onClick={() => first.refetch()}>Retry</Button>}>
              {first.error.message}
            </Banner>
          ) : first.isPending || scopeLoading ? (
            <div aria-busy="true" className="flex flex-col gap-2">
              {[0, 1, 2].map((i) => <Skeleton key={i} className="h-11" />)}
            </div>
          ) : runs.length === 0 ? (
            <EmptyState icon="play" title="No runs match these filters" actions={<Button variant="outline" onClick={clear}>Clear filters</Button>}>
              Try another status, or dispatch a ticket from the Inbox.
            </EmptyState>
          ) : (
            <div className="overflow-hidden rounded-xl border border-border bg-surface">
              <table className="w-full table-fixed text-left text-[13px] max-md:table-auto">
                <caption className="sr-only">Runs, newest first</caption>
                <thead className="border-b border-border text-xs text-fg-tertiary">
                  <tr>
                    <th scope="col" className="w-36 px-3 py-2 font-medium">Status</th>
                    <th scope="col" className="w-40 px-3 py-2 font-medium max-md:hidden">Agent</th>
                    {more && <th scope="col" className="w-28 px-3 py-2 font-medium max-md:hidden">Purpose</th>}
                    <th scope="col" className="px-3 py-2 font-medium">Work</th>
                    {more && <th scope="col" className="w-40 px-3 py-2 font-medium max-md:hidden">Model</th>}
                    {more && <th scope="col" className="w-24 px-3 py-2 font-medium max-md:hidden">Started</th>}
                    <th scope="col" className="w-20 px-3 py-2 font-medium">Elapsed</th>
                    <th scope="col" className="w-24 px-3 py-2 font-medium max-md:hidden">Cost</th>
                    {more && <th scope="col" className="w-20 px-3 py-2 text-right font-medium max-md:hidden">Sub runs</th>}
                  </tr>
                </thead>
                <tbody>{runs.map((run) => <RunRow key={run.id} run={run} more={more} />)}</tbody>
              </table>
              <div className="flex items-center justify-between border-t border-border px-4 py-2.5 text-xs text-fg-tertiary">
                <span>{runs.length} {runs.length === 1 ? "run" : "runs"} · newest first</span>
                {last.data?.has_more && (
                  <Button variant="outline" disabled={last.isFetching && last.isPlaceholderData} onClick={() => setLoaded({ query, pages: pages + 1 })}>
                    Load more
                  </Button>
                )}
              </div>
            </div>
          )}
          {results.some((x, i) => i > 0 && x.isError) && (
            <Banner tone="blocked" title="Couldn’t load more runs">Try again in a moment.</Banner>
          )}
        </div>
      </Content>
    </>
  );
}

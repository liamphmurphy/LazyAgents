"use client";

import Link from "next/link";
import type { Schemas } from "@/lib/api/client";
import { useAgentOverviews, useRunnerStatus } from "@/lib/api/hooks";
import { ago } from "@/lib/format";
import { AgentAvatar } from "@/components/domain/avatars";
import { Button, ButtonLink } from "@/components/ui/button";
import { Banner, EmptyState, Skeleton } from "@/components/ui/primitives";
import { Content, TopBar } from "@/components/shell/topbar";

type Row = {
  id: string;
  agent: Schemas["Agent"];
  label: string;
  title: string;
  state: string;
  model: string;
  started?: string;
};

/** Live work across agents. SSE invalidates ["agent-overviews"] and ["runner"]. */
export function RunsScreen() {
  const overviews = useAgentOverviews();
  const runner = useRunnerStatus();
  const rows: Row[] = (overviews.data?.agents ?? [])
    .flatMap((o) => [
      ...o.planning_runs.map((w) => ({
        id: w.run_id, agent: o.agent, label: "Planning", title: w.goal, state: w.state, model: `${w.harness} · ${w.model}`,
      })),
      ...o.assignments.map((w) => ({
        id: w.run_id, agent: o.agent, label: w.ticket.key, title: w.step || w.ticket.title, state: w.state,
        model: `${w.harness} · ${w.model}`, started: w.started_at,
      })),
    ])
    .sort((a, b) => Number(a.state === "queued") - Number(b.state === "queued"));
  const r = runner.data;
  return (
    <>
      <TopBar crumbs={[{ label: "Runs" }]} actions={<ButtonLink href="/inbox" variant="outline">Dispatch from Inbox</ButtonLink>} />
      <Content>
        <div className="flex flex-col gap-6">
          <div className="flex flex-wrap items-end justify-between gap-3">
            <div className="space-y-1">
              <h1 className="type-display">Runs</h1>
              <p className="text-sm text-fg-secondary">Ticket and planning runs in progress or waiting for a runner.</p>
            </div>
            {r && (
              <p role="status" className="font-mono text-xs text-fg-tertiary">
                {r.online ? `${r.busy} running · ${r.queued} queued · ${r.slots} slots` : `Runner offline · ${r.queued} queued`}
              </p>
            )}
          </div>
          {overviews.isError ? (
            <Banner tone="blocked" title="Couldn’t load runs" actions={<Button onClick={() => overviews.refetch()}>Retry</Button>}>
              {overviews.error.message}
            </Banner>
          ) : overviews.isPending ? (
            <div aria-busy="true" className="flex flex-col gap-2">
              {[0, 1, 2].map((i) => <Skeleton key={i} className="h-11" />)}
            </div>
          ) : rows.length === 0 ? (
            <EmptyState
              icon="play"
              title="No runs in progress"
              actions={<ButtonLink href="/inbox" variant="outline">Open Inbox</ButtonLink>}
            >
              Dispatch a ready ticket or start a plan, and its run appears here.
            </EmptyState>
          ) : (
            <ul aria-label="Live runs" className="overflow-hidden rounded-xl border border-border bg-surface">
              {rows.map((row) => (
                <li key={row.id} className="border-b border-border-row last:border-b-0">
                  <Link href={`/runs/${row.id}`} className="flex min-h-[44px] items-center gap-3 px-4 py-2 text-[13px] hover:bg-hover">
                    <AgentAvatar name={row.agent.name} colorIndex={row.agent.color_index} label={false} />
                    <span className="sr-only">{row.agent.name}</span>
                    <span className="w-24 flex-none truncate font-mono text-xs text-fg-secondary">{row.label}</span>
                    <span className="min-w-0 flex-1 truncate text-fg-2">{row.title}</span>
                    <span className="flex-none text-xs text-fg-tertiary">{row.state.replaceAll("_", " ")}</span>
                    <span className="hidden w-48 flex-none truncate text-right font-mono text-[11px] text-fg-tertiary md:block">{row.model}</span>
                    <span className="w-16 flex-none text-right text-xs text-fg-tertiary">{row.started ? ago(row.started) : ""}</span>
                  </Link>
                </li>
              ))}
            </ul>
          )}
        </div>
      </Content>
    </>
  );
}

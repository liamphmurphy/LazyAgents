"use client";

import Link from "next/link";
import { useState } from "react";
import { useAgentOverviews, useAgentRuns } from "@/lib/api/hooks";
import { ago, compact, duration } from "@/lib/format";
import { Button } from "@/components/ui/button";
import { Banner, Card, EmptyState, Skeleton } from "@/components/ui/primitives";
import { KpiTile } from "@/components/domain/data";
import {
  Assignment,
  merged,
  money,
  seconds,
  WorkRow,
} from "@/components/domain/overview";
import { EnforcementChip, ScopeMode } from "@/components/domain/labels";

export function OverviewTab({ handle }: { handle: string }) {
  const query = useAgentOverviews();
  const data = query.data?.agents.find((a) => a.agent.handle === handle);
  if (query.isError)
    return (
      <Banner
        tone="blocked"
        title="Couldn’t load agent activity"
        actions={<Button onClick={() => query.refetch()}>Retry</Button>}
      >
        {query.error.message}
      </Banner>
    );
  if (!data)
    return (
      <div aria-busy="true">
        <Skeleton className="h-64" />
      </div>
    );
  return (
    <div className="space-y-5">
      <div className="grid grid-cols-2 gap-3">
        <KpiTile label="Done · 7 days" value={data.stats.completed} />
        <KpiTile
          label="Merged PRs · 7 days"
          value={merged(data.stats)}
          context={`${data.stats.merged_prs}/${data.stats.closed_prs} closed PRs`}
        />
        <KpiTile
          label="Median cycle · 7 days"
          value={seconds(data.stats.median_cycle_seconds)}
        />
        <KpiTile
          label="Run spend · 7 days"
          value={money(data.stats.spend_usd)}
          context="runs started in window"
        />
      </div>
      <Card title="Current assignments">
        {data.assignments.length ? (
          <div className="space-y-3">
            {data.assignments.map((w) => (
              <Assignment key={w.run_id} work={w} />
            ))}
          </div>
        ) : (
          <p className="text-sm text-fg-tertiary">No active runs.</p>
        )}
      </Card>
      <Card
        title={`Awaiting dispatch · ${data.queue_count}`}
        bodyClassName="p-0"
      >
        {data.queue.length ? (
          data.queue.map((t) => <WorkRow key={t.id} ticket={t} />)
        ) : (
          <p className="p-4 text-sm text-fg-tertiary">
            No assigned tickets are waiting for dispatch.
          </p>
        )}
        {data.queue_count > data.queue.length && (
          <Link
            className="block px-4 py-3 text-xs text-accent"
            href={`/tickets?view=all&assignee=${handle}`}
          >
            View all assigned tickets →
          </Link>
        )}
      </Card>
      <Card title="Definition permissions">
        <div className="space-y-3">
          {data.scopes.map((s) => (
            <div key={s.scope} className="space-y-1">
              <div className="flex flex-wrap items-center gap-2">
                <span className="mr-auto font-mono text-xs">{s.scope}</span>
                <ScopeMode mode={s.mode} />
                <EnforcementChip level={s.enforcement} />
              </div>
              <p className="text-xs text-fg-tertiary">{s.how}</p>
            </div>
          ))}
        </div>
        <p className="mt-4 text-xs text-fg-tertiary">
          These are the base definition permissions; the resolved project
          definition applies to each new run.
        </p>
      </Card>
    </div>
  );
}
export function RunsTab({ handle }: { handle: string }) {
  const [offset, setOffset] = useState(0);
  const query = useAgentRuns(handle, offset);
  if (query.isError)
    return (
      <Banner
        tone="blocked"
        title="Couldn’t load runs"
        actions={<Button onClick={() => query.refetch()}>Retry</Button>}
      >
        {query.error.message}
      </Banner>
    );
  if (!query.data)
    return (
      <div aria-busy="true">
        <Skeleton className="h-64" />
      </div>
    );
  return (
    <Card title="Run history" bodyClassName="p-0">
      {query.data.runs.length ? (
        <div className="divide-y divide-border-subtle">
          {query.data.runs.map((r) => (
            <Link
              key={r.id}
              href={`/runs/${r.id}`}
              className="block space-y-2 px-4 py-3 hover:bg-row-hover"
            >
              <div className="flex flex-wrap items-center gap-2">
                <span className="font-mono text-xs text-fg-secondary">
                  {r.ticket.key}
                </span>
                <span className="min-w-0 flex-1 truncate text-sm font-medium">
                  {r.ticket.title}
                </span>
                <span className="text-xs text-fg-tertiary">{r.state}</span>
              </div>
              <p className="font-mono text-[11px] text-fg-tertiary">
                {r.harness} · {r.model} · {r.tool_calls} tools ·{" "}
                {compact(r.tokens)} tokens · {money(r.cost_usd)}
              </p>
              <p className="text-xs text-fg-tertiary">
                Started {ago(r.started_at)} ·{" "}
                {duration(r.started_at, r.ended_at)} elapsed
              </p>
            </Link>
          ))}
        </div>
      ) : (
        <EmptyState icon="play" title="No runs yet">
          Dispatch an assigned ticket to start this agent.
        </EmptyState>
      )}
      <div className="flex items-center justify-between gap-2 border-t border-border-subtle p-3">
        <Button
          disabled={offset === 0}
          onClick={() => setOffset(Math.max(0, offset - 50))}
        >
          Previous
        </Button>
        <span className="font-mono text-xs text-fg-tertiary">
          {query.data.runs.length
            ? `${offset + 1}–${offset + query.data.runs.length}`
            : "0 runs"}
        </span>
        <Button
          disabled={!query.data.has_more}
          onClick={() => setOffset(offset + 50)}
        >
          Next
        </Button>
      </div>
    </Card>
  );
}

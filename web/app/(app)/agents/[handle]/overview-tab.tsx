"use client";

import Link from "next/link";
import { useState } from "react";
import type { AgentDetail, Schemas } from "@/lib/api/client";
import { useAgentOverviews, useAgentRuns } from "@/lib/api/hooks";
import { liveCounts } from "@/lib/agent-scopes";
import { ago, duration, runCost, spendCost } from "@/lib/format";
import { Button } from "@/components/ui/button";
import { Segmented } from "@/components/ui/controls";
import { Banner, Card, EmptyState, Skeleton, TextArea } from "@/components/ui/primitives";
import { KpiTile } from "@/components/domain/data";
import { Flag, Pill, type PillTone } from "@/components/domain/labels";
import { Markdown } from "@/components/domain/markdown";
import { Assignment, PlanningAssignment, merged, WorkRow } from "@/components/domain/overview";
import type { Draft } from "./definition-editor";

/** Live work and the last 7 days, then the instructions (rendered, or edited into the draft). */
export function OverviewTab({
  detail,
  draft,
  onChange,
  project,
  onPreview,
}: {
  detail: AgentDetail;
  draft: Draft;
  onChange: (d: Draft) => void;
  project?: Schemas["Project"];
  onPreview: () => void;
}) {
  const handle = detail.agent.handle;
  const query = useAgentOverviews();
  const data = query.data?.agents.find((a) => a.agent.handle === handle);
  const [mode, setMode] = useState<"rendered" | "edit">("rendered");
  const edited = draft.spec.instructions !== detail.spec.instructions;
  const append = detail.overrides.find((o) => o.project.key === project?.key)?.override.instructions_append?.trim();
  const appendLines = append ? append.split("\n").length : 0;

  return (
    <div className="space-y-5">
      {query.isError ? (
        <Banner tone="blocked" title="Couldn’t load agent activity" actions={<Button onClick={() => query.refetch()}>Retry</Button>}>
          {query.error.message}
        </Banner>
      ) : !data ? (
        <Skeleton className="h-40" />
      ) : (
        <>
          <div className="grid grid-cols-[repeat(auto-fit,minmax(150px,1fr))] gap-3">
            <KpiTile label="Live runs" value={liveCounts(data).live} />
            <KpiTile label="Merged PRs · 7d" value={merged(data.stats)} context={`${data.stats.merged_prs} of ${data.stats.closed_prs} closed`} />
            <KpiTile label="Done · 7d" value={data.stats.completed} />
            <KpiTile label="Spend · 7d" value={spendCost(data.stats.spend_usd, data.stats.unpriced_runs)} context="runs started in window" />
          </div>
          {data.planning_runs.length > 0 && (
            <Card title="Planning runs">
              <div className="space-y-3">
                {data.planning_runs.map((p) => (
                  <PlanningAssignment key={p.id} work={p} />
                ))}
              </div>
            </Card>
          )}
          <Card title="Current assignments">
            {data.assignments.length ? (
              <div className="space-y-3">
                {data.assignments.map((w) => (
                  <Assignment key={w.run_id} work={w} />
                ))}
              </div>
            ) : (
              <p className="text-14 text-fg-tertiary">No active runs.</p>
            )}
          </Card>
          {detail.agent.kind === "main" && (
            <Card title={`Awaiting dispatch · ${data.queue_count}`} bodyClassName="p-0">
              {data.queue.length ? (
                data.queue.map((t) => <WorkRow key={t.id} ticket={t} />)
              ) : (
                <p className="p-4 text-14 text-fg-tertiary">No assigned tickets are waiting for dispatch.</p>
              )}
              {data.queue_count > data.queue.length && (
                <Link className="block px-4 py-3 text-12 text-accent" href={`/tickets?view=all&assignee=${handle}`}>
                  View all assigned tickets →
                </Link>
              )}
            </Card>
          )}
        </>
      )}

      <Card
        title={
          <span className="flex items-center gap-2">
            Instructions
            {edited && <Flag tone="human">edited</Flag>}
          </span>
        }
        action={
          <span className="flex items-center gap-2">
            {edited && (
              <Button variant="ghost" className="h-7 px-2 text-12" onClick={onPreview}>
                Preview diff
              </Button>
            )}
            <Segmented
              small
              label="Instructions view"
              value={mode}
              onChange={setMode}
              options={[
                { value: "rendered", label: "Rendered" },
                { value: "edit", label: "Edit" },
              ]}
            />
          </span>
        }
        bodyClassName="flex flex-col gap-3 px-[18px] py-4"
      >
        {mode === "edit" ? (
          <TextArea
            aria-label="Instructions"
            mono
            autoFocus
            rows={16}
            value={draft.spec.instructions}
            onChange={(e) => onChange({ ...draft, spec: { ...draft.spec, instructions: e.target.value } })}
          />
        ) : draft.spec.instructions.trim() ? (
          <Markdown className="text-13">{draft.spec.instructions}</Markdown>
        ) : (
          <p className="text-14 text-fg-tertiary">No instructions yet. Switch to Edit to write them.</p>
        )}
        <p className="text-12 text-fg-tertiary">
          Written to <code className="inline">AGENTS.md</code> in each run&apos;s workspace; a repo&apos;s own AGENTS.md is kept.
          {project && appendLines > 0 && ` In ${project.name}, the project override appends ${appendLines} line${appendLines === 1 ? "" : "s"}.`}
        </p>
        <p className="text-12 text-fg-tertiary">Edits join the unsaved changes in the top bar; nothing is saved until you review the diff.</p>
      </Card>
    </div>
  );
}

const RUN_STATE: Record<string, { tone: PillTone; label: string }> = {
  queued: { tone: "neutral", label: "Queued" },
  starting: { tone: "review", label: "Starting" },
  running: { tone: "review", label: "Running" },
  waiting: { tone: "human", label: "Waiting" },
  paused: { tone: "human", label: "Paused" },
  blocked: { tone: "blocked", label: "Blocked" },
  completed: { tone: "done", label: "Completed" },
  failed: { tone: "blocked", label: "Failed" },
  cancelled: { tone: "neutral", label: "Cancelled" },
};

const PAGE = 50;

/** Recent ticket runs with status and cost; each row opens the run log. */
export function ActivityTab({ handle }: { handle: string }) {
  const [offset, setOffset] = useState(0);
  const query = useAgentRuns(handle, offset);
  if (query.isError)
    return (
      <Banner tone="blocked" title="Couldn’t load runs" actions={<Button onClick={() => query.refetch()}>Retry</Button>}>
        {query.error.message}
      </Banner>
    );
  if (!query.data)
    return (
      <div aria-busy="true">
        <Skeleton className="h-64" />
      </div>
    );
  const runs = query.data.runs;
  return (
    <Card
      title="Recent runs"
      action={
        <span className="text-12 text-fg-tertiary">
          Cost per run · planning runs are in{" "}
          <Link href="/plans" className="text-accent">
            Plans
          </Link>
        </span>
      }
      bodyClassName="p-0"
    >
      {runs.length ? (
        <div className="overflow-x-auto">
          <table className="w-full min-w-[600px] border-collapse text-13">
            <thead>
              <tr className="text-left text-12 text-fg-tertiary">
                <th className="px-4 py-2.5 font-normal">Status</th>
                <th className="px-2 py-2.5 font-normal">Ticket</th>
                <th className="px-2 py-2.5 font-normal">Started · elapsed</th>
                <th className="px-4 py-2.5 text-right font-normal">Cost</th>
              </tr>
            </thead>
            <tbody>
              {runs.map((r) => {
                const st = RUN_STATE[r.state] ?? { tone: "neutral" as const, label: r.state };
                return (
                  <tr key={r.id} className="relative border-t border-border-row hover:bg-row-hover">
                    <td className="px-4 py-2.5 align-top">
                      <Pill tone={st.tone}>{st.label}</Pill>
                    </td>
                    <td className="px-2 py-2.5">
                      <Link href={`/runs/${r.id}`} className="flex min-w-0 items-baseline gap-2 after:absolute after:inset-0">
                        <span className="flex-none font-mono text-12 text-fg-secondary">{r.ticket.key}</span>
                        <span className="truncate font-medium">{r.ticket.title}</span>
                      </Link>
                      <span className="block font-mono text-11 text-fg-tertiary">
                        {r.model} · {r.tool_calls} tool{r.tool_calls === 1 ? "" : "s"}
                      </span>
                    </td>
                    <td className="whitespace-nowrap px-2 py-2.5 align-top text-12 text-fg-secondary">
                      {ago(r.started_at)} · {duration(r.started_at, r.ended_at)}
                    </td>
                    <td className="whitespace-nowrap px-4 py-2.5 text-right align-top font-mono text-12">{r.cost_source && r.cost_source !== "unknown" ? runCost(r.cost_usd, r.cost_source) : "—"}</td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      ) : (
        <EmptyState icon="play" title="No runs yet">
          Dispatch an assigned ticket to start this agent.
        </EmptyState>
      )}
      <div className="flex items-center justify-between gap-2 border-t border-border-subtle p-3">
        <Button disabled={offset === 0} onClick={() => setOffset(Math.max(0, offset - PAGE))}>
          Previous
        </Button>
        <span className="font-mono text-12 text-fg-tertiary">{runs.length ? `${offset + 1}–${offset + runs.length}` : "0 runs"}</span>
        <Button disabled={!query.data.has_more} onClick={() => setOffset(offset + PAGE)}>
          Next
        </Button>
      </div>
      <p className="border-t border-border-subtle px-4 py-2.5 text-12 text-fg-tertiary">
        Each row opens the run log. Cost is the harness-reported or priced cost; unpriced runs show &ldquo;—&rdquo;.
      </p>
    </Card>
  );
}

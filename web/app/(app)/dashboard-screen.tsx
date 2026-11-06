"use client";

import Link from "next/link";
import { useState } from "react";
import type { Schemas } from "@/lib/api/client";
import {
  useBootstrap,
  useDashboard,
  type DashboardRange,
} from "@/lib/api/hooks";
import { ago, absoluteTime } from "@/lib/format";
import { Button, ButtonLink } from "@/components/ui/button";
import { Segmented } from "@/components/ui/controls";
import { Banner, Card, EmptyState, Skeleton } from "@/components/ui/primitives";
import { Content, TopBar } from "@/components/shell/topbar";
import { KpiTile, ProgressBar } from "@/components/domain/data";
import {
  AGENT_STATUS,
  AgentStack,
  AgentStatus,
  Assignment,
  PlanningAssignment,
  seconds,
  WorkflowBar,
} from "@/components/domain/overview";
import { AgentAvatar } from "@/components/domain/avatars";
import { DocTypeTag, Flag } from "@/components/domain/labels";
import { RunnerCapacity } from "@/components/domain/runner-capacity";
import { DispatchDialog } from "@/components/domain/dispatch-dialog";
import { Greeting } from "./greeting";

function AttentionRow({ item }: { item: Schemas["InboxItem"] }) {
  const [dispatch, setDispatch] = useState(false);
  const names = {
    approval: "Approval",
    dispatch: "Dispatch",
    review: "Review",
    blocked: "Blocked",
    mention: "Mention",
    planning: "Review plan",
    suggestion: "Wiki edit",
    mockup: "Mockup",
  };
  return (
    <div className="flex flex-wrap items-center gap-3 border-b border-border-subtle px-[18px] py-3 last:border-0">
      <div className="min-w-0 flex-[1_1_230px]">
        <div className="mb-1 flex items-center gap-2">
          <span className="font-mono text-xs text-fg-tertiary">
            {item.ticket_key || item.project || "Wiki"}
          </span>
          <Flag tone={item.kind === "blocked" ? "blocked" : "human"}>
            {names[item.kind]}
          </Flag>
        </div>
        <Link
          href={item.href}
          className="line-clamp-1 text-[13px] font-medium hover:text-accent"
        >
          {item.title}
        </Link>
        <p
          className="mt-1 truncate text-xs text-fg-tertiary"
          title={item.summary}
        >
          {item.summary} · {ago(item.created_at)}
        </p>
      </div>
      <div className="flex gap-1">
        {item.kind === "dispatch" ? (
          <Button variant="approve" onClick={() => setDispatch(true)}>
            Review &amp; dispatch
          </Button>
        ) : (
          <ButtonLink
            href={item.href}
            variant={item.kind === "approval" || item.kind === "mockup" ? "approve" : "outline"}
          >
            {item.kind === "approval"
              ? "Review request"
              : item.kind === "suggestion"
                ? "Review edit"
                : item.kind === "mockup"
                  ? "Review mockup"
                  : "Open"}
          </ButtonLink>
        )}
        <ButtonLink href="/inbox" variant="ghost">
          Inbox
        </ButtonLink>
      </div>
      {dispatch && (
        <DispatchDialog
          keys={[item.ticket_key]}
          open={dispatch}
          onOpenChange={setDispatch}
        />
      )}
    </div>
  );
}
export function DashboardScreen() {
  const [range, setRange] = useState<DashboardRange>("7d");
  const query = useDashboard(range),
    boot = useBootstrap();
  const d = query.data;
  const live = d?.agents.filter((a) => a.agent.status !== "idle") ?? [];
  const running =
    d?.agents.filter((a) => a.agent.status === "running").length ?? 0;
  const period =
    range === "24h" ? "24 hours" : range === "7d" ? "7 days" : "30 days";
  const max = Math.max(1, ...(d?.stats.daily.map((day) => day.count) ?? []));
  return (
    <>
      <TopBar
        crumbs={[{ label: "Dashboard" }]}
        actions={
          <ButtonLink
            href="/tickets/new"
            variant="primary"
            icon="plus"
            hint="C"
          >
            New ticket
          </ButtonLink>
        }
      />
      <Content>
        <div className="flex flex-col gap-6">
          <div className="flex flex-wrap items-start justify-between gap-4">
            <div className="space-y-1">
              <Greeting />
              <p className="text-sm text-fg-secondary">
                {d
                  ? `${d.counts.in_progress + d.counts.in_review} tickets in motion · ${boot.data?.counts.inbox ?? d.attention.length} items need your attention.`
                  : "Your workspace at a glance."}
              </p>
            </div>
            <Segmented
              label="Statistics range"
              value={range}
              onChange={setRange}
              options={[
                { value: "24h", label: "24h" },
                { value: "7d", label: "7d" },
                { value: "30d", label: "30d" },
              ]}
            />
          </div>
          {query.isError && (
            <Banner
              tone="blocked"
              title="Couldn’t load the dashboard"
              actions={<Button onClick={() => query.refetch()}>Retry</Button>}
            >
              {query.error.message}
            </Banner>
          )}
          {!d && !query.isError ? (
            <div
              aria-busy="true"
              className="grid grid-cols-2 gap-4 max-sm:grid-cols-1"
            >
              {[0, 1, 2, 3].map((i) => (
                <Skeleton key={i} className="h-32 rounded-xl" />
              ))}
            </div>
          ) : (
            d && (
              <>
                <div className="grid grid-cols-2 gap-4 xl:grid-cols-4 max-sm:grid-cols-1">
                  <KpiTile
                    label="Active agents"
                    value={running}
                    context={`of ${d.agents.length} agents`}
                    visual={
                      <div
                        role="img"
                        aria-label={d.agents
                          .map((a) => `${a.agent.name}: ${a.agent.status}`)
                          .join(", ")}
                        className="mt-1 flex flex-wrap gap-1.5"
                      >
                        {d.agents.map((a) => (
                          <span
                            key={a.agent.id}
                            title={`${a.agent.name}: ${a.agent.status}`}
                            className="size-2 rounded-full"
                            style={{
                              background: AGENT_STATUS[a.agent.status].color,
                            }}
                          />
                        ))}
                      </div>
                    }
                  />
                  <KpiTile
                    label="In progress"
                    value={d.counts.in_progress}
                    context={`${d.counts.in_review} in review`}
                    visual={
                      <WorkflowBar
                        counts={{ ...d.counts, done: 0, todo: 0, backlog: 0 }}
                      />
                    }
                  />
                  <KpiTile
                    label="Awaiting review"
                    value={d.counts.in_review}
                    context={
                      <span
                        className={
                          d.review_over_day ? "text-pill-human-fg" : undefined
                        }
                      >
                        {d.review_over_day} older than a day
                      </span>
                    }
                    visual={
                      <Link
                        href="/tickets?status=in_review&view=all"
                        className="text-xs text-fg-secondary hover:text-accent"
                      >
                        Review tickets →
                      </Link>
                    }
                  />
                  <KpiTile
                    label={`Completed · ${period}`}
                    value={d.stats.performance.completed}
                    context={`${seconds(d.stats.performance.median_cycle_seconds)} median cycle`}
                    visual={
                      <div
                        role="img"
                        aria-label={`Daily completions (UTC): ${d.stats.daily.map((day) => `${day.date}: ${day.count}`).join(", ")}`}
                        className="flex h-7 items-end gap-1"
                      >
                        {d.stats.daily.map((day) => (
                          <span
                            key={day.date}
                            title={`${day.date}: ${day.count}`}
                            className="min-w-0 flex-1 rounded-t-sm bg-accent/70 last:bg-accent"
                            style={{
                              height: `${Math.max(8, (day.count / max) * 100)}%`,
                              opacity: day.count ? 1 : 0.2,
                            }}
                          />
                        ))}
                      </div>
                    }
                  />
                </div>
                <div className="grid min-w-0 gap-6 lg:grid-cols-[minmax(0,2fr)_minmax(0,1fr)]">
                  <div className="min-w-0 space-y-6">
                    <Card
                      title="Needs your attention"
                      action={
                        <Link
                          href="/inbox"
                          className="text-xs text-fg-secondary hover:text-accent"
                        >
                          View inbox →
                        </Link>
                      }
                      bodyClassName="p-0"
                    >
                      {d.attention.length ? (
                        d.attention.map((item) => (
                          <AttentionRow key={item.id} item={item} />
                        ))
                      ) : (
                        <EmptyState icon="inbox" title="You’re all caught up">
                          Requests and reviews will appear here when work needs
                          you.
                        </EmptyState>
                      )}
                    </Card>
                    <Card title="Projects" bodyClassName="p-0">
                      {d.projects.length ? (
                        <div className="overflow-x-auto">
                          <table className="w-full min-w-[570px] text-left text-xs">
                            <thead className="text-fg-tertiary">
                              <tr>
                                {[
                                  "Project",
                                  "Progress",
                                  "Open",
                                  "Agents",
                                  "Updated",
                                ].map((t) => (
                                  <th key={t} className="px-4 py-3 font-medium">
                                    {t}
                                  </th>
                                ))}
                              </tr>
                            </thead>
                            <tbody>
                              {d.projects.map((p) => (
                                <tr
                                  key={p.project.id}
                                  className="border-t border-border-subtle hover:bg-row-hover"
                                >
                                  <td className="max-w-[200px] px-4 py-3">
                                    <Link
                                      href={`/tickets?project=${p.project.key}`}
                                      className="flex items-center gap-2 text-[13px] font-medium"
                                    >
                                      <span
                                        className="size-2.5 shrink-0 rounded-sm"
                                        style={{ background: p.project.color }}
                                      />
                                      <span className="truncate">
                                        {p.project.name}
                                      </span>
                                    </Link>
                                  </td>
                                  <td className="min-w-28 px-4 py-3">
                                    <div className="mb-1.5 font-mono">
                                      {p.counts.total
                                        ? Math.round(
                                            (p.counts.done / p.counts.total) *
                                              100,
                                          )
                                        : 0}
                                      %
                                    </div>
                                    <ProgressBar
                                      value={
                                        p.counts.total
                                          ? p.counts.done / p.counts.total
                                          : 0
                                      }
                                      color={p.project.color}
                                      label={`${p.project.name} completed`}
                                    />
                                  </td>
                                  <td className="px-4 py-3 font-mono">
                                    {p.counts.total - p.counts.done}
                                  </td>
                                  <td className="px-4 py-3">
                                    <AgentStack agents={p.agents} />
                                  </td>
                                  <td className="whitespace-nowrap px-4 py-3 text-fg-tertiary">
                                    {ago(p.updated_at)}
                                  </td>
                                </tr>
                              ))}
                            </tbody>
                          </table>
                        </div>
                      ) : (
                        <EmptyState icon="tickets" title="No projects yet">
                          Projects and their progress will appear here.
                        </EmptyState>
                      )}
                    </Card>
                  </div>
                  <div className="min-w-0 space-y-6">
                    <Card
                      title="Live agents"
                      action={
                        <Link
                          href="/agents"
                          className="text-xs text-fg-secondary hover:text-accent"
                        >
                          All agents →
                        </Link>
                      }
                    >
                      {live.length ? (
                        <div className="space-y-4">
                          {live.map((a) => (
                            <div key={a.agent.id} className="space-y-2">
                              <div className="flex flex-wrap items-center gap-2">
                                <AgentAvatar
                                  name={a.agent.name}
                                  colorIndex={a.agent.color_index}
                                />
                                <Link
                                  href={`/agents/${a.agent.handle}`}
                                  className="flex-1 font-medium hover:text-accent"
                                >
                                  {a.agent.name}
                                </Link>
                                <AgentStatus status={a.agent.status} />
                              </div>
                              {a.planning_runs.map(p => <PlanningAssignment key={p.id} work={p} />)}
                              {a.assignments
                                .filter((w) => w.state !== "queued")
                                .slice(0, 2)
                                .map((w) => (
                                  <Assignment key={w.run_id} work={w} compact />
                                ))}
                            </div>
                          ))}
                        </div>
                      ) : (
                        <p className="py-2 text-sm text-fg-tertiary">
                          No agents are running. Assign and dispatch a ticket to
                          start work.
                        </p>
                      )}
                    </Card>
                    <Card title="Ticket flow">
                      <div className="space-y-3">
                        <WorkflowBar counts={d.counts} />
                        <div className="grid grid-cols-2 gap-2 text-xs text-fg-secondary">
                          {(
                            [
                              ["Done", d.counts.done, "done"],
                              ["In review", d.counts.in_review, "review"],
                              ["In progress", d.counts.in_progress, "progress"],
                              ["Todo", d.counts.todo, "todo"],
                              ["Backlog", d.counts.backlog, "backlog"],
                            ] as const
                          ).map(([name, n, color]) => (
                            <div
                              key={name}
                              className="flex items-center gap-1.5"
                            >
                              <span
                                className="size-1.5 rounded-full"
                                style={{ background: `var(--state-${color})` }}
                              />
                              {name}
                              <span className="ml-auto font-mono text-fg-tertiary">
                                {n}
                              </span>
                            </div>
                          ))}
                        </div>
                      </div>
                    </Card>
                    <Card title="Runner">
                      <RunnerCapacity runner={d.runner} />
                    </Card>
                    <Card
                      title="Recent artifacts"
                      action={
                        <Link
                          href="/artifacts"
                          className="text-xs text-fg-secondary hover:text-accent"
                        >
                          All →
                        </Link>
                      }
                    >
                      <div className="space-y-3">
                        {d.recent_artifacts.length ? (
                          d.recent_artifacts.map((a) => (
                            <Link
                              key={a.id}
                              href={a.href}
                              className="flex items-start gap-2 rounded-sm hover:text-accent"
                            >
                              <DocTypeTag type={a.type} />
                              <span className="min-w-0">
                                <span className="line-clamp-2 text-[13px]">
                                  {a.title}
                                </span>
                                <span className="mt-1 block text-xs text-fg-tertiary">
                                  {ago(a.updated_at)}
                                </span>
                              </span>
                            </Link>
                          ))
                        ) : (
                          <p className="text-sm text-fg-tertiary">
                            No artifacts yet.
                          </p>
                        )}
                      </div>
                    </Card>
                  </div>
                </div>
                <p
                  className="text-xs text-fg-tertiary"
                  title={absoluteTime(d.stats.generated_at)}
                >
                  Performance refreshed {ago(d.stats.generated_at)}. Cycles:
                  first In Progress to latest Done, or creation to Done when no
                  start was recorded. Daily bars use UTC.
                </p>
              </>
            )
          )}
        </div>
      </Content>
    </>
  );
}

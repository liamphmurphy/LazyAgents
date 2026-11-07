"use client";

import Link from "next/link";
import { useState, type ReactNode } from "react";
import type { Schemas } from "@/lib/api/client";
import { useBootstrap, useDashboard, useInbox } from "@/lib/api/hooks";
import { useBlockers, useDashboardBoards, useDashboardChats, useDashboardLiveRuns } from "@/lib/api/dashboard-hooks";
import { blockedTickets, currentStep, epicsAtRisk, liveRuns, setupSteps, throughput, topInbox } from "@/lib/dashboard";
import { KIND } from "@/lib/inbox";
import { duration, runCost, spendCost } from "@/lib/format";
import { cx } from "@/lib/cx";
import { Button, ButtonLink } from "@/components/ui/button";
import { Card, Skeleton } from "@/components/ui/primitives";
import { Content, TopBar } from "@/components/shell/topbar";
import { ProgressBar } from "@/components/domain/data";
import { AgentAvatar } from "@/components/domain/avatars";
import { Flag } from "@/components/domain/labels";
import { DispatchDialog } from "@/components/domain/dispatch-dialog";

const PREVIEW = 5;

type PanelQuery = { isPending: boolean; isError: boolean; error: Error | null; refetch: () => unknown };

/**
 * One dashboard panel: a header with its count and a link to the full view,
 * then loading, error (with retry), empty or content. A failed background
 * refresh keeps the last data on screen under the error.
 */
function Panel({ title, count, href, link, q, hasData, empty, children }: {
  title: string; count?: number | string; href: string; link: string;
  q: PanelQuery; hasData: boolean; empty: string | false; children: ReactNode;
}) {
  return (
    <div role="region" aria-label={title}>
      <Card
        title={<>{title}{count !== undefined && hasData && <span className="ml-1.5 font-mono text-fg-tertiary">· {count}</span>}</>}
        action={<Link href={href} className="text-xs text-fg-secondary hover:text-accent">{link} →</Link>}
        bodyClassName="p-0"
      >
        {q.isError && (
          <div role="alert" className="flex items-center justify-between gap-3 border-b border-border-subtle px-[18px] py-2.5 text-xs text-pill-blocked-fg">
            <span>{hasData ? "Couldn’t refresh" : "Couldn’t load"}: {q.error?.message ?? "request failed"}</span>
            <Button onClick={() => void q.refetch()}>Retry</Button>
          </div>
        )}
        {!hasData ? (
          q.isError ? null : (
            <div aria-busy="true" aria-label={`Loading ${title.toLowerCase()}`} className="space-y-3 px-[18px] py-4">
              <Skeleton className="h-3 w-2/3" />
              <Skeleton className="h-3 w-1/2" />
            </div>
          )
        ) : empty !== false ? (
          <p className="px-[18px] py-4 text-sm text-fg-tertiary">{empty}</p>
        ) : (
          children
        )}
      </Card>
    </div>
  );
}

function Row({ children }: { children: ReactNode }) {
  return <div className="flex flex-wrap items-center gap-3 border-b border-border-subtle px-[18px] py-3 last:border-0">{children}</div>;
}

function NeedsYouRow({ item }: { item: Schemas["InboxItem"] }) {
  const [dispatch, setDispatch] = useState(false);
  const kind = KIND[item.kind] ?? { label: item.kind, tone: "human" as const };
  const action = { approval: "Review request", question: "Answer", mockup: "Review mockup", planning: "Review plan", suggestion: "Review edit" }[item.kind as string] ?? "Open";
  return (
    <Row>
      <div className="min-w-0 flex-[1_1_230px]">
        <div className="mb-1 flex items-center gap-2">
          <span className="font-mono text-xs text-fg-tertiary">{item.ticket_key || item.project || "Wiki"}</span>
          <Flag tone={kind.tone === "blocked" ? "blocked" : kind.tone === "info" ? "review" : "human"}>{kind.label}</Flag>
        </div>
        <Link href={item.href} className="line-clamp-1 text-[13px] font-medium hover:text-accent">{item.title}</Link>
      </div>
      {item.kind === "dispatch" ? (
        <Button variant="approve" onClick={() => setDispatch(true)}>Review &amp; dispatch</Button>
      ) : (
        <ButtonLink href={item.href} variant={item.kind === "approval" || item.kind === "mockup" ? "approve" : "outline"}>{action}</ButtonLink>
      )}
      {dispatch && <DispatchDialog keys={[item.ticket_key]} open={dispatch} onOpenChange={setDispatch} />}
    </Row>
  );
}

function RunRow({ run }: { run: Schemas["Run"] }) {
  const step = currentStep(run.plan);
  return (
    <Row>
      <AgentAvatar name={run.agent.name} colorIndex={run.agent.color_index} />
      <div className="min-w-0 flex-1">
        <Link href={`/runs/${run.id}`} className="line-clamp-1 text-[13px] font-medium hover:text-accent">
          {run.agent.name} · {run.ticket_key || run.work_title || (run.purpose === "chat" ? "Chat" : "Plan")}
        </Link>
        <p className="truncate text-xs text-fg-tertiary">
          {step ? `${step.text} · step ${step.position} of ${step.total}` : run.state}
        </p>
      </div>
      <p className="whitespace-nowrap text-right text-xs text-fg-secondary">
        {duration(run.started_at)} elapsed · {runCost(run.cost_usd, run.cost_source)} so far
      </p>
    </Row>
  );
}

const CHAT_STATE: Record<string, string> = { idle: "waiting for your next turn", running: "agent responding", failed: "last turn failed" };

const HEALTH: Record<string, { label: string; tone: "human" | "blocked" }> = {
  at_risk: { label: "At risk", tone: "human" },
  off_track: { label: "Off track", tone: "blocked" },
};

export function DashboardScreen() {
  const boot = useBootstrap();
  const dash = useDashboard("30d");
  const inbox = useInbox();
  const runsQ = useDashboardLiveRuns();
  const chatsQ = useDashboardChats();
  const active = boot.data?.projects.filter((p) => !p.archived);
  const boards = useDashboardBoards(active?.map((p) => p.key));

  const needs = inbox.data ? topInbox(inbox.data, PREVIEW) : undefined;
  const live = runsQ.data ? liveRuns(runsQ.data.runs) : undefined;
  const liveCount = live && `${live.length}${runsQ.data?.complete ? "" : "+"}`;
  const chats = chatsQ.data?.chats;
  const chatCount = chats && `${chats.length}${chatsQ.data?.complete ? "" : "+"}`;
  const blocked = boards.data ? blockedTickets(boards.data) : undefined;
  const blockers = useBlockers(blocked?.slice(0, PREVIEW).map((t) => t.key) ?? []);
  const blockedProjects = new Set(blocked?.map((t) => t.project_key));
  const blockedHref = blockedProjects.size === 1 ? `/board?project=${encodeURIComponent([...blockedProjects][0])}&blocked=true` : "/board?blocked=true";
  const risky = boot.data ? epicsAtRisk(boot.data.epics) : undefined;
  const projectOf = (id: string) => boot.data?.projects.find((p) => p.id === id);
  // The board query waits for the project list, so its state follows bootstrap's first.
  const blockedQ: PanelQuery = boot.isError && !boot.data
    ? { isPending: false, isError: true, error: boot.error, refetch: () => Promise.all([boot.refetch(), boards.refetch()]) }
    : { isPending: boards.isPending, isError: boards.isError, error: boards.error, refetch: () => boards.refetch() };

  const d = dash.data;
  const days = d ? throughput(d.stats.daily, new Date(d.stats.generated_at)) : undefined;
  const done = days?.reduce((n, day) => n + day.count, 0) ?? 0;
  const max = Math.max(1, ...(days?.map((day) => day.count) ?? []));
  const steps = boot.data && d ? setupSteps({ projects: active?.length ?? 0, agents: boot.data.agents.length, runnerOnline: d.runner.online }) : undefined;
  const nextStep = steps?.find((s) => !s.done);

  return (
    <>
      <TopBar
        crumbs={[{ label: "Dashboard" }]}
        actions={<ButtonLink href="/tickets/new" variant="primary" icon="plus" hint="C">New ticket</ButtonLink>}
      />
      <Content>
        <div className="flex flex-col gap-6">
          <div className="space-y-1">
            <h1 className="type-display">Your team at a glance</h1>
            <p className="text-sm text-fg-secondary">
              {[
                needs && `${needs.open} ${needs.open === 1 ? "item needs" : "items need"} you`,
                liveCount !== undefined && `${liveCount} live ${live?.length === 1 ? "run" : "runs"}`,
                chatCount !== undefined && `${chatCount} active ${chats?.length === 1 ? "chat" : "chats"}`,
              ].filter(Boolean).join(" · ") || "What the team is doing and what needs you."}
            </p>
          </div>

          {steps && nextStep && (
            <section aria-label="Finish setup" className="flex flex-wrap items-center gap-3 rounded-xl border border-border bg-surface px-[18px] py-3 text-sm">
              <span className="font-semibold">Finish setup</span>
              <span className="text-fg-secondary">
                {steps.filter((s) => s.done).length} / {steps.length} complete — {nextStep.label}
              </span>
              <ButtonLink href={nextStep.href} variant="primary" className="ml-auto">Continue setup →</ButtonLink>
            </section>
          )}

          <div className="grid min-w-0 gap-6 lg:grid-cols-[minmax(0,2fr)_minmax(0,1fr)]">
            <div className="min-w-0 space-y-6">
              <Panel title="Needs you" count={needs?.open} href="/inbox" link="View Inbox" q={inbox} hasData={!!needs}
                empty={!!needs && !needs.items.length && "You’re all caught up. Approvals, questions and reviews appear here."}>
                {needs?.items.map((item) => <NeedsYouRow key={item.id} item={item} />)}
              </Panel>

              <Panel title="Live runs" count={liveCount} href="/runs" link="All Runs" q={runsQ} hasData={!!live}
                empty={!!live && !live.length && "No agents are working right now. Dispatch a ticket to start one."}>
                {live?.slice(0, PREVIEW).map((run) => <RunRow key={run.id} run={run} />)}
              </Panel>

              <Panel title="Blocked tickets" count={blocked?.length} href={blockedHref} link="Blocked Board" q={blockedQ} hasData={!!blocked}
                empty={!!blocked && !blocked.length && "Nothing is waiting on another ticket."}>
                {blocked?.slice(0, PREVIEW).map((t) => {
                  const by = blockers.data?.[t.key];
                  return (
                    <Row key={t.key}>
                      <div className="min-w-0 flex-1">
                        <Link href={`/tickets/${encodeURIComponent(t.key)}`} className="line-clamp-1 text-[13px] font-medium hover:text-accent">
                          <span className="mr-2 font-mono text-xs text-fg-tertiary">{t.key}</span>{t.title}
                        </Link>
                        <p className="truncate text-xs text-fg-tertiary">
                          {by?.length
                            ? <>Blocked by <Link href={`/tickets/${encodeURIComponent(by[0].key)}`} className="font-mono hover:text-accent">{by[0].key}</Link> · {by[0].title}{by.length > 1 && ` and ${by.length - 1} more`}</>
                            : `Blocked by ${t.unmet_dependencies} open ${t.unmet_dependencies === 1 ? "ticket" : "tickets"}`}
                        </p>
                      </div>
                    </Row>
                  );
                })}
              </Panel>
            </div>

            <div className="min-w-0 space-y-6">
              <Panel title="Chats active" count={chatCount} href="/chat" link="All Chats" q={chatsQ} hasData={!!chats}
                empty={!!chats && !chats.length && "No open chats. Ask an agent about a project from Chat."}>
                {chats?.slice(0, PREVIEW).map((c) => (
                  <Row key={c.id}>
                    <div className="min-w-0 flex-1">
                      <Link href={`/chat/${c.id}`} className="line-clamp-1 text-[13px] font-medium hover:text-accent">{c.title || "Untitled chat"}</Link>
                      <p className={cx("truncate text-xs", c.state === "failed" ? "text-pill-blocked-fg" : "text-fg-tertiary")}>
                        <span className="font-mono">{c.project}</span> · {CHAT_STATE[c.state] ?? c.state}
                      </p>
                    </div>
                  </Row>
                ))}
              </Panel>

              <Panel title="Epics at risk" count={risky?.length} href="/epics" link="All Epics" q={boot} hasData={!!risky}
                empty={!!risky && !risky.length && "Every open epic is on track."}>
                {risky?.slice(0, PREVIEW).map((e) => {
                  const project = projectOf(e.project_id);
                  return (
                    <div key={e.id} className="space-y-2 border-b border-border-subtle px-[18px] py-3 last:border-0">
                      <div className="flex flex-wrap items-center gap-2">
                        <span className="min-w-0 flex-1 truncate text-[13px] font-medium">{e.name}</span>
                        <Flag tone={HEALTH[e.health].tone}>{HEALTH[e.health].label}</Flag>
                        <Link href={`/epics/${e.id}`} className="text-xs text-fg-secondary hover:text-accent">Open epic →</Link>
                      </div>
                      <ProgressBar value={e.points_total ? e.points_done / e.points_total : 0} color={project?.color} label={`${e.name} points done`} />
                      <p className="text-xs text-fg-tertiary">{e.points_done} / {e.points_total} points done · {e.tickets_open} {e.tickets_open === 1 ? "ticket" : "tickets"} open</p>
                    </div>
                  );
                })}
              </Panel>
            </div>
          </div>

          <div className="grid min-w-0 gap-6 lg:grid-cols-[minmax(0,1fr)_minmax(0,2fr)]">
            <Panel title="Spend this week" href="/costs" link="View Costs" q={dash} hasData={!!d?.weekly_spend} empty={false}>
              {d?.weekly_spend && (
                <div className="space-y-1 px-[18px] py-4">
                  <p className="font-mono text-2xl font-semibold">{spendCost(d.weekly_spend.cost_usd, d.weekly_spend.unpriced_runs)}</p>
                  <p className="text-xs text-fg-tertiary">Since Monday · UTC</p>
                  <p className="text-xs text-fg-secondary">
                    Across {d.weekly_spend.runs} {d.weekly_spend.runs === 1 ? "run" : "runs"}
                    {d.weekly_spend.unpriced_runs > 0 && ` · ${d.weekly_spend.unpriced_runs} not fully priced`}
                  </p>
                </div>
              )}
            </Panel>

            <Panel title="Throughput · last 14 days" href="/tickets?status=done&view=all" link="Done tickets" q={dash} hasData={!!days}
              empty={!!days && done === 0 && "No tickets done in the last 14 days."}>
              {days && (
                <div className="space-y-2 px-[18px] py-4">
                  <div
                    role="img"
                    aria-label={`Tickets done per day (UTC): ${days.map((day) => `${day.date}: ${day.count}`).join(", ")}`}
                    className="flex h-24 items-end gap-1"
                  >
                    {days.map((day) => (
                      <span key={day.date} title={`${day.date}: ${day.count}`} className="min-w-0 flex-1 rounded-t-sm bg-accent/70 last:bg-accent"
                        style={{ height: `${Math.max(4, (day.count / max) * 100)}%`, opacity: day.count ? 1 : 0.25 }} />
                    ))}
                  </div>
                  <div className="flex justify-between font-mono text-[11px] text-fg-tertiary">
                    <span>{days[0].date}</span>
                    <span>{days[days.length - 1].date}</span>
                  </div>
                  <p className="text-xs text-fg-secondary">{done} {done === 1 ? "ticket" : "tickets"} done · canceled excluded · UTC</p>
                </div>
              )}
            </Panel>
          </div>
        </div>
      </Content>
    </>
  );
}

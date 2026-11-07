"use client";

import Link from "next/link";
import { useState } from "react";
import { useMutation, useQueries, useQueryClient } from "@tanstack/react-query";
import { api, unwrap, type Schemas } from "@/lib/api/client";
import { hasLiveRun, keys, useBootstrap, useBulkUpdate, useDispatch, useEpicOverviews, useTickets, useUpdateEpic } from "@/lib/api/hooks";
import { useBoard } from "@/lib/api/board-hooks";
import { dueDate } from "@/lib/format";
import { burnUp, isOverdue, ymd, type BurnUp } from "@/lib/epics";
import { Button, ButtonLink } from "@/components/ui/button";
import { Banner, Card, EmptyState, Skeleton } from "@/components/ui/primitives";
import { Toggle } from "@/components/ui/controls";
import { Content, TopBar, type Crumb } from "@/components/shell/topbar";
import { AgentStack, HealthPill, type EpicOverview } from "@/components/domain/overview";
import { HumanAvatar } from "@/components/domain/avatars";
import { DocTypeTag, Pill } from "@/components/domain/labels";
import { StatusGlyph } from "@/components/domain/glyphs";
import { STATUS_NAME, STATUS_ORDER } from "@/components/domain/meta";
import { EpicCompletion } from "@/components/domain/epic-completion";
import { DependencyMap } from "@/components/domain/dependency-map";

type TicketSummary = Schemas["TicketSummary"];

export function EpicDetailScreen({ epicId }: { epicId: string }) {
  const query = useEpicOverviews();
  const e = query.data?.find((x) => x.epic.id === epicId);
  const crumbs: Crumb[] = e
    ? [
        { label: e.project.name, swatch: e.project.color, href: `/tickets?project=${encodeURIComponent(e.project.key)}` },
        { label: "Epics", href: `/epics?project=${encodeURIComponent(e.project.key)}` },
        { label: e.epic.name },
      ]
    : [{ label: "Epics", href: "/epics" }];
  return (
    <>
      <TopBar
        crumbs={crumbs}
        actions={e && (
          <ButtonLink variant="outline" icon="board" href={boardHref(e)}>View on board</ButtonLink>
        )}
      />
      <Content>
        {query.isError ? (
          <Banner tone="blocked" title="Couldn’t load the epic" actions={<Button onClick={() => query.refetch()}>Retry</Button>}>
            {query.error.message}
          </Banner>
        ) : query.isPending ? (
          <div aria-busy="true" className="space-y-4">
            <Skeleton className="h-32 rounded-xl" />
            <Skeleton className="h-72 rounded-xl" />
          </div>
        ) : !e ? (
          <EmptyState icon="epics" title="Epic not found" actions={<ButtonLink href="/epics">All epics</ButtonLink>}>
            It may have been deleted, or the link is wrong.
          </EmptyState>
        ) : (
          <EpicDetail epic={e} />
        )}
      </Content>
    </>
  );
}

const boardHref = (e: EpicOverview) => `/board?project=${encodeURIComponent(e.project.key)}&epic=${encodeURIComponent(e.epic.id)}`;

function EpicDetail({ epic: e }: { epic: EpicOverview }) {
  const [tab, setTab] = useState("overview");
  const today = ymd(new Date());
  const overdue = isOverdue(e, today);
  const tickets = useTickets({ epic: e.epic.id, view: "all" });
  return (
    <div className="space-y-6">
      <header className="space-y-3">
        <div className="flex flex-wrap items-center gap-2">
          <h1 className="type-display">{e.epic.name}</h1>
          <Pill tone={e.epic.status === "done" ? "done" : "neutral"}>{e.epic.status === "done" ? "Done" : "Open"}</Pill>
          <HealthPill health={e.epic.health} />
        </div>
        <p className="flex flex-wrap gap-x-2 text-xs text-fg-tertiary">
          <span>{e.project.name}</span>·
          <span className={overdue ? "text-pill-blocked-fg" : undefined}>
            {e.target_date ? `${overdue ? "Overdue ·" : "Target"} ${dueDate(e.target_date)}` : "No target date"}
          </span>·
          <span>{e.owner ? `Owner ${e.owner.name}` : "No owner"}</span>
        </p>
        <p className="max-w-3xl text-sm text-fg-secondary">{e.objective || "No objective has been written yet."}</p>
        <div className="flex flex-wrap gap-2">
          <ButtonLink variant="outline" href={boardHref(e)}>Board filtered to this epic</ButtonLink>
          <ButtonLink variant="outline" href={`/tickets?view=all&epic=${e.epic.id}&project=${encodeURIComponent(e.project.key)}`}>Ticket list</ButtonLink>
          <ButtonLink variant="outline" href={`/plans?epic=${e.epic.id}&project=${encodeURIComponent(e.project.key)}${e.source ? `&artifact=${e.source.id}` : ""}`}>Plan with an agent</ButtonLink>
        </div>
      </header>

      <nav aria-label="Epic views" className="flex gap-5 border-b border-border pb-3 text-sm">
        <button aria-current={tab === "overview" ? "page" : undefined} className={tab === "overview" ? "font-semibold text-accent" : ""} onClick={() => setTab("overview")}>Overview</button>
        <button aria-current={tab === "map" ? "page" : undefined} className={tab === "map" ? "font-semibold text-accent" : ""} onClick={() => setTab("map")}>Map</button>
      </nav>
      {tab === "map" ? <DependencyMap key={e.epic.id} project={e.project.key} epic={e.epic.id} /> : <>
      <Card title="Done gate and acceptance criteria" as="section">
        <Completion projectKey={e.project.key} epicId={e.epic.id} />
      </Card>

      <div className="grid gap-6 lg:grid-cols-[minmax(0,2fr)_minmax(0,1fr)]">
        <Card title="Points burn-up">
          {tickets.isError ? (
            <p role="alert" className="text-xs text-pill-blocked-fg">Couldn’t load tickets: {tickets.error.message}</p>
          ) : tickets.isPending ? (
            <Skeleton className="h-48" />
          ) : (
            <BurnUpChart data={burnUp(tickets.data, { start: e.start_date, today })} />
          )}
        </Card>
        <Card title="Details">
          <Details epic={e} />
        </Card>
      </div>

      <Card title="Dispatch" bodyClassName="p-0">
        <EpicDispatch key={e.epic.id} epic={e} />
      </Card>

      <Card
        title="Tickets"
        bodyClassName="p-0"
        action={<span className="font-mono text-xs text-fg-tertiary">{e.epic.tickets_open} open of {e.epic.tickets_total}</span>}
      >
        {tickets.isError ? (
          <p role="alert" className="p-5 text-xs text-pill-blocked-fg">Couldn’t load tickets: {tickets.error.message}</p>
        ) : tickets.isPending ? (
          <Skeleton className="m-5 h-24" />
        ) : (
          <TicketGroups tickets={tickets.data} />
        )}
      </Card>
      </>}
    </div>
  );
}

/** Criteria editor and done gate, from the board's epic summary (structured done_blockers). */
function Completion({ projectKey, epicId }: { projectKey: string; epicId: string }) {
  const board = useBoard(projectKey, { epic: epicId, include_canceled: false });
  return (
    <section aria-label="Epic completion">
      {board.data?.epic ? <EpicCompletion summary={board.data.epic} />
        : board.isError ? <p role="alert" className="text-xs">Couldn&apos;t load acceptance criteria: {board.error.message} <button type="button" className="underline" onClick={() => void board.refetch()}>Retry</button></p>
        : <p role="status" className="text-xs text-fg-tertiary">Loading acceptance criteria…</p>}
    </section>
  );
}

const W = 720, H = 180, PAD = { l: 36, r: 12, t: 12, b: 26 };

export function BurnUpChart({ data }: { data: BurnUp }) {
  const { scope, done, points } = data;
  if (!scope) {
    return <p className="text-sm text-fg-tertiary">No estimates yet. Estimate the epic’s tickets to see a burn-up.</p>;
  }
  const day = (d: string) => new Date(`${d}T00:00:00`).getTime();
  const t0 = day(points[0].date), t1 = day(points.at(-1)!.date);
  const x = (d: string) => PAD.l + (t1 > t0 ? (day(d) - t0) / (t1 - t0) : 0) * (W - PAD.l - PAD.r);
  const y = (v: number) => PAD.t + (1 - v / scope) * (H - PAD.t - PAD.b);
  // A step line: completions land on their day.
  const path = points.map((p, i) => `${i ? `H${x(p.date).toFixed(1)}V` : `M${x(p.date).toFixed(1)},`}${y(p.done).toFixed(1)}`).join(" ");
  const pct = Math.round((done / scope) * 100);
  return (
    <figure className="space-y-2">
      <svg viewBox={`0 0 ${W} ${H}`} className="h-auto w-full" role="img" aria-label={`Points burn-up: ${done} of ${scope} points done (${pct}%)`}>
        <line x1={PAD.l} x2={W - PAD.r} y1={y(0)} y2={y(0)} stroke="var(--border-subtle)" />
        <line x1={PAD.l} x2={W - PAD.r} y1={y(scope)} y2={y(scope)} stroke="var(--text-tertiary)" strokeDasharray="4 3" />
        <text x={PAD.l - 6} y={y(scope) + 4} textAnchor="end" fontSize="11" fill="var(--text-tertiary)">{scope}</text>
        <text x={PAD.l - 6} y={y(0) + 4} textAnchor="end" fontSize="11" fill="var(--text-tertiary)">0</text>
        <path d={path} fill="none" stroke="var(--accent)" strokeWidth="2" />
        {points.map((p) => <circle key={p.date} cx={x(p.date)} cy={y(p.done)} r="3" fill="var(--accent)" />)}
        <text x={PAD.l} y={H - 8} fontSize="11" fill="var(--text-tertiary)">{dueDate(points[0].date)}</text>
        <text x={W - PAD.r} y={H - 8} textAnchor="end" fontSize="11" fill="var(--text-tertiary)">Today</text>
      </svg>
      <figcaption className="flex flex-wrap justify-between gap-2 text-xs text-fg-tertiary">
        <span><span className="font-mono text-fg">{done} / {scope}</span> pts done · {pct}%</span>
        <span>Dashed line is today’s scope. Done tickets count on the day they were last updated.</span>
      </figcaption>
    </figure>
  );
}

function Details({ epic: e }: { epic: EpicOverview }) {
  const updateEpic = useUpdateEpic(e.epic.id);
  return (
    <dl className="grid grid-cols-[80px_minmax(0,1fr)] items-start gap-x-3 gap-y-3 text-xs">
      <dt className="text-fg-tertiary">Owner</dt>
      <dd>
        {e.owner ? (
          <span className="flex items-center gap-2">
            <HumanAvatar name={e.owner.name} initials={e.owner.initials} color={e.owner.avatar_color} />
            {e.owner.name}
          </span>
        ) : "Unassigned"}
      </dd>
      <dt className="text-fg-tertiary">Schedule</dt>
      <dd>{e.start_date ? dueDate(e.start_date) : "No start"} → {e.target_date ? dueDate(e.target_date) : "No target"}</dd>
      <dt className="text-fg-tertiary">Agents</dt>
      <dd>{e.agents.length ? <AgentStack agents={e.agents} /> : <span className="text-fg-tertiary">None yet</span>}</dd>
      <dt className="text-fg-tertiary">Source</dt>
      <dd>
        {e.source ? (
          <Link href={e.source.href} className="flex items-start gap-2 hover:text-accent">
            <DocTypeTag type={e.source.type} />
            <span>{e.source.title}</span>
          </Link>
        ) : <span className="text-fg-tertiary">No linked PRD</span>}
      </dd>
      <dt className="text-fg-tertiary">Mockup</dt>
      <dd className="space-y-1">
        <Toggle
          checked={e.epic.requires_mockup}
          disabled={updateEpic.isPending}
          onChange={(v) => updateEpic.mutate({ requires_mockup: v })}
          label="Requires mockup"
        />
        <span className="block text-fg-tertiary">Tickets need an approved UI mockup before an agent can finish.</span>
        {updateEpic.isError && <span role="alert" className="block text-pill-blocked-fg">{updateEpic.error.message}</span>}
      </dd>
      <dt className="text-fg-tertiary">Agent hours</dt>
      <dd className="font-mono">{(e.agent_seconds / 3600).toFixed(1)}h</dd>
    </dl>
  );
}

function TicketGroups({ tickets }: { tickets: TicketSummary[] }) {
  const groups = STATUS_ORDER.map((status) => ({ status, items: tickets.filter((t) => t.status === status) })).filter((g) => g.items.length);
  if (!groups.length) return <p className="p-5 text-sm text-fg-tertiary">No tickets in this epic yet.</p>;
  return (
    <div>
      {groups.map((g) => (
        <section key={g.status} aria-label={`${STATUS_NAME[g.status]} tickets`}>
          <h3 className="flex items-center gap-2 border-t border-border-subtle bg-surface-sunken px-4 py-2 text-xs font-medium text-fg-secondary first:border-t-0">
            <StatusGlyph state={g.status} label={false} />
            {STATUS_NAME[g.status]}
            <span className="font-mono text-fg-tertiary">{g.items.length}</span>
            <span className="ml-auto font-mono text-fg-tertiary">{g.items.reduce((n, t) => n + (t.estimate_points ?? 0), 0)} pts</span>
          </h3>
          <ul>
            {g.items.map((t) => (
              <li key={t.id}>
                <Link href={`/tickets/${encodeURIComponent(t.key)}`} className="flex min-h-11 items-center gap-3 border-t border-border-row px-4 py-2 hover:bg-row-hover">
                  <span className="shrink-0 font-mono text-xs text-fg-tertiary">{t.key}</span>
                  <span className="min-w-0 flex-1 truncate text-[13px] font-medium">{t.title}</span>
                  {t.assignee && <span className="hidden text-xs text-fg-tertiary sm:inline">{t.assignee.name}</span>}
                  <span className="w-12 shrink-0 text-right font-mono text-xs text-fg-tertiary">{t.estimate_points != null ? `${t.estimate_points} pt` : "—"}</span>
                </Link>
              </li>
            ))}
          </ul>
        </section>
      ))}
    </div>
  );
}

function EpicDispatch({ epic }: { epic: EpicOverview }) {
  const [agentId, setAgentId] = useState("");
  const bootstrap = useBootstrap();
  const agents = (bootstrap.data?.agents ?? []).filter((a) => a.kind === "main");
  const selectedAgent = agents.find((a) => a.id === agentId) ?? agents[0];
  const bulk = useBulkUpdate();
  const dispatch = useDispatch();
  const qc = useQueryClient();
  const start = useMutation({
    mutationFn: async ({ ticketKeys, unassignedKeys, assignee }: { ticketKeys: string[]; unassignedKeys: string[]; assignee?: string }) => {
      if (unassignedKeys.length) {
        if (!assignee) throw new Error("Choose a main agent for unassigned tickets.");
        await bulk.mutateAsync({ keys: unassignedKeys, patch: { assignee_agent_id: assignee } });
      }
      return dispatch.mutateAsync(ticketKeys);
    },
    onSettled: () => qc.invalidateQueries({ queryKey: ["epics"] }),
  });
  // The overview only contains a sample of tickets. Fetch the whole epic.
  const list = useTickets({ epic: epic.epic.id, view: "all" });
  const remaining = (list.data ?? []).filter((t) => t.status !== "done" && t.status !== "canceled");
  const details = useQueries({ queries: remaining.map((t) => ({
    queryKey: keys.ticket(t.key),
    queryFn: () => unwrap(api.GET("/tickets/{key}", { params: { path: { key: t.key } } })),
  })) });
  const loading = list.isPending || details.some((q) => q.isPending);
  const failed = list.isError || details.some((q) => q.isError);
  const incomplete = !!list.data && list.data.length < epic.counts.total;
  const candidates = details.flatMap((q) => q.data && q.data.status !== "done" && q.data.status !== "canceled" && !hasLiveRun(q.data) ? [q.data] : []);
  const dispatchKeys = candidates.map((t) => t.key);
  const unassignedKeys = candidates.filter((t) => !t.assignee).map((t) => t.key);
  const unassigned = unassignedKeys.length;
  return (
    <div className="space-y-2 px-5 py-3">
      <div className="flex flex-wrap items-center gap-3">
        {unassigned > 0 && (
          <label className="flex items-center gap-2 text-xs text-fg-secondary">
            Agent for unassigned tickets
            <select aria-label="Agent for unassigned tickets" value={selectedAgent?.id ?? ""}
              disabled={start.isPending || bootstrap.isPending}
              onChange={(e) => { setAgentId(e.target.value); start.reset(); }}
              className="h-[30px] rounded-md border border-border-strong bg-surface px-2 text-fg">
              {!agents.length && <option value="">No main agents available</option>}
              {agents.map((a) => <option key={a.id} value={a.id}>{a.name}</option>)}
            </select>
          </label>
        )}
        <Button variant="approve" disabled={epic.epic.status === "done" || loading || failed || incomplete || !dispatchKeys.length || start.isPending || (unassigned > 0 && !selectedAgent)}
          onClick={() => start.mutate({ ticketKeys: dispatchKeys, unassignedKeys, assignee: selectedAgent?.id })}>
          {start.isPending ? "Dispatching…" : loading ? "Loading tickets…" : `${unassigned ? "Assign & dispatch epic" : "Dispatch epic"} (${dispatchKeys.length})`}
        </Button>
        <p className="text-xs text-fg-secondary">
          Queue all remaining tickets. Existing agent assignments are kept. Runs start automatically as dependencies finish and runner slots become available.
        </p>
      </div>
      {unassigned > 0 && <p className="text-xs text-fg-secondary">{unassigned} tickets will be assigned to {selectedAgent?.name ?? "the agent you choose"} before dispatch.</p>}
      {unassigned > 0 && !bootstrap.isPending && !agents.length && <p role="alert" className="text-xs text-pill-blocked-fg">{bootstrap.isError ? "Couldn’t load agents. Refresh to try again." : "Create a main agent to assign and dispatch these tickets."}</p>}
      {start.isError && <p role="alert" className="text-xs text-pill-blocked-fg">Couldn’t dispatch the epic: {start.error.message}{bulk.isSuccess && " Agent assignments were saved; retry to dispatch."}</p>}
      {failed && <p role="alert" className="text-xs text-pill-blocked-fg">Couldn’t load all ticket details. Refresh to try again.</p>}
      {incomplete && <p role="alert" className="text-xs text-pill-blocked-fg">The ticket list is incomplete. Open the ticket list to dispatch in smaller batches.</p>}
      {start.isSuccess && <p role="status" className="text-xs text-fg-secondary">{start.data} epic tickets queued. Follow their progress below.</p>}
    </div>
  );
}

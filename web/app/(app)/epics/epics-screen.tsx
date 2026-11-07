"use client";

import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { useState } from "react";
import { useMutation, useQueries, useQueryClient } from "@tanstack/react-query";
import { api, unwrap } from "@/lib/api/client";
import { hasLiveRun, keys, useBootstrap, useBulkUpdate, useDispatch, useEpicOverviews, useTickets, useUpdateEpic } from "@/lib/api/hooks";
import { useBoard } from "@/lib/api/board-hooks";
import { dueDate } from "@/lib/format";
import { Button, ButtonLink } from "@/components/ui/button";
import { Banner, Card, EmptyState, Skeleton } from "@/components/ui/primitives";
import { Toggle } from "@/components/ui/controls";
import { Icon } from "@/components/ui/icon";
import { Content, TopBar } from "@/components/shell/topbar";
import {
  AgentStack,
  HealthPill,
  WorkflowBar,
  WorkRow,
  type EpicOverview,
} from "@/components/domain/overview";
import { HumanAvatar } from "@/components/domain/avatars";
import { DocTypeTag } from "@/components/domain/labels";
import { EpicCompletion } from "@/components/domain/epic-completion";
import { cx } from "@/lib/cx";

function Timeline({
  epics,
  onSelect,
}: {
  epics: EpicOverview[];
  onSelect: (id: string) => void;
}) {
  const now = new Date();
  const [quarter, setQuarter] = useState(
    () => now.getFullYear() * 4 + Math.floor(now.getMonth() / 3),
  );
  const year = Math.floor(quarter / 4),
    month = (quarter % 4) * 3;
  const start = new Date(year, month, 1),
    end = new Date(year, month + 3, 1),
    span = end.getTime() - start.getTime();
  const at = (iso: string) => new Date(`${iso}T00:00:00`).getTime();
  const visible = epics.filter(
    (e) =>
      e.start_date &&
      e.target_date &&
      at(e.start_date) < end.getTime() &&
      at(e.target_date) + 86400000 > start.getTime(),
  );
  const today = ((now.getTime() - start.getTime()) / span) * 100;
  return (
    <Card
      title={`Q${(quarter % 4) + 1} ${year} timeline`}
      action={
        <div className="flex gap-1">
          <Button
            variant="ghost"
            aria-label="Previous quarter"
            onClick={() => setQuarter(quarter - 1)}
          >
            ←
          </Button>
          <Button
            variant="ghost"
            onClick={() =>
              setQuarter(now.getFullYear() * 4 + Math.floor(now.getMonth() / 3))
            }
          >
            Today
          </Button>
          <Button
            variant="ghost"
            aria-label="Next quarter"
            onClick={() => setQuarter(quarter + 1)}
          >
            →
          </Button>
        </div>
      }
      bodyClassName="p-0"
    >
      {visible.length ? (
        <div className="overflow-x-auto">
          <div className="min-w-[600px] px-4 pb-4">
            <div className="grid grid-cols-[180px_1fr] border-b border-border-subtle">
              <span className="py-3 text-xs text-fg-tertiary">Epic</span>
              <div className="grid grid-cols-3">
                {[0, 1, 2].map((i) => (
                  <span
                    key={i}
                    className="border-l border-border-subtle px-3 py-3 text-xs text-fg-tertiary"
                  >
                    {new Date(year, month + i, 1).toLocaleDateString(
                      undefined,
                      { month: "long" },
                    )}
                  </span>
                ))}
              </div>
            </div>
            {visible.map((e) => {
              const left = Math.max(
                0,
                ((at(e.start_date!) - start.getTime()) / span) * 100,
              );
              const right = Math.min(
                100,
                ((at(e.target_date!) + 86400000 - start.getTime()) / span) *
                  100,
              );
              return (
                <div
                  key={e.epic.id}
                  className="grid min-h-[46px] grid-cols-[180px_1fr] border-b border-border-subtle last:border-0"
                >
                  <button
                    onClick={() => onSelect(e.epic.id)}
                    className="truncate pr-3 text-left text-[13px] hover:text-accent"
                  >
                    {e.epic.name}
                  </button>
                  <div className="relative my-1.5 grid grid-cols-3">
                    {[0, 1, 2].map((i) => (
                      <span key={i} className="border-l border-border-subtle" />
                    ))}
                    {today >= 0 && today <= 100 && (
                      <span
                        aria-label="Today"
                        className="pointer-events-none absolute inset-y-[-6px] z-10 w-px bg-accent"
                        style={{ left: `${today}%` }}
                      />
                    )}
                    <button
                      onClick={() => onSelect(e.epic.id)}
                      title={`${e.epic.name}: ${dueDate(e.start_date!)} – ${dueDate(e.target_date!)}`}
                      aria-label={`${e.epic.name}, ${e.start_date} to ${e.target_date}`}
                      className="absolute inset-y-1 overflow-hidden rounded-md border px-2 text-left text-[11px]"
                      style={{
                        left: `${left}%`,
                        width: `${Math.max(0.5, right - left)}%`,
                        borderColor: `color-mix(in srgb, ${e.project.color} 40%, transparent)`,
                        background: `color-mix(in srgb, ${e.project.color} 15%, var(--surface))`,
                      }}
                    >
                      <span className="whitespace-nowrap">{e.epic.name}</span>
                    </button>
                  </div>
                </div>
              );
            })}
          </div>
        </div>
      ) : (
        <p className="p-5 text-sm text-fg-tertiary">
          No epics are scheduled in this quarter.
        </p>
      )}
      {epics.some((e) => !e.start_date || !e.target_date) && (
        <p className="border-t border-border-subtle px-5 py-3 text-xs text-fg-tertiary">
          {epics.filter((e) => !e.start_date || !e.target_date).length} epics
          have no complete schedule.
        </p>
      )}
    </Card>
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
  const remaining = (list.data ?? []).filter((t) => t.status !== "done");
  const details = useQueries({ queries: remaining.map((t) => ({
    queryKey: keys.ticket(t.key),
    queryFn: () => unwrap(api.GET("/tickets/{key}", { params: { path: { key: t.key } } })),
  })) });
  const loading = list.isPending || details.some((q) => q.isPending);
  const failed = list.isError || details.some((q) => q.isError);
  const incomplete = !!list.data && list.data.length < epic.counts.total;
  const candidates = details.flatMap((q) => q.data && q.data.status !== "done" && !hasLiveRun(q.data) ? [q.data] : []);
  const dispatchKeys = candidates.map((t) => t.key);
  const unassignedKeys = candidates.filter((t) => !t.assignee).map((t) => t.key);
  const unassigned = unassignedKeys.length;
  return (
    <div className="space-y-2 border-t border-border-subtle px-5 py-3">
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
        <Button variant="approve" disabled={loading || failed || incomplete || !dispatchKeys.length || start.isPending || (unassigned > 0 && !selectedAgent)}
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

function CardMockupToggle({ epic: e }: { epic: EpicOverview }) {
  const updateEpic = useUpdateEpic(e.epic.id);
  return (
    <div className="flex items-center gap-2" onClick={(ev) => ev.stopPropagation()}>
      <Toggle
        checked={e.epic.requires_mockup}
        disabled={updateEpic.isPending}
        onChange={(v) => updateEpic.mutate({ requires_mockup: v })}
        label={
          <>
            <span aria-hidden>Mockup</span>
            <span className="sr-only">Requires mockup for {e.epic.name}</span>
          </>
        }
      />
      {updateEpic.isError && (
        <span role="alert" className="text-xs text-pill-blocked-fg">
          {updateEpic.error.message}
        </span>
      )}
    </div>
  );
}

function Featured({ epic: e }: { epic: EpicOverview }) {
  const updateEpic = useUpdateEpic(e.epic.id);
  return (
    <Card bodyClassName="p-0" className="overflow-hidden rounded-[14px]">
      <div className="grid gap-6 p-5 lg:grid-cols-[minmax(0,2fr)_minmax(0,1fr)]">
        <div className="min-w-0 space-y-4">
          <div className="flex items-start gap-3">
            <span className="grid size-11 shrink-0 place-items-center rounded-xl border border-border-strong bg-selected text-fg-secondary">
              <Icon name="epics" size={22} />
            </span>
            <div className="min-w-0 flex-1">
              <div className="mb-1 flex flex-wrap items-center gap-2">
                <h2 className="type-title">{e.epic.name}</h2>
                <ButtonLink variant="outline" href={`/plans?epic=${e.epic.id}&project=${e.project.key}${e.source ? `&artifact=${e.source.id}` : ""}`}>Plan with an agent</ButtonLink>
                <HealthPill health={e.epic.health} />
              </div>
              <p className="text-xs text-fg-tertiary">
                {e.project.name} ·{" "}
                {e.target_date
                  ? `Target ${dueDate(e.target_date)}`
                  : "No target date"}
              </p>
            </div>
          </div>
          <p className="text-sm text-fg-secondary">
            {e.objective || "No objective has been written yet."}
          </p>
          <div className="space-y-2">
            <WorkflowBar counts={e.counts} />
            <div className="flex justify-between text-xs text-fg-tertiary">
              <span>
                <span className="font-mono text-fg">
                  {e.counts.done} of {e.counts.total}
                </span>{" "}
                tickets done
              </span>
              <span className="font-mono">
                {e.counts.total
                  ? Math.round((e.counts.done / e.counts.total) * 100)
                  : 0}
                %
              </span>
            </div>
          </div>
        </div>
        <dl className="grid grid-cols-[75px_minmax(0,1fr)] items-start gap-x-3 gap-y-3 text-xs">
          <dt className="text-fg-tertiary">Agents</dt>
          <dd>
            <AgentStack agents={e.agents} />
          </dd>
          <dt className="text-fg-tertiary">Owner</dt>
          <dd>
            {e.owner ? (
              <span className="flex items-center gap-2">
                <HumanAvatar name={e.owner.name} initials={e.owner.initials} />
                {e.owner.name}
              </span>
            ) : (
              "Unassigned"
            )}
          </dd>
          <dt className="text-fg-tertiary">Source</dt>
          <dd>
            {e.source ? (
              <Link
                href={e.source.href}
                className="flex items-start gap-2 hover:text-accent"
              >
                <DocTypeTag type={e.source.type} />
                <span>{e.source.title}</span>
              </Link>
            ) : (
              <span className="text-fg-tertiary">No linked PRD</span>
            )}
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
      </div>
      <FeaturedCompletion projectKey={e.project.key} epicId={e.epic.id} />
      <EpicDispatch key={e.epic.id} epic={e} />
      <div className="flex items-center justify-between border-t border-border-subtle bg-surface-sunken px-5 py-3">
        <h3 className="text-xs font-medium text-fg-secondary">Tickets</h3>
        <Link
          className="text-xs text-fg-secondary hover:text-accent"
          href={`/tickets?view=all&epic=${e.epic.id}`}
        >
          View all {e.counts.total} →
        </Link>
      </div>
      {e.tickets.length ? (
        e.tickets.map((t) => <WorkRow key={t.id} ticket={t} />)
      ) : (
        <p className="p-5 text-sm text-fg-tertiary">
          No tickets in this epic yet.
        </p>
      )}
    </Card>
  );
}
/** Same criteria editor and done gate as the board header, sourced from the epic's board summary. */
function FeaturedCompletion({ projectKey, epicId }: { projectKey: string; epicId: string }) {
  const board = useBoard(projectKey, { epic: epicId, include_canceled: false });
  return (
    <section aria-label="Epic completion" className="border-t border-border-subtle px-5 py-4">
      {board.data?.epic ? <EpicCompletion summary={board.data.epic} />
        : board.isError ? <p role="alert" className="text-xs">Couldn&apos;t load acceptance criteria: {board.error.message} <button type="button" className="underline" onClick={() => void board.refetch()}>Retry</button></p>
        : <p role="status" className="text-xs text-fg-tertiary">Loading acceptance criteria…</p>}
    </section>
  );
}
export function EpicsScreen() {
  const query = useEpicOverviews();
  const params = useSearchParams(),
    router = useRouter();
  const [project, setProject] = useState("");
  const epics = (query.data ?? []).filter(
    (e) => !project || e.project.key === project,
  );
  const selected =
    epics.find((e) => e.epic.id === params.get("epic")) ??
    epics.find(
      (e) => e.counts.total > e.counts.done && e.epic.health !== "on_track",
    ) ??
    epics[0];
  const active = epics.filter(
    (e) => e.counts.total === 0 || e.counts.done < e.counts.total,
  );
  const select = (id: string) =>
    router.replace(`/epics?epic=${encodeURIComponent(id)}`, { scroll: false });
  const projects = [
    ...new Map(
      (query.data ?? []).map((e) => [e.project.key, e.project]),
    ).values(),
  ];
  return (
    <>
      <TopBar
        crumbs={[{ label: "Epics" }]}
        actions={
          <ButtonLink href="/tickets/new" variant="primary" icon="plus">
            New ticket
          </ButtonLink>
        }
      />
      <Content>
        <div className="space-y-6">
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div className="space-y-1">
              <h1 className="type-display">Epics</h1>
              <p className="text-sm text-fg-secondary">
                Objectives, progress and the work ahead.
              </p>
            </div>
            <label className="flex items-center gap-2 text-xs text-fg-tertiary">
              Project
              <select
                value={project}
                onChange={(e) => setProject(e.target.value)}
                className="h-[30px] rounded-md border border-border-strong bg-surface px-2 text-fg"
              >
                <option value="">All projects</option>
                {projects.map((p) => (
                  <option value={p.key} key={p.key}>
                    {p.name}
                  </option>
                ))}
              </select>
            </label>
          </div>
          {query.isError ? (
            <Banner
              tone="blocked"
              title="Couldn’t load epics"
              actions={<Button onClick={() => query.refetch()}>Retry</Button>}
            >
              {query.error.message}
            </Banner>
          ) : query.isLoading ? (
            <div aria-busy="true" className="space-y-4">
              <Skeleton className="h-72 rounded-xl" />
              <Skeleton className="h-56 rounded-xl" />
            </div>
          ) : !epics.length ? (
            <EmptyState icon="epics" title="No epics here yet">
              Epics and their objectives will appear here once they’re created.
            </EmptyState>
          ) : (
            <>
              {selected && <Featured epic={selected} />}
              <Timeline epics={epics} onSelect={select} />
              <div className="flex items-baseline justify-between">
                <h2 className="type-heading">All active epics</h2>
                <span className="font-mono text-xs text-fg-tertiary">
                  {active.length}
                </span>
              </div>
              {active.length ? (
                <div className="grid grid-cols-[repeat(auto-fit,minmax(min(100%,300px),1fr))] gap-4">
                  {active.map((e) => (
                    <article
                      key={e.epic.id}
                      className={cx(
                        "flex min-w-0 flex-col gap-3 rounded-xl border bg-surface p-[18px]",
                        selected?.epic.id === e.epic.id
                          ? "border-accent/50"
                          : "border-border hover:border-border-hover",
                      )}
                    >
                      <div className="flex flex-wrap items-center justify-between gap-2">
                        <span className="text-xs text-fg-tertiary">
                          {e.project.name}
                        </span>
                        <HealthPill health={e.epic.health} />
                      </div>
                      <button
                        onClick={() => select(e.epic.id)}
                        className="text-left type-heading hover:text-accent"
                      >
                        {e.epic.name}
                      </button>
                      <p className="line-clamp-2 min-h-10 text-[13px] text-fg-secondary">
                        {e.objective || "No objective yet."}
                      </p>
                      <div className="mt-auto space-y-2">
                        <WorkflowBar counts={e.counts} />
                        <p className="font-mono text-xs text-fg-tertiary">
                          {e.counts.done}/{e.counts.total} done ·{" "}
                          {e.counts.total
                            ? Math.round((e.counts.done / e.counts.total) * 100)
                            : 0}
                          %
                        </p>
                      </div>
                      <CardMockupToggle epic={e} />
                      <div className="flex items-center justify-between gap-3 border-t border-border-subtle pt-3">
                        <AgentStack agents={e.agents} />
                        <span className="text-xs text-fg-tertiary">
                          {e.target_date ? dueDate(e.target_date) : "No target"}
                        </span>
                      </div>
                    </article>
                  ))}
                </div>
              ) : (
                <p className="text-sm text-fg-tertiary">
                  All epics are complete.
                </p>
              )}
            </>
          )}
        </div>
      </Content>
    </>
  );
}

"use client";

import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { useEffect, useState } from "react";
import { useEpicOverviews } from "@/lib/api/hooks";
import { dueDate } from "@/lib/format";
import {
  EPIC_SORTS, HEALTH_LABEL, epicFilterQuery, filterEpics, isOverdue, parseEpicFilters, ratio, sortEpics, statusCounts, ymd,
  type EpicFilters, type EpicHealthFilter, type EpicSort, type EpicStatusFilter,
} from "@/lib/epics";
import { Button, ButtonLink } from "@/components/ui/button";
import { Banner, Card, EmptyState, Skeleton } from "@/components/ui/primitives";
import { Segmented } from "@/components/ui/controls";
import { Content, TopBar } from "@/components/shell/topbar";
import { ProjectScopeState, projectCrumbs, useProjectScope } from "@/components/shell/project-scope";
import { HealthPill, type EpicOverview } from "@/components/domain/overview";
import { HumanAvatar } from "@/components/domain/avatars";
import { SmallPill } from "@/components/domain/labels";
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

/** One row of the card: label, value and a bar. */
function Meter({ label, value, children }: { label: string; value: string; children: React.ReactNode }) {
  return (
    <div className="space-y-1.5">
      <div className="flex justify-between gap-3 text-xs">
        <span className="text-fg-tertiary">{label}</span>
        <span className="font-mono text-fg-secondary">{value}</span>
      </div>
      {children}
    </div>
  );
}

export function EpicCard({ epic: e, today }: { epic: EpicOverview; today: string }) {
  const { epic } = e;
  const pct = Math.round(ratio(epic.points_done, epic.points_total) * 100);
  const overdue = isOverdue(e, today);
  const segments = Math.min(epic.criteria_total, 12);
  // More than 12 criteria collapse into a proportional bar.
  const met = epic.criteria_total > 12 ? Math.round((epic.criteria_done / epic.criteria_total) * segments) : epic.criteria_done;
  return (
    <article aria-label={epic.name} className="relative flex min-w-0 flex-col gap-3 rounded-xl border border-border bg-surface p-[18px] hover:border-border-hover">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <span className="text-xs text-fg-tertiary">{e.project.name}</span>
        <span className="flex gap-1.5">
          <SmallPill tone={epic.status === "done" ? "done" : "neutral"}>{epic.status === "done" ? "Done" : "Open"}</SmallPill>
          <HealthPill health={epic.health} />
        </span>
      </div>
      <h3 className="type-heading">
        {/* The stretched link makes the whole card open the epic. */}
        <Link href={`/epics/${epic.id}`} className="after:absolute after:inset-0 after:rounded-xl hover:text-accent">
          {epic.name}
        </Link>
      </h3>
      <p className="line-clamp-2 min-h-10 text-[13px] text-fg-secondary">{e.objective || "No objective yet."}</p>
      <Meter label="Points" value={epic.points_total ? `${epic.points_done} / ${epic.points_total} pts · ${pct}%` : "No estimates yet"}>
        <div role="progressbar" aria-label={`Points for ${epic.name}`} aria-valuemin={0} aria-valuemax={epic.points_total || 1} aria-valuenow={epic.points_done}
          className="h-1.5 overflow-hidden rounded-full bg-surface-sunken">
          <div className="h-full rounded-full bg-accent" style={{ width: `${pct}%` }} />
        </div>
      </Meter>
      <Meter label="Criteria" value={epic.criteria_total ? `${epic.criteria_done} / ${epic.criteria_total} met` : "None yet"}>
        {epic.criteria_total ? (
          <div role="progressbar" aria-label={`Criteria for ${epic.name}`} aria-valuemin={0} aria-valuemax={epic.criteria_total} aria-valuenow={epic.criteria_done} className="flex gap-1">
            {Array.from({ length: segments }, (_, i) => (
              <span key={i} className={cx("h-1.5 flex-1 rounded-full", i < met ? "bg-state-done" : "bg-surface-sunken")} />
            ))}
          </div>
        ) : epic.status === "open" ? (
          <p className="text-[11px] text-pill-human-fg">Add acceptance criteria before it can be marked done</p>
        ) : null}
      </Meter>
      <div className="mt-auto flex flex-wrap items-center justify-between gap-3 border-t border-border-subtle pt-3 text-xs">
        <span className="text-fg-secondary"><span className="font-mono text-fg">{epic.tickets_open}</span> open tickets</span>
        <span className="flex items-center gap-2 text-fg-secondary">
          {e.owner ? <><HumanAvatar name={e.owner.name} initials={e.owner.initials} color={e.owner.avatar_color} />{e.owner.name}</>
            : <span className="text-fg-tertiary">No owner</span>}
        </span>
        <span className={overdue ? "text-pill-blocked-fg" : "text-fg-tertiary"}>
          {e.target_date ? `${overdue ? "Overdue ·" : "Target"} ${dueDate(e.target_date)}` : "No target date"}
        </span>
      </div>
    </article>
  );
}

const HEALTH_OPTIONS: EpicHealthFilter[] = ["any", "on_track", "at_risk", "off_track"];
const HEALTH_DOT: Record<Exclude<EpicHealthFilter, "any">, string> = {
  on_track: "bg-state-done", at_risk: "bg-state-human", off_track: "bg-state-blocked",
};

export function EpicsScreen() {
  const query = useEpicOverviews();
  const params = useSearchParams(),
    router = useRouter();
  // The sidebar switcher sets ?project=; none (All projects) lists every epic.
  const scope = useProjectScope("optional");
  const current = scope.status === "ready" ? scope.project : undefined;
  const project = current?.key ?? "";
  const legacy = params.get("epic");
  // Old links (/epics?epic=…) open the detail page.
  useEffect(() => {
    if (legacy) router.replace(`/epics/${encodeURIComponent(legacy)}`);
  }, [legacy, router]);
  const filters = parseEpicFilters(params);
  const setFilters = (patch: Partial<EpicFilters>) =>
    router.replace(`/epics${epicFilterQuery({ ...filters, ...patch }, project)}`, { scroll: false });
  const inProject = (query.data ?? []).filter((e) => !project || e.project.key === project);
  const counts = statusCounts(inProject);
  const shown = sortEpics(filterEpics(inProject, filters), filters.sort);
  const today = ymd(new Date());
  return (
    <>
      <TopBar
        crumbs={projectCrumbs(current, "Epics")}
        actions={
          <ButtonLink href={project ? `/tickets/new?project=${encodeURIComponent(project)}` : "/tickets/new"} variant="primary" icon="plus">
            New ticket
          </ButtonLink>
        }
      />
      <Content>
        <div className="space-y-6">
          <div className="space-y-1">
            <h1 className="type-display">Epics</h1>
            <p className="text-sm text-fg-secondary">
              Objectives, progress and the done gate for every epic in this project.
            </p>
          </div>
          {scope.status !== "ready" ? (
            <ProjectScopeState base="/epics" scope={scope} />
          ) : query.isError ? (
            <Banner
              tone="blocked"
              title="Couldn’t load epics"
              actions={<Button onClick={() => query.refetch()}>Retry</Button>}
            >
              {query.error.message}
            </Banner>
          ) : query.isLoading || legacy ? (
            <div aria-busy="true" className="grid grid-cols-[repeat(auto-fill,minmax(min(100%,320px),1fr))] gap-4">
              {[0, 1, 2].map((i) => <Skeleton key={i} className="h-60 rounded-xl" />)}
            </div>
          ) : !inProject.length ? (
            <EmptyState icon="epics" title="No epics here yet">
              Epics and their objectives will appear here once they’re created.
            </EmptyState>
          ) : (
            <>
              <div className="flex flex-wrap items-center gap-3">
                <Segmented<EpicStatusFilter>
                  label="Epic status"
                  value={filters.status}
                  onChange={(status) => setFilters({ status })}
                  options={(["open", "done", "all"] as const).map((s) => ({
                    value: s,
                    label: <>{s === "open" ? "Open" : s === "done" ? "Done" : "All"} <span className="font-mono text-fg-tertiary">{counts[s]}</span></>,
                  }))}
                />
                <div role="radiogroup" aria-label="Health" className="flex flex-wrap items-center gap-1.5">
                  <span className="text-xs text-fg-tertiary">Health</span>
                  {HEALTH_OPTIONS.map((h) => (
                    <button key={h} type="button" role="radio" aria-checked={filters.health === h} onClick={() => setFilters({ health: h })}
                      className={cx("flex h-7 items-center gap-1.5 rounded-full border px-3 text-xs",
                        filters.health === h ? "border-border-strong bg-selected text-fg" : "border-border text-fg-secondary hover:text-fg")}>
                      {h !== "any" && <span aria-hidden className={cx("size-2 rounded-full", HEALTH_DOT[h])} />}
                      {h === "any" ? "Any" : HEALTH_LABEL[h]}
                    </button>
                  ))}
                </div>
                <span className="ml-auto font-mono text-xs text-fg-tertiary" role="status">{shown.length} {shown.length === 1 ? "epic" : "epics"}</span>
                <label className="flex items-center gap-2 text-xs text-fg-secondary">
                  Sort
                  <select aria-label="Sort epics" value={filters.sort} onChange={(ev) => setFilters({ sort: ev.target.value as EpicSort })}
                    className="h-[30px] rounded-md border border-border-strong bg-surface px-2 text-fg">
                    {(Object.keys(EPIC_SORTS) as EpicSort[]).map((s) => <option key={s} value={s}>{EPIC_SORTS[s]}</option>)}
                  </select>
                </label>
              </div>
              {shown.length ? (
                <div className="grid grid-cols-[repeat(auto-fill,minmax(min(100%,320px),1fr))] gap-4">
                  {shown.map((e) => <EpicCard key={e.epic.id} epic={e} today={today} />)}
                </div>
              ) : (
                <EmptyState icon="epics" title="No epics match"
                  actions={<Button onClick={() => setFilters({ status: "all", health: "any" })}>Clear filters</Button>}>
                  No {filters.status === "all" ? "" : `${filters.status} `}epics{filters.health === "any" ? "" : ` are ${HEALTH_LABEL[filters.health].toLowerCase()}`}.
                </EmptyState>
              )}
              <Timeline epics={shown} onSelect={(id) => router.push(`/epics/${id}`)} />
            </>
          )}
        </div>
      </Content>
    </>
  );
}

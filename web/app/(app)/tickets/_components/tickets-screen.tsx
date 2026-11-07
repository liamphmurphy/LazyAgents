"use client";

import { useRouter, useSearchParams } from "next/navigation";
import { DropdownMenu } from "radix-ui";
import { useEffect, useMemo, useRef, useState, useSyncExternalStore, type MouseEvent } from "react";
import { cx } from "@/lib/cx";
import type { TicketStatus, TicketSummary } from "@/lib/api/client";
import { useBulkEach, useBootstrap, useSavedViews, useTickets, type TicketView } from "@/lib/api/hooks";
import {
  GROUPS,
  GROUP_NAME,
  applyLocalFilters,
  groupTickets,
  moveFocus,
  parseGroup,
  rangeKeys,
  type BulkResult,
  type TicketGroup,
} from "@/lib/ticket-list";
import { ButtonLink } from "@/components/ui/button";
import { Tabs } from "@/components/ui/controls";
import { Icon } from "@/components/ui/icon";
import { Banner, EmptyState, FilterChip, Kbd, Skeleton } from "@/components/ui/primitives";
import { AgentAvatar, UnassignedAvatar } from "@/components/domain/avatars";
import { PriorityGlyph, StatusGlyph } from "@/components/domain/glyphs";
import { PRIORITY_NAME, STATUS_NAME } from "@/components/domain/meta";
import { AgentPicker, floatPanel } from "@/components/domain/pickers";
import { SavedViewHeader } from "@/components/domain/saved-views";
import { TopBar } from "@/components/shell/topbar";
import { ProjectScopeState, projectCrumbs, useProjectScope } from "@/components/shell/project-scope";
import { BulkBar, BulkResultPanel } from "./bulk-bar";
import { COL, TicketRow, type Density } from "./ticket-row";

const VIEWS: Array<{ key: TicketView; label: string }> = [
  { key: "all", label: "All tickets" },
  { key: "active", label: "Active" },
  { key: "backlog", label: "Backlog" },
  { key: "agent_owned", label: "Agent-owned" },
  { key: "needs_human", label: "Needs human" },
];

// Density is a per-browser preference, not part of the URL or a saved view.
const DENSITY_KEY = "la:tickets-density";
const DENSITY_EVENT = "la:tickets-density";
function readDensity(): Density {
  try {
    return window.localStorage.getItem(DENSITY_KEY) === "compact" ? "compact" : "comfortable";
  } catch {
    return "comfortable";
  }
}
function subscribeDensity(cb: () => void) {
  window.addEventListener("storage", cb);
  window.addEventListener(DENSITY_EVENT, cb);
  return () => {
    window.removeEventListener("storage", cb);
    window.removeEventListener(DENSITY_EVENT, cb);
  };
}
function setDensity(d: Density) {
  try {
    window.localStorage.setItem(DENSITY_KEY, d);
  } catch {
    /* Storage unavailable: the choice lasts until reload. */
  }
  window.dispatchEvent(new Event(DENSITY_EVENT));
}

function isTyping(el: EventTarget | null) {
  if (!(el instanceof HTMLElement)) return false;
  if (el instanceof HTMLInputElement && ["checkbox", "radio", "button", "submit", "reset"].includes(el.type)) return false;
  return el.isContentEditable || ["INPUT", "TEXTAREA", "SELECT"].includes(el.tagName) || el.getAttribute("role") === "combobox";
}

function ListSkeleton() {
  return (
    <div aria-busy="true" aria-label="Loading tickets">
      {[0, 1].map((g) => (
        <div key={g}>
          <div className="flex h-[38px] items-center gap-3 border-b border-border-row bg-surface-sunken px-6">
            <Skeleton className="size-3.5 rounded-full" />
            <Skeleton className="h-3 w-24" />
          </div>
          {[0.6, 0.45, 0.7, 0.5].map((w, i) => (
            <div key={i} className="flex h-11 items-center gap-3 border-b border-border-row px-6 pl-[50px]">
              <Skeleton className="h-3 w-16" />
              <Skeleton className="size-3.5 rounded-full" />
              <Skeleton className="h-3" style={{ width: `${w * 50}%` }} />
            </div>
          ))}
        </div>
      ))}
    </div>
  );
}

function GroupLabel({ g }: { g: TicketGroup }) {
  if (g.by === "status") {
    const s = g.id as TicketStatus;
    return (
      <>
        <StatusGlyph state={s} surface="var(--surface-sunken)" label={false} />
        <span className="font-medium">{STATUS_NAME[s]}</span>
      </>
    );
  }
  if (g.by === "priority") {
    const p = g.id as TicketSummary["priority"];
    return (
      <>
        <PriorityGlyph priority={p} label={false} />
        <span className="font-medium">{PRIORITY_NAME[p]}</span>
      </>
    );
  }
  if (g.by === "epic") {
    return (
      <>
        <Icon name="epics" size={14} className={g.id === "none" ? "text-fg-tertiary" : "text-fg-secondary"} />
        <span className={cx("font-medium", g.id === "none" && "text-fg-secondary")}>{g.id === "none" ? "No epic" : g.name}</span>
      </>
    );
  }
  const a = g.tickets[0]?.assignee;
  return a ? (
    <>
      <AgentAvatar name={a.name} colorIndex={a.color_index} label={false} />
      <span className="font-medium">{a.name}</span>
    </>
  ) : (
    <>
      <UnassignedAvatar />
      <span className="font-medium text-fg-secondary">Unassigned</span>
    </>
  );
}

function groupLabelText(g: TicketGroup) {
  if (g.by === "status") return STATUS_NAME[g.id as TicketStatus];
  if (g.by === "priority") return PRIORITY_NAME[g.id as TicketSummary["priority"]];
  if (g.id === "none") return g.by === "epic" ? "No epic" : "Unassigned";
  return g.name;
}

/** Composer link that starts a ticket in this group. */
function newInGroup(g: TicketGroup, project?: string) {
  const p = new URLSearchParams();
  if (project) p.set("project", project);
  if (g.by === "status") p.set("status", g.id);
  return `/tickets/new${p.size ? `?${p}` : ""}`;
}

export function TicketsScreen() {
  const params = useSearchParams();
  const router = useRouter();
  const project = params.get("project") ?? undefined;
  const view = (params.get("view") as TicketView) ?? "all";
  const assignee = params.get("assignee") ?? undefined;
  const q = params.get("q") ?? "";
  const epic = params.get("epic") ?? undefined;
  const status = (params.get("status") as TicketStatus) ?? undefined;
  const ready = params.get("ready") === "true";
  const blockedOnly = params.get("blocked") === "true";
  const unestimated = params.get("unestimated") === "true";
  const group = parseGroup(params.get("group"));
  const svId = params.get("sv");
  const [search, setSearch] = useState(q);
  // Opening a saved view (or Discard) changes ?q= from outside the box; the
  // box's own debounced writes (pushedQ) must not overwrite what's typed since.
  const [urlQ, setUrlQ] = useState(q);
  const [pushedQ, setPushedQ] = useState(q);
  if (q !== urlQ) {
    setUrlQ(q);
    if (q !== pushedQ) setSearch(q);
  }
  const density = useSyncExternalStore(subscribeDensity, readDensity, () => "comfortable" as Density);

  const boot = useBootstrap();
  const each = useBulkEach();
  // No ?project= resolves to the remembered project; "All projects" leaves it empty.
  const scope = useProjectScope("optional");
  const list = useTickets({ ready, project, view, status, assignee, epic, q: q || undefined });
  const savedViews = useSavedViews();
  const savedView = svId ? savedViews.data?.find((v) => v.id === svId) : undefined;

  // Debounced search → URL, so views are shareable.
  useEffect(() => {
    if (search === q) return;
    const id = setTimeout(() => {
      setPushedQ(search);
      setParam("q", search || null);
    }, 250);
    return () => clearTimeout(id);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [search]);

  function replaceParams(edit: (p: URLSearchParams) => void) {
    const next = new URLSearchParams(params);
    edit(next);
    router.replace(`/tickets${next.size ? `?${next}` : ""}`);
  }
  function setParam(k: string, v: string | null) {
    replaceParams((next) => (v ? next.set(k, v) : next.delete(k)));
  }
  const href = (v: TicketView) => {
    const next = new URLSearchParams(params);
    next.set("view", v);
    return `/tickets?${next}`;
  };

  const tickets = useMemo(() => applyLocalFilters(list.data ?? [], { blocked: blockedOnly, unestimated }), [list.data, blockedOnly, unestimated]);
  const groups = useMemo(() => groupTickets(tickets, group), [tickets, group]);
  const flat = useMemo(() => groups.flatMap((g) => g.tickets.map((t) => t.key)), [groups]);

  // Selection (x, Shift+x, shift-click ranges) and the j/k cursor, keyed by ticket key.
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [focused, setFocused] = useState<string | null>(null);
  const anchor = useRef<string | null>(null);
  const [result, setResult] = useState<BulkResult | null>(null);
  const visible = useMemo(() => new Set(flat), [flat]);
  const selectedKeys = flat.filter((k) => selected.has(k));

  function toggleKey(key: string, range: boolean) {
    const next = new Set(selected);
    if (range && anchor.current) {
      for (const k of rangeKeys(flat, anchor.current, key)) next.add(k);
    } else if (next.has(key)) next.delete(key);
    else next.add(key);
    anchor.current = key;
    setSelected(next);
  }
  function toggle(t: TicketSummary, e: MouseEvent<HTMLInputElement>) {
    toggleKey(t.key, e.shiftKey);
    setFocused(t.key);
  }

  // Latest state for the keyboard handler without re-subscribing every render.
  const keyState = useRef({ flat, focused, toggleKey, router });
  useEffect(() => {
    keyState.current = { flat, focused, toggleKey, router };
  });
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.metaKey || e.ctrlKey || e.altKey || isTyping(e.target) || document.querySelector('[role="dialog"], [role="menu"]')) return;
      const s = keyState.current;
      if (e.key === "Escape") {
        setSelected(new Set());
        anchor.current = null;
        return;
      }
      if (e.key === "j" || e.key === "k") {
        e.preventDefault();
        const next = moveFocus(s.flat, s.focused && s.flat.includes(s.focused) ? s.focused : null, e.key === "j" ? 1 : -1);
        setFocused(next);
        if (next) document.querySelector(`[data-ticket-row="${CSS.escape(next)}"]`)?.scrollIntoView({ block: "nearest" });
        return;
      }
      if ((e.key === "x" || e.key === "X") && s.focused && s.flat.includes(s.focused)) {
        e.preventDefault();
        s.toggleKey(s.focused, e.shiftKey);
        return;
      }
      if (e.key === "Enter" && s.focused && s.flat.includes(s.focused) && !(e.target instanceof HTMLButtonElement || e.target instanceof HTMLAnchorElement)) {
        e.preventDefault();
        s.router.push(`/tickets/${s.focused}`);
      }
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  async function applyBulk(patch: Parameters<typeof each.mutateAsync>[0]["patch"], label: string) {
    const keys = [...selectedKeys];
    try {
      const outcome = await each.mutateAsync({ keys, patch });
      onResult({ label, ...outcome });
    } catch (error) {
      onResult({ label, updated: [], failed: keys.map((key) => ({ key, status: 0, message: error instanceof Error ? error.message : String(error) })) });
    }
  }

  function onResult(r: BulkResult) {
    setResult(r);
    // Only the failed tickets stay selected, so they can be retried or opened.
    setSelected(new Set(r.failed.map((f) => f.key)));
    anchor.current = null;
  }
  // A clean result dismisses itself.
  useEffect(() => {
    if (!result || result.failed.length) return;
    const id = setTimeout(() => setResult(null), 5000);
    return () => clearTimeout(id);
  }, [result]);

  const proj = boot.data?.projects.find((p) => p.key === project);
  const crumbs = projectCrumbs(proj, "Tickets");
  if (savedView) {
    crumbs[crumbs.length - 1] = { ...crumbs[crumbs.length - 1], href: `/tickets${project ? `?project=${encodeURIComponent(project)}` : ""}` };
    crumbs.push({ label: savedView.name });
  }
  // Archived projects keep their read-only history (banner below); unknown keys and redirects don't list.
  const blocked = scope.status !== "ready" && !(scope.status === "unavailable" && scope.archived);
  const agentFilter = boot.data?.agents.find((a) => a.handle === assignee);
  const projectEpics = (boot.data?.epics ?? []).filter((e) => !proj || e.project_id === proj.id);
  const filtered = !!(q || assignee || status || blockedOnly || unestimated || epic || ready);
  const allSelected = flat.length > 0 && selectedKeys.length === flat.length;

  return (
    <>
      <TopBar
        crumbs={crumbs}
        actions={
          proj?.archived ? <ButtonLink href="/settings/projects" variant="outline">Manage project</ButtonLink> : <ButtonLink href={`/tickets/new${project ? `?project=${project}` : ""}`} variant="primary" icon="plus" hint="C">New ticket</ButtonLink>
        }
      />
      {proj?.archived && <div className="px-6 py-3"><Banner tone="human" title="Archived project">Tickets and history remain available. Restore the project in Settings to start new work.</Banner></div>}
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-border-subtle px-6 py-2.5">
        <Tabs label="Views" value={view} items={VIEWS.map((v) => ({ ...v, href: href(v.key) }))} />
        <div className="flex flex-wrap items-center gap-2">
          <button type="button" aria-pressed={ready} className={cx("h-[26px] rounded-sm border border-border-strong px-2 text-xs hover:bg-hover", ready && "bg-selected text-accent")} onClick={() => replaceParams((next) => { if (ready) next.delete("ready"); else { next.set("ready", "true"); next.set("view", "all"); } })}>Ready to dispatch</button>
          {epic && <FilterChip label="Epic" value={`${boot.data?.epics.find(e => e.id === epic)?.name ?? "Selected epic"} ×`} onClick={() => setParam("epic", null)} title="Clear epic filter"/>}
          {status && <FilterChip label="Status" value={`${STATUS_NAME[status] ?? status} ×`} onClick={() => setParam("status", null)} title="Clear status filter" />}
          {blockedOnly && <FilterChip label="Only" value="Blocked ×" onClick={() => setParam("blocked", null)} title="Clear blocked filter" />}
          {unestimated && <FilterChip label="Only" value="Unestimated ×" onClick={() => setParam("unestimated", null)} title="Clear unestimated filter" />}
          <label className="flex h-[26px] items-center gap-1.5 rounded-sm border border-border-strong px-2 text-xs text-fg-tertiary focus-within:border-accent">
            <Icon name="search" size={12} />
            <span className="sr-only">Search tickets</span>
            <input
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder="Search"
              className="w-28 bg-transparent text-fg-2 outline-none placeholder:text-fg-tertiary"
            />
          </label>
          {boot.data && (
            <AgentPicker agents={boot.data.agents} value={agentFilter?.id} onChange={(id) => setParam("assignee", boot.data.agents.find((a) => a.id === id)?.handle ?? null)}>
              <button type="button" className="inline-flex h-[26px] items-center overflow-hidden rounded-sm border border-border-strong text-xs hover:bg-hover">
                <span className="flex h-full items-center border-r border-border-strong px-2 text-fg-tertiary">Agent</span>
                <span className="px-2 text-fg-2">{agentFilter?.name ?? "any"}</span>
              </button>
            </AgentPicker>
          )}
          <DropdownMenu.Root>
            <DropdownMenu.Trigger asChild>
              <button
                type="button"
                aria-label={`Group by ${GROUP_NAME[group]}`}
                className={cx("inline-flex h-[26px] items-center overflow-hidden rounded-sm border text-xs hover:bg-hover", group === "status" ? "border-border-strong" : "border-accent/55")}
              >
                <span className="flex h-full items-center border-r border-border-strong px-2 text-fg-tertiary">Group</span>
                <span className="flex items-center gap-1 px-2 text-fg">
                  {GROUP_NAME[group]}
                  <Icon name="chev" size={12} className="text-fg-secondary" />
                </span>
              </button>
            </DropdownMenu.Trigger>
            <DropdownMenu.Portal>
              <DropdownMenu.Content align="end" sideOffset={6} className={cx(floatPanel, "w-[200px]")}>
                <DropdownMenu.Label className="px-2 pb-1 pt-1.5 text-[11.5px] text-fg-tertiary">Group by</DropdownMenu.Label>
                <DropdownMenu.RadioGroup value={group} onValueChange={(g) => setParam("group", g === "status" ? null : g)}>
                  {GROUPS.map((g) => (
                    <DropdownMenu.RadioItem key={g} value={g} className="flex h-[30px] cursor-default select-none items-center rounded-md px-2 outline-none data-[highlighted]:bg-hover">
                      {GROUP_NAME[g]}
                      <DropdownMenu.ItemIndicator className="ml-auto text-accent">
                        <Icon name="check" size={14} strokeWidth={1.8} />
                      </DropdownMenu.ItemIndicator>
                    </DropdownMenu.RadioItem>
                  ))}
                </DropdownMenu.RadioGroup>
              </DropdownMenu.Content>
            </DropdownMenu.Portal>
          </DropdownMenu.Root>
          <div role="radiogroup" aria-label="Density" className="inline-flex h-[26px] overflow-hidden rounded-sm border border-border-strong">
            {(["comfortable", "compact"] as const).map((d) => (
              <button
                key={d}
                type="button"
                role="radio"
                aria-checked={density === d}
                aria-label={d === "comfortable" ? "Comfortable rows" : "Compact rows"}
                title={d === "comfortable" ? "Comfortable rows" : "Compact rows"}
                onClick={() => setDensity(d)}
                className={cx("grid w-[27px] place-items-center", d === "compact" && "border-l border-border-strong", density === d ? "bg-selected text-fg" : "text-fg-tertiary hover:bg-hover")}
              >
                <svg aria-hidden width="12" height="12" viewBox="0 0 12 12" stroke="currentColor" strokeWidth="1.3">
                  {d === "comfortable" ? <path d="M0 2h12M0 6h12M0 10h12" /> : <path d="M0 1.5h12M0 4.5h12M0 7.5h12M0 10.5h12" />}
                </svg>
              </button>
            ))}
          </div>
        </div>
      </div>

      {savedView && <SavedViewHeader view={savedView} />}

      {blocked ? (
        <ProjectScopeState base="/tickets" scope={scope} />
      ) : list.isError ? (
        <div className="p-6">
          <Banner tone="blocked" title="Couldn't load tickets">
            {list.error.message}
          </Banner>
        </div>
      ) : list.isPending ? (
        <ListSkeleton />
      ) : tickets.length === 0 ? (
        <EmptyState
          icon="tickets"
          title={filtered ? "No tickets match" : "No tickets in this view"}
          actions={<ButtonLink href="/tickets/new" variant="outline" icon="plus">New ticket</ButtonLink>}
        >
          {filtered ? "Try a different search or clear a filter." : "Tickets you create, or that agents open as follow-ups, will appear here."}
        </EmptyState>
      ) : (
        <div className="overflow-x-auto pb-32">
          <div className="min-w-[760px]">
            <div className="flex h-8 items-center gap-3 border-b border-border-row pl-6 pr-6 text-[11.5px] text-fg-tertiary">
              <input
                type="checkbox"
                aria-label={allSelected ? "Clear selection" : "Select all tickets"}
                checked={allSelected}
                ref={(el) => {
                  if (el) el.indeterminate = selectedKeys.length > 0 && !allSelected;
                }}
                onChange={() => setSelected(allSelected ? new Set() : new Set(flat))}
                className="size-3.5 flex-none accent-[var(--accent)]"
              />
              <span className="flex-1 pl-[94px]">Ticket</span>
              <span className={COL.criteria}>Criteria</span>
              <span className={COL.estimate}>Est.</span>
              <span className={COL.pr}>PR</span>
              {group !== "epic" && <span className={COL.epic}>Epic</span>}
              <span className={COL.assignee}>Assignee</span>
              <span className={COL.updated}>Updated</span>
            </div>
            {groups.map((g) => (
              <section key={`${g.by}:${g.id}`} aria-label={`${groupLabelText(g)}, ${g.tickets.length}`}>
                <h2 className="flex h-[38px] items-center gap-2.5 border-b border-border-row bg-surface-sunken pl-6 pr-6 text-[13px]">
                  <GroupLabel g={g} />
                  <span className="font-mono text-xs text-fg-tertiary">{g.tickets.length}</span>
                  <span className="flex-1" />
                  <span className={cx(COL.criteria, "font-mono text-[11.5px] text-fg-tertiary")} title="Criteria done in this group">
                    {g.criteriaTotal > 0 ? `${g.criteriaDone}/${g.criteriaTotal}` : ""}
                  </span>
                  <span className={cx(COL.estimate, "whitespace-nowrap text-[11.5px] text-fg-tertiary")} title="Estimated points in this group">
                    {g.points > 0 ? `${g.points} pts` : ""}
                  </span>
                  <ButtonLink
                    href={newInGroup(g, project)}
                    variant="ghost"
                    icon="plus"
                    aria-label={`New ticket in ${groupLabelText(g)}`}
                    className="ml-auto h-7 px-2 text-fg-tertiary"
                  />
                </h2>
                {g.tickets.map((t) => (
                  <TicketRow
                    key={t.id}
                    t={t}
                    selected={selected.has(t.key)}
                    focused={focused === t.key}
                    density={density}
                    showEpic={group !== "epic"}
                    onSelect={(e) => toggle(t, e)}
                    onFocusRow={() => setFocused(t.key)}
                  />
                ))}
              </section>
            ))}
          </div>
        </div>
      )}

      {result && (
        <BulkResultPanel
          result={result}
          raised={selectedKeys.length > 0}
          onSelectFailed={() => setSelected(new Set(result.failed.map((f) => f.key).filter((k) => visible.has(k))))}
          onDismiss={() => setResult(null)}
        />
      )}
      {selectedKeys.length > 0 && boot.data && (
        <>
          <BulkBar keys={selectedKeys} agents={boot.data.agents} epics={projectEpics} onClear={() => setSelected(new Set())} onApply={applyBulk} saving={each.isPending} />
          <p aria-hidden className="pointer-events-none fixed bottom-1 left-1/2 z-40 hidden -translate-x-1/2 whitespace-nowrap text-[11.5px] text-fg-tertiary md:block">
            <Kbd>x</Kbd> select · <Kbd>Shift+x</Kbd> range · <Kbd>j</Kbd>/<Kbd>k</Kbd> move · <Kbd>Enter</Kbd> open · <Kbd>Esc</Kbd> clear
          </p>
        </>
      )}
    </>
  );
}

"use client";

import { useRouter, useSearchParams } from "next/navigation";
import { useEffect, useMemo, useRef, useState, type MouseEvent } from "react";
import type { TicketStatus, TicketSummary } from "@/lib/api/client";
import { useBootstrap, useTickets, type TicketView } from "@/lib/api/hooks";
import { ButtonLink } from "@/components/ui/button";
import { Tabs } from "@/components/ui/controls";
import { Icon } from "@/components/ui/icon";
import { Banner, EmptyState, FilterChip, Skeleton } from "@/components/ui/primitives";
import { StatusGlyph } from "@/components/domain/glyphs";
import { STATUS_NAME, STATUS_ORDER } from "@/components/domain/meta";
import { AgentPicker } from "@/components/domain/pickers";
import { TopBar, type Crumb } from "@/components/shell/topbar";
import { BulkBar } from "./bulk-bar";
import { TicketRow } from "./ticket-row";

const VIEWS: Array<{ key: TicketView; label: string }> = [
  { key: "all", label: "All tickets" },
  { key: "active", label: "Active" },
  { key: "backlog", label: "Backlog" },
  { key: "agent_owned", label: "Agent-owned" },
  { key: "needs_human", label: "Needs human" },
];

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

export function TicketsScreen() {
  const params = useSearchParams();
  const router = useRouter();
  const project = params.get("project") ?? undefined;
  const view = (params.get("view") as TicketView) ?? "all";
  const assignee = params.get("assignee") ?? undefined;
  const q = params.get("q") ?? "";
  const epic = params.get("epic") ?? undefined;
  const [search, setSearch] = useState(q);

  const boot = useBootstrap();
  const list = useTickets({ project, view, assignee, epic, q: q || undefined });

  // Debounced search → URL, so views are shareable.
  useEffect(() => {
    if (search === q) return;
    const id = setTimeout(() => setParam("q", search || null), 250);
    return () => clearTimeout(id);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [search]);

  function setParam(k: string, v: string | null) {
    const next = new URLSearchParams(params);
    if (v) next.set(k, v);
    else next.delete(k);
    router.replace(`/tickets${next.size ? `?${next}` : ""}`);
  }
  const href = (v: TicketView) => {
    const next = new URLSearchParams(params);
    next.set("view", v);
    return `/tickets?${next}`;
  };

  // Selection with shift-click ranges, keyed by ticket key.
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const anchor = useRef<string | null>(null);
  const tickets = useMemo(() => list.data ?? [], [list.data]);
  const visible = useMemo(() => new Set(tickets.map((t) => t.key)), [tickets]);
  const selectedKeys = [...selected].filter((k) => visible.has(k));

  function toggle(t: TicketSummary, e: MouseEvent<HTMLInputElement>) {
    const next = new Set(selected);
    const flat = groups.flatMap(([, ts]) => ts);
    if (e.shiftKey && anchor.current) {
      const a = flat.findIndex((x) => x.key === anchor.current);
      const b = flat.findIndex((x) => x.key === t.key);
      if (a >= 0 && b >= 0) {
        for (const x of flat.slice(Math.min(a, b), Math.max(a, b) + 1)) next.add(x.key);
        setSelected(next);
        return;
      }
    }
    if (next.has(t.key)) next.delete(t.key);
    else next.add(t.key);
    anchor.current = t.key;
    setSelected(next);
  }

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && setSelected(new Set());
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  const groups = useMemo(() => {
    const by = new Map<TicketStatus, TicketSummary[]>();
    for (const t of tickets) by.set(t.status, [...(by.get(t.status) ?? []), t]);
    return STATUS_ORDER.filter((s) => by.has(s)).map((s) => [s, by.get(s)!] as const);
  }, [tickets]);

  const proj = boot.data?.projects.find((p) => p.key === project);
  const crumbs: Crumb[] = proj
    ? [{ label: proj.name, swatch: proj.color, href: `/tickets?project=${proj.key}` }, { label: "Tickets" }]
    : [{ label: "Tickets" }];
  const agentFilter = boot.data?.agents.find((a) => a.handle === assignee);

  return (
    <>
      <TopBar
        crumbs={crumbs}
        actions={
          <ButtonLink href={`/tickets/new${project ? `?project=${project}` : ""}`} variant="primary" icon="plus" hint="C">
            New ticket
          </ButtonLink>
        }
      />
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-border-subtle px-6 py-2.5">
        <Tabs label="Views" value={view} items={VIEWS.map((v) => ({ ...v, href: href(v.key) }))} />
        <div className="flex flex-wrap items-center gap-2">
          {epic && <FilterChip label="Epic" value={`${boot.data?.epics.find(e => e.id === epic)?.name ?? "Selected epic"} ×`} onClick={() => setParam("epic", null)} title="Clear epic filter"/>}
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
        </div>
      </div>

      {list.isError ? (
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
          title={q || assignee ? "No tickets match" : "No tickets in this view"}
          actions={<ButtonLink href="/tickets/new" variant="outline" icon="plus">New ticket</ButtonLink>}
        >
          {q || assignee ? "Try a different search or clear the agent filter." : "Tickets you create, or that agents open as follow-ups, will appear here."}
        </EmptyState>
      ) : (
        <div className="overflow-x-auto pb-24">
          <div className="min-w-[620px]">
            {groups.map(([status, ts]) => (
              <section key={status} aria-label={`${STATUS_NAME[status]}, ${ts.length}`}>
                <h2 className="flex h-[38px] items-center gap-2.5 border-b border-border-row bg-surface-sunken pl-6 pr-4 text-[13px]">
                  <StatusGlyph state={status} surface="var(--surface-sunken)" label={false} />
                  <span className="font-medium">{STATUS_NAME[status]}</span>
                  <span className="font-mono text-xs text-fg-tertiary">{ts.length}</span>
                  <ButtonLink
                    href={`/tickets/new?status=${status}${project ? `&project=${project}` : ""}`}
                    variant="ghost"
                    icon="plus"
                    aria-label={`New ${STATUS_NAME[status]} ticket`}
                    className="ml-auto h-7 px-2 text-fg-tertiary"
                  />
                </h2>
                {ts.map((t) => (
                  <TicketRow key={t.id} t={t} selected={selected.has(t.key)} onSelect={(e) => toggle(t, e)} />
                ))}
              </section>
            ))}
          </div>
        </div>
      )}

      {selectedKeys.length > 0 && boot.data && (
        <BulkBar keys={selectedKeys} agents={boot.data.agents} onClear={() => setSelected(new Set())} />
      )}
    </>
  );
}

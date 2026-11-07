"use client";

import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { useRef, useState } from "react";
import { DropdownMenu } from "radix-ui";
import { useBootstrap } from "@/lib/api/hooks";
import { useBoard, useBoardMoves, type BoardFilters } from "@/lib/api/board-hooks";
import { BOARD_STATUSES, withBoardMoves, type Board, type BoardCard, type BoardStatus } from "@/lib/board";
import { cx } from "@/lib/cx";
import { TopBar } from "@/components/shell/topbar";
import { AgentAvatar, UnassignedAvatar } from "@/components/domain/avatars";
import { PriorityGlyph, StatusGlyph } from "@/components/domain/glyphs";
import { PRIORITY_NAME, STATUS_NAME } from "@/components/domain/meta";
import { floatPanel } from "@/components/domain/pickers";
import { EpicCompletion } from "@/components/domain/epic-completion";

const selectClass = "h-8 max-w-52 rounded-md border border-border-strong bg-surface px-2 text-xs text-fg";

export function BoardScreen() {
  const bootstrap = useBootstrap();
  const params = useSearchParams();
  const projects = bootstrap.data?.projects.filter((p) => !p.archived) ?? [];
  const requested = params.get("project");
  const project = requested ? projects.find((p) => p.key === requested) : projects[0];
  const filters: BoardFilters = {
    epic: params.get("epic") || undefined,
    assignee: params.get("assignee") || undefined,
    include_canceled: params.get("include_canceled") === "true",
  };
  const epics = bootstrap.data?.epics.filter((e) => e.project_id === project?.id) ?? [];
  function setParam(name: string, value: string) {
    // These filters only fetch client data. Native history integrates with
    // useSearchParams without an asynchronous server navigation per input.
    const next = new URLSearchParams(window.location.search);
    if (project) next.set("project", project.key);
    if (value) next.set(name, value); else next.delete(name);
    if (name === "project") next.delete("epic");
    window.history.replaceState(null, "", `/board?${next}`);
  }
  return (
    <div data-testid="board-screen">
      <TopBar crumbs={[{ label: "Board" }, ...(project ? [{ label: project.key, mono: true }] : [])]} actions={
        <>
          <select aria-label="Project" className={selectClass} value={project?.key ?? ""} onChange={(e) => setParam("project", e.target.value)}>
            {!project && <option value="">Choose project</option>}
            {projects.map((p) => <option key={p.key} value={p.key}>{p.name}</option>)}
          </select>
          <select aria-label="Epic" className={selectClass} value={filters.epic ?? ""} onChange={(e) => setParam("epic", e.target.value)}>
            <option value="">All epics</option>
            {epics.map((e) => <option key={e.id} value={e.id}>{e.name}</option>)}
          </select>
          <select aria-label="Assignee" className={selectClass} value={filters.assignee ?? ""} onChange={(e) => setParam("assignee", e.target.value)}>
            <option value="">All assignees</option>
            {bootstrap.data?.agents.map((a) => <option key={a.id} value={a.handle}>@{a.handle}</option>)}
          </select>
          <label className="flex items-center gap-1.5 text-xs text-fg-secondary">
            <input key={String(filters.include_canceled)} type="checkbox" defaultChecked={filters.include_canceled} onChange={(e) => setParam("include_canceled", e.target.checked ? "true" : "")} /> Show canceled
          </label>
          <Link href={`/tickets${project ? `?project=${encodeURIComponent(project.key)}` : ""}`} className="text-xs text-fg-secondary hover:text-fg">Tickets view ↗</Link>
        </>
      } />
      {bootstrap.isPending ? <p role="status" className="p-6 text-fg-secondary">Loading projects…</p>
        : bootstrap.isError ? <p role="alert" className="p-6">Couldn&apos;t load projects: {bootstrap.error.message}</p>
        : !project ? <p className="p-6 text-fg-secondary">{requested ? "Project unavailable. Choose an active project." : "Create a project to view its board."}</p>
        : <BoardView key={JSON.stringify([project.key, filters])} project={project.key} filters={filters} />}
    </div>
  );
}

function EpicHeader({ summary }: { summary: NonNullable<Board["epic"]> }) {
  const { epic } = summary;
  return (
    <section aria-label="Epic summary" className="mb-4 space-y-3 rounded-lg border border-border bg-surface p-4">
      <div className="flex flex-wrap items-center gap-2">
        <h1 className="text-[15px] font-semibold">{epic.name}</h1>
        <span className="rounded-full bg-accent-soft px-2 py-0.5 text-xs text-accent">{{ on_track: "On track", at_risk: "At risk", off_track: "Off track" }[epic.health]}</span>
      </div>
      {summary.objective && <p className="text-xs text-fg-secondary">{summary.objective}</p>}
      <EpicCompletion summary={summary} />
    </section>
  );
}

function BoardView({ project, filters }: { project: string; filters: BoardFilters }) {
  const query = useBoard(project, filters);
  const moves = useBoardMoves();
  const dragging = useRef<string | null>(null);
  const [over, setOver] = useState<BoardStatus>();
  const statuses = filters.include_canceled ? [...BOARD_STATUSES, "canceled" as const] : BOARD_STATUSES;
  const board = query.data && withBoardMoves(query.data, Object.values(moves.pending));
  return (
    <div className="p-5 max-sm:p-3">
      {moves.error && <div role="alert" className="mb-4 flex items-start justify-between gap-4 rounded-lg border border-state-blocked bg-surface p-3 text-sm">
        <span>{moves.error}</span><button type="button" aria-label="Dismiss move error" onClick={moves.dismissError}>×</button>
      </div>}
      <p role="status" className="sr-only">{moves.notice ?? (Object.keys(moves.pending).length ? "Moving ticket…" : "")}</p>
      {query.isPending && <p role="status" className="text-sm text-fg-secondary">Loading board…</p>}
      {query.isError && <div role="alert" className="mb-4 text-sm">Couldn&apos;t load board: {query.error.message} <button type="button" onClick={() => void query.refetch()} className="underline">Retry</button></div>}
      {board?.epic && <EpicHeader summary={board.epic} />}
      {board && <div aria-label="Board columns" className="flex min-h-[calc(100dvh-240px)] gap-2 overflow-x-auto pb-3">
        {statuses.map((status) => {
          const column = board.columns.find((c) => c.status === status);
          return <section key={status} aria-label={STATUS_NAME[status]} data-testid={`column-${status}`}
            className={cx("min-w-[220px] flex-1 rounded-lg border p-2", over === status ? "border-accent bg-accent-soft" : "border-transparent bg-surface-sunken")}
            onDragOver={(e) => { if (dragging.current) { e.preventDefault(); e.dataTransfer.dropEffect = "move"; setOver(status); } }}
            onDragLeave={(e) => { if (!e.currentTarget.contains(e.relatedTarget as Node | null)) setOver(undefined); }}
            onDrop={(e) => {
              e.preventDefault();
              const key = dragging.current;
              dragging.current = null;
              setOver(undefined);
              const card = query.data?.columns.flatMap((c) => c.cards).find((c) => c.key === key);
              if (card) void moves.move(card, status);
            }}>
            <header className="mb-3 flex items-center gap-2 px-1 pt-1 text-xs">
              <StatusGlyph state={status} label={false} /><h2 className="font-semibold">{STATUS_NAME[status]}</h2>
              <span className="ml-auto whitespace-nowrap text-fg-tertiary">{column?.count ?? 0} · {column?.points ?? 0} pts</span>
            </header>
            <div className="space-y-2">
              {column?.cards.map((card) => <TicketCard key={card.key} card={card} statuses={statuses} pending={!!moves.pending[card.key]}
                onMove={(target) => void moves.move(card, target)}
                onDragStart={() => { dragging.current = card.key; }}
                onDragEnd={() => { dragging.current = null; setOver(undefined); }} />)}
              {!column?.cards.length && <p className="p-1 text-xs text-fg-tertiary">No tickets</p>}
              {column?.truncated && <p className="px-1 py-2 text-xs text-fg-tertiary">Showing {column.cards.length} most recent of {column.count}</p>}
            </div>
          </section>;
        })}
      </div>}
    </div>
  );
}

function TicketCard({ card, statuses, pending, onMove, onDragStart, onDragEnd }: {
  card: BoardCard; statuses: BoardStatus[]; pending: boolean; onMove: (status: BoardStatus) => void; onDragStart: () => void; onDragEnd: () => void;
}) {
  const [menu, setMenu] = useState(false);
  const router = useRouter();
  const href = `/tickets/${encodeURIComponent(card.key)}`;
  return <article tabIndex={0} aria-label={`Ticket ${card.key}`} aria-busy={pending} draggable={!pending}
    onDragStart={(e) => { e.dataTransfer.setData("text/plain", card.key); e.dataTransfer.effectAllowed = "move"; onDragStart(); }} onDragEnd={onDragEnd}
    onClick={(e) => { if (!(e.target as HTMLElement).closest("a,button,[role=menu]")) router.push(href); }}
    onKeyDown={(e) => { if (e.target === e.currentTarget && (e.key === "Enter" || e.key === " ") && !pending) { e.preventDefault(); setMenu(true); } }}
    className={cx("rounded-md border border-border bg-surface p-3 shadow-sm outline-offset-2", pending ? "opacity-60" : "cursor-grab hover:border-border-strong")}>
    <div className="mb-2 flex items-start justify-between gap-1 text-[11px]">
      <Link href={href} className="font-mono text-fg-tertiary hover:text-fg">{card.key}</Link>
      {card.unmet_dependencies > 0 && <span className="rounded-full bg-accent-soft px-1.5 text-state-blocked" title={`${card.unmet_dependencies} open blockers`}>⛔ Blocked</span>}
    </div>
    <Link href={href} className="mb-3 block text-[13px] font-medium leading-snug hover:underline">{card.title}</Link>
    <div className="flex flex-wrap items-center gap-2 text-[11px] text-fg-secondary">
      <span className="flex items-center gap-1"><PriorityGlyph priority={card.priority} label={false} />{PRIORITY_NAME[card.priority]}</span>
      <span className={cx("rounded-full px-1.5 py-0.5", card.estimate_points == null ? "border border-dashed border-border-strong text-fg-tertiary" : "bg-accent-soft text-accent")}>{card.estimate_points == null ? "unestimated" : `${card.estimate_points} pt`}</span>
      <span aria-label={`${card.criteria_done} of ${card.criteria_total} criteria met`}>☑ {card.criteria_done}/{card.criteria_total}</span>
    </div>
    <div className="mt-2 flex items-center gap-1.5 text-[11px] text-fg-tertiary">
      {card.assignee ? <><AgentAvatar name={card.assignee.name} colorIndex={card.assignee.color_index} /><span className="truncate">@{card.assignee.handle}</span></> : <><UnassignedAvatar /><span>Unassigned</span></>}
      <DropdownMenu.Root open={menu} onOpenChange={setMenu}>
        <DropdownMenu.Trigger asChild><button type="button" disabled={pending} aria-label={`Move ${card.key} to…`} className="ml-auto rounded px-2 py-1 text-base leading-none hover:bg-hover disabled:opacity-50">⋯</button></DropdownMenu.Trigger>
        <DropdownMenu.Portal><DropdownMenu.Content aria-label={`Move ${card.key}`} align="end" sideOffset={4} className={cx(floatPanel, "w-44")} onClick={(e) => e.stopPropagation()}>
          <DropdownMenu.Label className="px-2 py-1 text-[11px] uppercase text-fg-tertiary">Move to…</DropdownMenu.Label>
          {statuses.map((status) => <DropdownMenu.Item key={status} disabled={status === card.status || pending} onSelect={() => onMove(status)} className="flex cursor-default items-center gap-2 rounded-md px-2 py-1.5 outline-none data-[highlighted]:bg-hover data-[disabled]:opacity-50">
            <StatusGlyph state={status} label={false} />{STATUS_NAME[status]}{status === card.status && <span className="ml-auto">✓</span>}
          </DropdownMenu.Item>)}
        </DropdownMenu.Content></DropdownMenu.Portal>
      </DropdownMenu.Root>
    </div>
  </article>;
}

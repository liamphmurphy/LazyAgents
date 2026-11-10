"use client";

import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { useEffect, useRef, useState, type MouseEvent } from "react";
import { DropdownMenu } from "radix-ui";
import { useBootstrap } from "@/lib/api/hooks";
import { useBoard, useBoardMoves, useTicketEditorDetail, type BoardFilters } from "@/lib/api/board-hooks";
import { BOARD_STATUSES, onlyBlocked, withBoardMoves, type Board, type BoardCard, type BoardStatus } from "@/lib/board";
import { cx } from "@/lib/cx";
import { TopBar } from "@/components/shell/topbar";
import { ProjectScopeState, projectCrumbs, useProjectScope } from "@/components/shell/project-scope";
import { AgentAvatar, UnassignedAvatar } from "@/components/domain/avatars";
import { PriorityGlyph, StatusGlyph } from "@/components/domain/glyphs";
import { PRIORITY_NAME, STATUS_NAME } from "@/components/domain/meta";
import { floatPanel } from "@/components/domain/pickers";
import { EpicCompletion } from "@/components/domain/epic-completion";
import { TicketEditorDialog } from "@/components/domain/ticket-editor-dialog";
import { DependencyMap } from "@/components/domain/dependency-map";
import { useMotionList } from "@/components/ui/motion";
import { Toggle } from "@/components/ui/controls";
import { Icon } from "@/components/ui/icon";

/** A filter in the top bar: a menu like the app's other pickers rather than a native <select>. */
function FilterMenu({ label, value, options, onChange }: { label: string; value: string; options: { value: string; label: string }[]; onChange: (v: string) => void }) {
  const current = options.find((o) => o.value === value) ?? options[0];
  return (
    <DropdownMenu.Root>
      <DropdownMenu.Trigger asChild>
        <button type="button" aria-label={`${label}: ${current.label}`}
          className="flex h-8 max-w-52 items-center gap-1.5 rounded-md border border-border-strong bg-surface px-2.5 text-xs text-fg hover:bg-hover data-[state=open]:border-accent">
          <span className="truncate">{current.label}</span>
          <Icon name="chev" size={12} strokeWidth={1.6} className="flex-none text-fg-tertiary" />
        </button>
      </DropdownMenu.Trigger>
      <DropdownMenu.Portal>
        <DropdownMenu.Content align="end" sideOffset={6} className={cx(floatPanel, "max-h-80 w-56 overflow-y-auto")}>
          <DropdownMenu.RadioGroup value={value} onValueChange={onChange}>
            {options.map((o) => (
              <DropdownMenu.RadioItem key={o.value} value={o.value} className="flex h-8 cursor-default select-none items-center gap-2 rounded-md px-2 text-[13px] outline-none data-[highlighted]:bg-hover">
                <span className="flex-1 truncate">{o.label}</span>
                <DropdownMenu.ItemIndicator><Icon name="check" size={14} className="text-accent" /></DropdownMenu.ItemIndicator>
              </DropdownMenu.RadioItem>
            ))}
          </DropdownMenu.RadioGroup>
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  );
}

export function BoardScreen() {
  const bootstrap = useBootstrap();
  const params = useSearchParams();
  // The sidebar switcher owns the project; ?project= wins, then the remembered one.
  const scope = useProjectScope("required");
  const project = scope.status === "ready" ? scope.project : undefined;
  const filters: BoardFilters = {
    epic: params.get("epic") || undefined,
    assignee: params.get("assignee") || undefined,
    include_canceled: params.get("include_canceled") === "true",
  };
  // Client-side: the board returns open columns complete, so filtering loses nothing.
  const blockedOnly = params.get("blocked") === "true";
  const epics = bootstrap.data?.epics.filter((e) => e.project_id === project?.id) ?? [];
  function setParam(name: string, value: string) {
    // These filters only fetch client data. Native history integrates with
    // useSearchParams without an asynchronous server navigation per input.
    const next = new URLSearchParams(window.location.search);
    if (project) next.set("project", project.key);
    if (value) next.set(name, value); else next.delete(name);
    window.history.replaceState(null, "", `/board?${next}`);
  }
  const map = params.get("tab") === "map";
  const ticketKey = params.get("ticket") || undefined;
  const editor = useTicketEditorDetail(ticketKey);
  // Only a failed first load closes the editor; a failed background refetch keeps the draft.
  const loadFailed = editor.isError && !editor.data;
  function setTicket(key?: string) {
    const next = new URLSearchParams(window.location.search);
    if (key) next.set("ticket", key); else next.delete("ticket");
    const qs = next.toString();
    window.history.replaceState(null, "", qs ? `/board?${qs}` : "/board");
  }
  return (
    <div data-testid="board-screen">
      <TopBar crumbs={projectCrumbs(project, "Board")} actions={project &&
        <>
          <FilterMenu label="Epic" value={filters.epic ?? ""} onChange={(v) => setParam("epic", v)}
            options={[{ value: "", label: "All epics" }, ...epics.map((e) => ({ value: e.id, label: e.name }))]} />
          <FilterMenu label="Assignee" value={filters.assignee ?? ""} onChange={(v) => setParam("assignee", v)}
            options={[{ value: "", label: "All assignees" }, ...(bootstrap.data?.agents ?? []).map((a) => ({ value: a.handle, label: `@${a.handle}` }))]} />
          <Toggle label="Show canceled" checked={filters.include_canceled} onChange={(on) => setParam("include_canceled", on ? "true" : "")} />
          <Toggle label="Blocked only" checked={blockedOnly} onChange={(on) => setParam("blocked", on ? "true" : "")} />
          <Link href={`/tickets${project ? `?project=${encodeURIComponent(project.key)}` : ""}`} className="text-xs text-fg-secondary hover:text-fg">Tickets view ↗</Link>
        </>
      } />
      {project && <nav aria-label="Board views" className="flex gap-5 border-b border-border px-5 py-3 text-sm">
        <button aria-current={!map ? "page" : undefined} className={!map ? "font-semibold text-accent" : ""} onClick={() => setParam("tab", "")}>Board</button>
        <button aria-current={map ? "page" : undefined} className={map ? "font-semibold text-accent" : ""} onClick={() => setParam("tab", "map")}>Map</button>
      </nav>}
      {!project ? <ProjectScopeState base="/board" scope={scope} />
        : map ? <div className="p-5"><DependencyMap key={JSON.stringify([project.key, filters])} project={project.key} epic={filters.epic} assignee={filters.assignee} includeCanceled={filters.include_canceled} onOpen={setTicket} /></div>
        : <BoardView key={JSON.stringify([project.key, filters])} project={project.key} filters={filters} blockedOnly={blockedOnly} onOpen={setTicket} />}
      {ticketKey && loadFailed && <div role="alert" className="mx-5 mb-4 flex items-start justify-between gap-4 rounded-lg border border-state-blocked bg-surface p-3 text-sm">
        <span>Couldn&apos;t open {ticketKey}: {editor.error.message}</span>
        <button type="button" aria-label="Dismiss ticket error" onClick={() => setTicket()}>×</button>
      </div>}
      {ticketKey && !loadFailed && bootstrap.data && <TicketEditorDialog key={ticketKey} ticketKey={ticketKey} detail={editor.data} bootstrap={bootstrap.data}
        onClose={() => setTicket()} onReload={async () => { const r = await editor.refetch(); return r.isError ? undefined : r.data; }} />}
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

function BoardView({ project, filters, blockedOnly, onOpen }: { project: string; filters: BoardFilters; blockedOnly: boolean; onOpen: (key: string) => void }) {
  const query = useBoard(project, filters);
  const moves = useBoardMoves();
  const dragging = useRef<string | null>(null);
  const columns = useRef<HTMLDivElement>(null);
  // Dropped cards are already under the pointer; only the others need to glide.
  const landing = useRef<string | null>(null);
  useMotionList(columns, landing);
  const [over, setOver] = useState<BoardStatus>();
  const statuses = filters.include_canceled ? [...BOARD_STATUSES, "canceled" as const] : BOARD_STATUSES;
  const moved = query.data && withBoardMoves(query.data, Object.values(moves.pending));
  const board = moved && (blockedOnly ? onlyBlocked(moved) : moved);
  return (
    <div className="p-5 max-sm:p-3">
      {moves.error && <div role="alert" className="mb-4 flex items-start justify-between gap-4 rounded-lg border border-state-blocked bg-surface p-3 text-sm">
        <span>{moves.error}</span><button type="button" aria-label="Dismiss move error" onClick={moves.dismissError}>×</button>
      </div>}
      {moves.conflict && <ConflictToast {...moves.conflict} onDismiss={moves.dismissConflict} />}
      <p role="status" className="sr-only">{moves.notice ?? (Object.keys(moves.pending).length ? "Moving ticket…" : "")}</p>
      {query.isPending && <p role="status" className="text-sm text-fg-secondary">Loading board…</p>}
      {query.isError && <div role="alert" className="mb-4 text-sm">Couldn&apos;t load board: {query.error.message} <button type="button" onClick={() => void query.refetch()} className="underline">Retry</button></div>}
      {board?.epic && <EpicHeader summary={board.epic} />}
      {board && <div ref={columns} aria-label="Board columns" className="relative flex min-h-[calc(100dvh-240px)] gap-2 overflow-x-auto pb-3">
        {statuses.map((status) => {
          const column = board.columns.find((c) => c.status === status);
          return <section key={status} aria-label={STATUS_NAME[status]} data-testid={`column-${status}`}
            className={cx("min-w-[220px] flex-1 rounded-lg border p-2 transition-colors duration-150", over === status ? "border-accent bg-accent-soft" : "border-transparent bg-surface-sunken")}
            onDragOver={(e) => { if (dragging.current) { e.preventDefault(); e.dataTransfer.dropEffect = "move"; setOver(status); } }}
            onDragLeave={(e) => { if (!e.currentTarget.contains(e.relatedTarget as Node | null)) setOver(undefined); }}
            onDrop={(e) => {
              e.preventDefault();
              const key = dragging.current;
              dragging.current = null;
              setOver(undefined);
              const card = query.data?.columns.flatMap((c) => c.cards).find((c) => c.key === key);
              if (card && card.status !== status) landing.current = card.key;
              if (card) void moves.move(card, status);
            }}>
            <header className="mb-3 flex items-center gap-2 px-1 pt-1 text-xs">
              <StatusGlyph state={status} label={false} /><h2 className="font-semibold">{STATUS_NAME[status]}</h2>
              <span className="ml-auto whitespace-nowrap text-fg-tertiary">{column?.count ?? 0} · {column?.points ?? 0} pts</span>
            </header>
            <div className="space-y-2">
              {column?.cards.map((card) => <TicketCard key={card.key} card={card} statuses={statuses} pending={!!moves.pending[card.key]} onOpen={() => onOpen(card.key)}
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

/** Stale-revision moves: the board already refetched, so this only explains why the card snapped back. */
function ConflictToast({ ticket, message, onDismiss }: { ticket: string; message: string; onDismiss: () => void }) {
  useEffect(() => {
    const timer = setTimeout(onDismiss, 6000);
    return () => clearTimeout(timer);
  }, [ticket, message, onDismiss]);
  return <div role="alert" aria-label="Move conflict" className="motion-rise fixed bottom-5 right-5 z-50 flex max-w-sm items-start gap-4 rounded-lg border border-border-float bg-surface-raised p-3 text-sm shadow-lg">
    <div>
      <p className="font-semibold">{ticket} changed elsewhere</p>
      <p className="text-xs text-fg-secondary">Board refreshed. Review it and try again. ({message})</p>
    </div>
    <button type="button" aria-label="Dismiss move conflict" onClick={onDismiss} className="text-base leading-none">×</button>
  </div>;
}

function TicketCard({ card, statuses, pending, onOpen, onMove, onDragStart, onDragEnd }: {
  card: BoardCard; statuses: BoardStatus[]; pending: boolean; onOpen: () => void; onMove: (status: BoardStatus) => void; onDragStart: () => void; onDragEnd: () => void;
}) {
  const [menu, setMenu] = useState(false);
  const [lifted, setLifted] = useState(false);
  const href = `/tickets/${encodeURIComponent(card.key)}`;
  // Plain clicks open the editor; modified clicks keep normal link behavior.
  const open = (e: MouseEvent) => { if (e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return; e.preventDefault(); onOpen(); };
  return <article tabIndex={0} data-ticket-key={card.key} data-motion-key={card.key} aria-label={`Ticket ${card.key}`} aria-busy={pending} draggable={!pending}
    onDragStart={(e) => { e.dataTransfer.setData("text/plain", card.key); e.dataTransfer.effectAllowed = "move"; onDragStart(); requestAnimationFrame(() => setLifted(true)); /* After the browser captures the drag image. */ }} onDragEnd={() => { setLifted(false); onDragEnd(); }}
    onClick={(e) => { if (!(e.target as HTMLElement).closest("a,button,[role=menu]")) onOpen(); }}
    onKeyDown={(e) => { if (e.target === e.currentTarget && !pending) {
      if (e.key === "Enter") { e.preventDefault(); onOpen(); } else if (e.key === " ") { e.preventDefault(); setMenu(true); }
    } }}
    className={cx("rounded-md border border-border bg-surface p-3 shadow-sm outline-offset-2 transition-[opacity,scale] duration-150", lifted && "scale-[0.98] opacity-40", pending ? "opacity-60" : "cursor-grab hover:border-border-strong")}>
    <div className="mb-2 flex items-start justify-between gap-1 text-[11px]">
      <Link href={href} onClick={open} className="font-mono text-fg-tertiary hover:text-fg">{card.key}</Link>
      {card.unmet_dependencies > 0 && <span className="rounded-full bg-accent-soft px-1.5 text-state-blocked" title={`${card.unmet_dependencies} open blockers`}>⛔ Blocked</span>}
    </div>
    <Link href={href} onClick={open} className="mb-3 block text-[13px] font-medium leading-snug hover:underline">{card.title}</Link>
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

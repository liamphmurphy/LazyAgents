"use client";

import Link from "next/link";
import { ViewTransition, type MouseEvent } from "react";
import type { Schemas, TicketSummary } from "@/lib/api/client";
import { cx } from "@/lib/cx";
import { shortAgo } from "@/lib/format";
import { isBlocked } from "@/lib/ticket-list";
import { Icon } from "@/components/ui/icon";
import { AgentAvatar, UnassignedAvatar } from "@/components/domain/avatars";
import { PriorityGlyph, StatusGlyph } from "@/components/domain/glyphs";
import { Flag, LabelChip } from "@/components/domain/labels";

export type Density = "comfortable" | "compact";

const PR_STATE: Record<Schemas["PullRequest"]["state"], { label: string; cls: string }> = {
  draft: { label: "Draft", cls: "text-fg-tertiary" },
  open: { label: "Open", cls: "text-pill-done-fg" },
  merged: { label: "Merged", cls: "text-accent" },
  closed: { label: "Closed", cls: "text-pill-blocked-fg" },
};

function PrIcon({ className }: { className?: string }) {
  return (
    <svg aria-hidden width="12" height="12" viewBox="0 0 12 12" fill="none" stroke="currentColor" strokeWidth="1.3" className={className}>
      <circle cx="3" cy="2.5" r="1.5" />
      <circle cx="3" cy="9.5" r="1.5" />
      <circle cx="9" cy="9.5" r="1.5" />
      <path d="M3 4v4M9 8V5.5Q9 2.5 6 2.5H5" />
    </svg>
  );
}

/** Criteria done/total with a small progress bar (green when complete). */
export function CriteriaProgress({ done, total, compact }: { done: number; total: number; compact?: boolean }) {
  if (total === 0) return <span className="text-fg-tertiary" aria-label="No criteria">–</span>;
  const pct = Math.round((done / total) * 100);
  return (
    <span className="flex items-center gap-2" title={`${done} of ${total} criteria done`}>
      <span className="font-mono text-xs text-fg-secondary">
        {done}/{total}
      </span>
      {!compact && (
        <span aria-hidden className="h-1 w-10 overflow-hidden rounded-full bg-border-strong">
          <span className={cx("block h-full rounded-full", done === total ? "bg-state-done" : "bg-accent")} style={{ width: `${pct}%` }} />
        </span>
      )}
    </span>
  );
}

/** Column widths shared by the header row and ticket rows. */
export const COL = {
  criteria: "w-[84px] flex-none",
  estimate: "w-10 flex-none text-center",
  pr: "hidden w-[96px] flex-none md:flex",
  epic: "hidden w-[120px] flex-none lg:flex",
  assignee: "w-[120px] flex-none",
  updated: "w-[44px] flex-none text-right",
};

/**
 * Ticket row (DESIGN.md §4): 44px comfortable, 32px compact (labels hidden).
 * The title is the link; the checkbox is a separate control that appears on
 * hover, focus or selection. `focused` is the j/k keyboard cursor.
 */
export function TicketRow({
  t,
  selected,
  focused,
  density,
  showEpic,
  onSelect,
  onFocusRow,
}: {
  t: TicketSummary;
  selected: boolean;
  focused: boolean;
  density: Density;
  showEpic: boolean;
  onSelect: (e: MouseEvent<HTMLInputElement>) => void;
  onFocusRow: () => void;
}) {
  const compact = density === "compact";
  const pr = t.pr ? PR_STATE[t.pr.state] : null;
  return (
    <div
      data-ticket-row={t.key}
      data-focused={focused || undefined}
      onMouseDown={onFocusRow}
      className={cx(
        "group relative flex items-center gap-3 border-b border-border-row pl-6 pr-6 hover:bg-row-hover",
        compact ? "h-8" : "h-11",
        selected && "bg-accent/8 hover:bg-accent/10",
        focused && "outline outline-1 -outline-offset-1 outline-accent/45 before:absolute before:inset-y-0 before:left-0 before:w-0.5 before:bg-accent",
      )}
    >
      <input
        type="checkbox"
        checked={selected}
        onClick={onSelect}
        onChange={() => {}}
        aria-label={`Select ${t.key}`}
        className={cx(
          "relative z-10 size-3.5 flex-none accent-[var(--accent)]",
          selected || focused ? "opacity-100" : "opacity-0 focus-visible:opacity-100 group-hover:opacity-100",
        )}
      />
      <PriorityGlyph priority={t.priority} />
      <span className="w-16 flex-none font-mono text-xs text-fg-tertiary">{t.key}</span>
      <StatusGlyph state={t.status} />
      <span className="flex min-w-0 flex-1 items-center gap-2">
        {/* Stretched link: the whole row opens the ticket. */}
        <Link href={`/tickets/${t.key}`} className="truncate font-medium text-fg after:absolute after:inset-0 hover:text-fg">
          <ViewTransition name={`ticket-title-${t.key}`} share="auto" default="none"><span>{t.title}</span></ViewTransition>
        </Link>
        {isBlocked(t) && <Flag tone="human">{compact ? "Blocked" : `Blocked · ${t.unmet_dependencies}`}</Flag>}
        {!compact && t.assignee && t.status === "todo" && <Flag tone="neutral">Assigned</Flag>}
      </span>
      {!compact && (
        <span className="hidden flex-none gap-1.5 xl:flex">
          {t.labels.slice(0, 3).map((l) => (
            <LabelChip key={l.id} name={l.name} color={l.color} />
          ))}
        </span>
      )}
      <span className={COL.criteria}>
        <CriteriaProgress done={t.criteria_done} total={t.criteria_total} compact={compact} />
      </span>
      <span className={COL.estimate}>
        {t.estimate_points != null ? (
          <span className="inline-grid h-[18px] min-w-6 place-items-center rounded-full bg-border-strong px-1.5 font-mono text-[11.5px] text-fg-2" title={`${t.estimate_points} points`}>
            {t.estimate_points}
          </span>
        ) : (
          <span className="font-mono text-[11.5px] text-fg-tertiary" title="Not estimated">–</span>
        )}
      </span>
      <span className={cx(COL.pr, "items-center gap-1.5 text-xs")}>
        {t.pr && pr ? (
          <>
            <PrIcon className={pr.cls} />
            <span className="font-mono text-fg-secondary">#{t.pr.number}</span>
            <span className={pr.cls}>{t.pr.closed_unmerged ? "Closed" : pr.label}</span>
          </>
        ) : (
          <span className="text-fg-tertiary" aria-label="No pull request">—</span>
        )}
      </span>
      {showEpic && (
        <span className={cx(COL.epic, "items-center gap-1.5 truncate text-[12.5px] text-fg-secondary")}>
          {t.epic ? (
            <>
              <Icon name="epics" size={13} className="text-fg-tertiary" />
              <span className="truncate">{t.epic.name}</span>
            </>
          ) : (
            <span className="text-fg-tertiary">Unscoped</span>
          )}
        </span>
      )}
      <span className={cx(COL.assignee, "flex items-center gap-2 text-[13px]")}>
        {t.assignee ? (
          <>
            <AgentAvatar name={t.assignee.name} colorIndex={t.assignee.color_index} label={false} />
            <span className={cx("truncate", compact && "sr-only")}>{t.assignee.name}</span>
          </>
        ) : (
          <>
            <UnassignedAvatar />
            <span className={cx("text-fg-tertiary", compact && "sr-only")}>Unassigned</span>
          </>
        )}
      </span>
      <span className={cx(COL.updated, "text-xs text-fg-tertiary")} title={new Date(t.updated_at).toLocaleString()}>
        {shortAgo(t.updated_at)}
      </span>
    </div>
  );
}

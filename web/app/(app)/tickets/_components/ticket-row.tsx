"use client";

import Link from "next/link";
import type { MouseEvent } from "react";
import type { TicketSummary } from "@/lib/api/client";
import { cx } from "@/lib/cx";
import { shortAgo } from "@/lib/format";
import { Icon } from "@/components/ui/icon";
import { AgentAvatar, UnassignedAvatar } from "@/components/domain/avatars";
import { PriorityGlyph, StatusGlyph } from "@/components/domain/glyphs";
import { Flag, LabelChip } from "@/components/domain/labels";

/**
 * 44px ticket row (DESIGN.md §4). The title is the link; the checkbox is a
 * separate control that appears on hover, focus or selection.
 */
export function TicketRow({
  t,
  selected,
  onSelect,
}: {
  t: TicketSummary;
  selected: boolean;
  onSelect: (e: MouseEvent<HTMLInputElement>) => void;
}) {
  return (
    <div
      className={cx(
        "group relative flex h-11 items-center gap-3 border-b border-border-row pl-6 pr-6 hover:bg-row-hover",
        selected && "bg-accent/8 hover:bg-accent/10",
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
          selected ? "opacity-100" : "opacity-0 focus-visible:opacity-100 group-hover:opacity-100",
        )}
      />
      <PriorityGlyph priority={t.priority} />
      <span className="w-16 flex-none font-mono text-xs text-fg-tertiary">{t.key}</span>
      <StatusGlyph state={t.status} />
      <span className="flex min-w-0 flex-1 items-center gap-2">
        {/* Stretched link: the whole row opens the ticket. */}
        <Link href={`/tickets/${t.key}`} className="truncate font-medium text-fg after:absolute after:inset-0 hover:text-fg">
          {t.title}
        </Link>
        {t.pr && t.status === "in_review" && <Flag tone="review">PR #{t.pr.number}</Flag>}
        {t.assignee && t.status === "todo" && <Flag tone="neutral">Assigned</Flag>}
      </span>
      <span className="hidden flex-none gap-1.5 lg:flex">
        {t.labels.slice(0, 3).map((l) => (
          <LabelChip key={l.id} name={l.name} color={l.color} />
        ))}
      </span>
      <span className="hidden w-[120px] flex-none items-center gap-1.5 truncate text-[12.5px] text-fg-secondary md:flex">
        {t.epic ? (
          <>
            <Icon name="epics" size={13} className="text-fg-tertiary" />
            <span className="truncate">{t.epic.name}</span>
          </>
        ) : (
          <span className="text-fg-tertiary">Unscoped</span>
        )}
      </span>
      <span className="flex w-[108px] flex-none items-center gap-2 text-[13px]">
        {t.assignee ? (
          <>
            <AgentAvatar name={t.assignee.name} colorIndex={t.assignee.color_index} label={false} />
            <span className="truncate">{t.assignee.name}</span>
          </>
        ) : (
          <>
            <UnassignedAvatar />
            <span className="text-fg-tertiary">Unassigned</span>
          </>
        )}
      </span>
      <span className="w-[52px] flex-none text-right text-xs text-fg-tertiary" title={new Date(t.updated_at).toLocaleString()}>
        {shortAgo(t.updated_at)}
      </span>
    </div>
  );
}

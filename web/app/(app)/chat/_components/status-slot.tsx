"use client";

import { useState, type ReactNode } from "react";
import { splitStatus, type StatusEntry } from "@/lib/chat-status-slot";
import { cx } from "@/lib/cx";

export interface StatusItem extends StatusEntry {
  /** A few words naming it in the "+N more" row. */
  label: string;
  node: ReactNode;
  /**
   * False keeps the node mounted but out of the slot: a tray that renders nothing until it has
   * something still sees its first item arrive (the ticket-run hand-off depends on that).
   */
  present?: boolean;
}

/**
 * The banners and trays above the composer, in one place: the most important
 * shows in full, the rest sit behind "+N more" (see STATUS_PRIORITY). Hidden
 * items stay mounted so a tray keeps its state; a hidden error stays in the
 * accessibility tree (visually hidden) so its role="alert" is still announced.
 */
export function StatusSlot({ items }: { items: StatusItem[] }) {
  const [open, setOpen] = useState(false);
  // Focus inside an item pins it, so it never collapses while someone is using it.
  const [focused, setFocused] = useState<string | null>(null);
  const present = items.filter((i) => i.present !== false);
  const entries = present.map((i) => (i.key === focused ? { ...i, pinned: true } : i));
  // Whether there is anything to expand ignores focus, so focusing an item doesn't pull the "Show less" row out from under the pointer.
  const expanded = open && splitStatus(present, false).hidden.length > 0;
  const { hidden } = splitStatus(entries, expanded);
  const extra = hidden.length;
  const hiddenKeys = new Set(hidden.map((i) => i.key));
  // Every item renders in priority order, shown or not, so expanding doesn't remount anything; absent ones go last.
  const all = [...splitStatus(entries, true).shown, ...items.filter((i) => i.present === false)];
  return (
    <div data-testid="status-slot" className={cx(entries.length === 0 ? "hidden" : "flex flex-col gap-2", expanded && "max-md:max-h-[40dvh] max-md:overflow-y-auto")}>
      {all.map((item) => {
        const absent = item.present === false;
        const off = absent || hiddenKeys.has(item.key);
        return (
          <div
            key={item.key}
            data-testid={`status-item-${item.key}`}
            data-kind={absent ? undefined : item.kind}
            hidden={off && item.kind !== "error"}
            className={off && item.kind === "error" ? "sr-only" : undefined}
            onFocus={() => setFocused(item.key)}
            onBlur={(e) => {
              if (!e.currentTarget.contains(e.relatedTarget as Node | null)) setFocused((k) => (k === item.key ? null : k));
            }}
          >
            {item.node}
          </div>
        );
      })}
      {(expanded || extra > 0) && (
        <button
          type="button"
          data-testid="status-slot-more"
          aria-expanded={expanded}
          onClick={() => setOpen(!expanded)}
          className="flex h-7 w-full items-center gap-1.5 rounded-lg bg-surface-sunken px-3 text-left text-xs text-fg-secondary hover:bg-hover"
        >
          {expanded ? (
            <span className="font-semibold text-fg">Show less</span>
          ) : (
            <>
              <span className="flex-none font-semibold text-fg">+{extra} more</span>
              <span className="min-w-0 truncate">· {hidden.map((i) => i.label).join(" · ")}</span>
            </>
          )}
          <span aria-hidden className="ml-auto flex-none">{expanded ? "▴" : "▾"}</span>
        </button>
      )}
    </div>
  );
}

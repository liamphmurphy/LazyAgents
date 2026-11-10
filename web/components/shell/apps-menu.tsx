"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { Popover } from "radix-ui";
import { useState, useSyncExternalStore, type CSSProperties, type KeyboardEvent } from "react";
import { cx } from "@/lib/cx";
import { APP_ITEMS, TOOL_ITEMS, inAppsMenu, isActive, type NavBadge, type NavItem } from "@/lib/project-scope";
import { Icon } from "@/components/ui/icon";

const COLUMNS = 4;
const WIDE = "(min-width: 800px)";

function subscribeWide(onChange: () => void) {
  const mq = window.matchMedia(WIDE);
  mq.addEventListener("change", onChange);
  return () => mq.removeEventListener("change", onChange);
}

/** Below 800px the expanded menu fills the screen, with no room to its right: the grid opens under the row. */
function useWide(): boolean {
  return useSyncExternalStore(subscribeWide, () => window.matchMedia(WIDE).matches, () => true);
}

/** Arrow keys move through the grid as it reads: across a row, or a row up or down. */
function moveInGrid(e: KeyboardEvent<HTMLElement>) {
  const step = { ArrowRight: 1, ArrowLeft: -1, ArrowDown: COLUMNS, ArrowUp: -COLUMNS }[e.key];
  if (!step) return;
  const tiles = [...e.currentTarget.querySelectorAll<HTMLElement>("[data-tile]")];
  const at = tiles.indexOf(document.activeElement as HTMLElement);
  if (at < 0) return;
  e.preventDefault();
  tiles[Math.max(0, Math.min(tiles.length - 1, at + step))]?.focus();
}

function Tile({ item, index, count, current, onPick }: { item: NavItem; index: number; count: number; current: boolean; onPick: () => void }) {
  return (
    <Link
      href={item.href}
      data-tile
      onClick={onPick}
      aria-current={current ? "page" : undefined}
      aria-label={count > 0 ? `${item.label}, ${count} need${count === 1 ? "s" : ""} you` : undefined}
      style={{ "--i": index } as CSSProperties}
      className={cx(
        "motion-tile relative flex h-[70px] flex-col items-center justify-center gap-1.5 rounded-[10px] border text-xs outline-none focus-visible:border-accent",
        current ? "border-accent/60 bg-selected text-fg" : "border-border bg-surface text-fg-secondary hover:bg-hover hover:text-fg",
      )}
    >
      <Icon name={item.icon} size={17} />
      <span className="max-w-full truncate px-1">{item.label}</span>
      {count > 0 && (
        <span aria-hidden className="absolute right-1.5 top-1.5 grid h-4 min-w-4 place-items-center rounded-full bg-state-human px-1 text-[10px] font-semibold text-approve-fg">
          {count}
        </span>
      )}
    </Link>
  );
}

function Grid({ counts, onPick, onNewSpace }: { counts: Record<NavBadge, number>; onPick: () => void; onNewSpace: () => void }) {
  const pathname = usePathname();
  const section = (label: string, items: NavItem[], offset: number) => (
    <div role="group" aria-label={label} className="flex flex-col gap-2">
      <span aria-hidden className="text-[11px] font-semibold uppercase tracking-[0.04em] text-fg-tertiary">{label}</span>
      <div className="grid grid-cols-4 gap-2">
        {items.map((item, i) => (
          <Tile key={item.href} item={item} index={offset + i} count={item.badge ? counts[item.badge] : 0} current={isActive(pathname, item.href)} onPick={onPick} />
        ))}
      </div>
    </div>
  );
  return (
    <div onKeyDown={moveInGrid} className="flex flex-col gap-3.5">
      {section("Apps", APP_ITEMS, 0)}
      {section("Tools", TOOL_ITEMS, APP_ITEMS.length)}
      <div className="flex items-center justify-between border-t border-border pt-2.5">
        <button
          type="button"
          onClick={() => { onPick(); onNewSpace(); }}
          className="flex h-7 items-center gap-1.5 rounded-md px-1.5 text-[13px] font-medium text-accent hover:bg-hover"
        >
          <Icon name="plus" size={13} />
          New space
        </button>
        <span aria-hidden className="text-[11px] text-fg-tertiary">Arrow keys move · Esc closes</span>
      </div>
    </div>
  );
}

/**
 * The menu's Apps row: Docs, Finance and Library, then the tools (Workflows,
 * Scheduled, Learn, Wiki), in a grid that opens to the right. Opens on click,
 * not hover: the row sits over Spaces, and a hover flyout would open on the way there.
 */
export function AppsMenu({ rail, counts, onNewSpace }: { rail?: boolean; counts: Record<NavBadge, number>; onNewSpace: () => void }) {
  const pathname = usePathname();
  const wide = useWide();
  const [open, setOpen] = useState(false);
  const current = inAppsMenu(pathname);
  const waiting = TOOL_ITEMS.some((i) => i.badge && counts[i.badge] > 0);
  const label = waiting ? "Apps, something needs you" : "Apps";
  const close = () => setOpen(false);

  const row = (
    <button
      type="button"
      aria-label={rail || waiting ? label : undefined}
      title={rail ? "Apps" : undefined}
      aria-expanded={open}
      aria-current={current && !open ? "page" : undefined}
      onClick={rail || wide ? undefined : () => setOpen(!open)}
      className={cx(
        "relative flex h-[30px] w-full items-center gap-2.5 rounded-sm text-[13px] font-medium",
        rail ? "justify-center px-0" : "px-2",
        open || current ? "bg-selected text-fg" : "text-fg-secondary hover:bg-hover hover:text-fg",
      )}
    >
      <Icon name="dashboard" />
      {!rail && <span className="flex-1 text-left">Apps</span>}
      {waiting && <span aria-hidden className={cx("size-1.5 rounded-full bg-state-human", rail && "absolute right-1 top-1")} />}
      {!rail && <Icon name="chevr" size={12} className={cx("text-fg-tertiary", !wide && open && "rotate-90")} />}
    </button>
  );

  if (!rail && !wide) {
    return (
      <div className="flex flex-col gap-2">
        {row}
        {open && (
          <div className="motion-flyout rounded-xl border border-border-float bg-surface-raised p-3.5" data-state="open">
            <Grid counts={counts} onPick={close} onNewSpace={onNewSpace} />
          </div>
        )}
      </div>
    );
  }
  return (
    <Popover.Root open={open} onOpenChange={setOpen}>
      <Popover.Trigger asChild>{row}</Popover.Trigger>
      <Popover.Portal>
        <Popover.Content
          side="right"
          align="start"
          sideOffset={rail ? 12 : 16}
          alignOffset={-12}
          collisionPadding={12}
          aria-label="Apps"
          // The current page's tile, else the first, takes focus so arrows work straight away.
          onOpenAutoFocus={(e) => {
            e.preventDefault();
            const content = e.currentTarget as HTMLElement;
            (content.querySelector<HTMLElement>("[data-tile][aria-current]") ?? content.querySelector<HTMLElement>("[data-tile]"))?.focus();
          }}
          className="motion-flyout z-50 w-[336px] rounded-[14px] border border-border-float bg-surface-raised p-3.5 shadow-[var(--shadow-float)] outline-none"
        >
          <Grid counts={counts} onPick={close} onNewSpace={onNewSpace} />
        </Popover.Content>
      </Popover.Portal>
    </Popover.Root>
  );
}

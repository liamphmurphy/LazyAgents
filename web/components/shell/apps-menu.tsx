"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { Popover } from "radix-ui";
import { useState, type CSSProperties, type KeyboardEvent } from "react";
import { cx } from "@/lib/cx";
import { useMediaQuery } from "@/lib/viewport";
import { APP_ITEMS, TOOL_ITEMS, inAppsMenu, isActive, type NavBadge, type NavItem } from "@/lib/project-scope";
import { Icon } from "@/components/ui/icon";

/**
 * Below 800px the expanded menu fills the screen, with no room to its right: the grid opens under the row.
 * This follows the expanded sidebar's 800px rule, not COMPACT.
 */
function useWide(): boolean {
  return useMediaQuery("(min-width: 800px)", true);
}

/**
 * Arrow keys move through the menu as it reads: left and right step along the
 * tiles, up and down jump between the Apps row and the Tools row, keeping the
 * column (the Tools row is shorter, so it clamps to its last pill).
 */
function moveInGrid(e: KeyboardEvent<HTMLElement>) {
  if (!["ArrowRight", "ArrowLeft", "ArrowDown", "ArrowUp"].includes(e.key)) return;
  const tiles = [...e.currentTarget.querySelectorAll<HTMLElement>("[data-tile]")];
  const at = tiles.indexOf(document.activeElement as HTMLElement);
  if (at < 0) return;
  e.preventDefault();
  if (e.key === "ArrowRight" || e.key === "ArrowLeft") {
    tiles[Math.max(0, Math.min(tiles.length - 1, at + (e.key === "ArrowRight" ? 1 : -1)))]?.focus();
    return;
  }
  const inTools = tiles[at].dataset.tile === "tool";
  if ((e.key === "ArrowDown") === inTools) return; // already on the edge row
  const column = inTools ? at - APP_ITEMS.length : at;
  const target = inTools ? tiles.slice(0, APP_ITEMS.length) : tiles.slice(APP_ITEMS.length);
  target[Math.min(column, target.length - 1)]?.focus();
}

function Badge({ count, className }: { count: number; className: string }) {
  return (
    <span aria-hidden className={cx("grid h-4 min-w-4 place-items-center rounded-full bg-state-human px-1 text-[10px] font-semibold text-approve-fg", className)}>
      {count}
    </span>
  );
}

function tileProps(item: NavItem, kind: "app" | "tool", index: number, count: number, current: boolean, onPick: () => void) {
  return {
    href: item.href,
    "data-tile": kind,
    onClick: onPick,
    "aria-current": current ? ("page" as const) : undefined,
    "aria-label": count > 0 ? `${item.label}, ${count} need${count === 1 ? "s" : ""} you` : undefined,
    style: { "--i": index } as CSSProperties,
  };
}

const TILE_STATE = (current: boolean) =>
  current ? "border-accent/60 bg-selected text-fg" : "border-border bg-surface text-fg-secondary hover:bg-hover hover:text-fg";

type TileArgs = { item: NavItem; index: number; count: number; current: boolean; onPick: () => void };

function AppTile({ item, index, count, current, onPick }: TileArgs) {
  return (
    <Link
      {...tileProps(item, "app", index, count, current, onPick)}
      className={cx("motion-tile relative flex h-[70px] flex-col items-center justify-center gap-1.5 rounded-[10px] border text-xs outline-none focus-visible:border-accent", TILE_STATE(current))}
    >
      <Icon name={item.icon} size={17} />
      <span className="max-w-full truncate px-1">{item.label}</span>
      {count > 0 && <Badge count={count} className="absolute right-1.5 top-1.5" />}
    </Link>
  );
}

/** Tools are visited now and then, so they get a quieter pill than the apps' tiles. */
function ToolPill({ item, index, count, current, onPick }: TileArgs) {
  return (
    <Link
      {...tileProps(item, "tool", index, count, current, onPick)}
      className={cx("motion-tile flex h-8 flex-auto items-center justify-center gap-1.5 whitespace-nowrap rounded-lg border px-2.5 text-xs outline-none focus-visible:border-accent", TILE_STATE(current))}
    >
      <Icon name={item.icon} size={14} />
      <span>{item.label}</span>
      {count > 0 && <Badge count={count} className="" />}
    </Link>
  );
}

function Grid({ counts, onPick, onNewSpace }: { counts: Record<NavBadge, number>; onPick: () => void; onNewSpace: () => void }) {
  const pathname = usePathname();
  const countOf = (item: NavItem) => (item.badge ? counts[item.badge] : 0);
  return (
    <div onKeyDown={moveInGrid} className="flex flex-col gap-3.5">
      <div role="group" aria-label="Apps" className="flex flex-col gap-2">
        <span aria-hidden className="text-[11px] font-semibold uppercase tracking-[0.04em] text-fg-tertiary">Apps</span>
        <div className="grid grid-cols-3 gap-2 min-[480px]:grid-cols-5">
          {APP_ITEMS.map((item, i) => (
            <AppTile key={item.href} item={item} index={i} count={countOf(item)} current={isActive(pathname, item.href)} onPick={onPick} />
          ))}
        </div>
      </div>
      <div role="group" aria-label="Tools" className="flex flex-col gap-2">
        <span aria-hidden className="text-[11px] font-semibold uppercase tracking-[0.04em] text-fg-tertiary">Tools</span>
        <div className="flex flex-wrap gap-2">
          {TOOL_ITEMS.map((item, i) => (
            <ToolPill key={item.href} item={item} index={APP_ITEMS.length + i} count={countOf(item)} current={isActive(pathname, item.href)} onPick={onPick} />
          ))}
        </div>
      </div>
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
 * The menu's Apps row: apps as a row of five tiles, then the tools (Workflows,
 * Scheduled, Learn, Wiki) as pills, in a card that opens to the right. Opens on click,
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
          className="motion-flyout z-50 w-[420px] rounded-[14px] border border-border-float bg-surface-raised p-3.5 shadow-[var(--shadow-float)] outline-none"
        >
          <Grid counts={counts} onPick={close} onNewSpace={onNewSpace} />
        </Popover.Content>
      </Popover.Portal>
    </Popover.Root>
  );
}

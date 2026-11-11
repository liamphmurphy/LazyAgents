"use client";

import { DropdownMenu } from "radix-ui";
import type { ReactNode } from "react";
import { cx } from "@/lib/cx";
import { THEME_COLORS, resolveColor, type ColorRef, type Theme } from "@/lib/slides/schema";
import { floatPanel } from "@/components/domain/pickers";

export const chip = "inline-flex h-7 items-center gap-1 rounded-md border border-border-strong px-2 text-[12.5px] text-fg-2 outline-none hover:bg-hover focus-visible:ring-2 focus-visible:ring-accent disabled:opacity-40 data-[state=open]:bg-hover";
export const on = "border-accent bg-selected text-fg";
export const item = "flex cursor-pointer select-none items-center gap-2 rounded-md px-2.5 py-1.5 text-[13px] text-fg outline-none data-[highlighted]:bg-hover";

/** Keeps focus where it is (the text being edited) when a button is pressed. */
export const keep = (e: React.MouseEvent) => e.preventDefault();

export function Menu({ label, trigger, children, disabled, className }: { label: string; trigger: ReactNode; children: ReactNode; disabled?: boolean; className?: string }) {
  return (
    <DropdownMenu.Root modal={false}>
      <DropdownMenu.Trigger asChild disabled={disabled}>
        <button type="button" aria-label={label} className={chip} onMouseDown={keep}>
          {trigger}
        </button>
      </DropdownMenu.Trigger>
      <DropdownMenu.Portal>
        <DropdownMenu.Content align="start" sideOffset={6} className={cx(floatPanel, "z-50 max-h-[70vh] min-w-40 overflow-y-auto", className)} onCloseAutoFocus={(e) => e.preventDefault()}>
          {children}
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  );
}

export const colorName = (c: ColorRef) => (c.startsWith("#") ? c.toUpperCase() : c === "accent2" ? "Accent 2" : c[0].toUpperCase() + c.slice(1));

export function Swatches({ theme, onPick, none, current }: { theme: Theme; onPick: (c: ColorRef | undefined) => void; none?: string; current?: ColorRef | null }) {
  return (
    <>
      {none && (
        <DropdownMenu.Item className={cx(item, current === null && "font-semibold")} onSelect={() => onPick(undefined)}>
          <span className="size-4 rounded-sm border border-dashed border-border-strong" /> {none}
          {current === null && " ✓"}
        </DropdownMenu.Item>
      )}
      {THEME_COLORS.map((c) => (
        <DropdownMenu.Item key={c} className={cx(item, current === c && "font-semibold")} onSelect={() => onPick(c)}>
          <span className="size-4 rounded-sm border border-border-strong" style={{ background: resolveColor(theme, c) }} /> {colorName(c)}
          {current === c && " ✓"}
        </DropdownMenu.Item>
      ))}
    </>
  );
}

/** Keys typed into an input inside a menu stay there (no typeahead or arrow navigation). */
export const ownKeys = (e: React.KeyboardEvent) => {
  if (e.key !== "Escape") e.stopPropagation();
};

"use client";

import { Dialog } from "radix-ui";
import type { ReactNode } from "react";
import { cx } from "@/lib/cx";
import { Icon } from "./icon";

/**
 * A bottom sheet (DESIGN.md §3 Responsive, compact shell): a modal panel that slides up from the
 * bottom edge over a scrim, with a title row, padded for the home indicator. Compact screens use it
 * where wide ones open a popover or menu.
 */
export function Sheet({
  open,
  onOpenChange,
  title,
  children,
  className,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: ReactNode;
  children: ReactNode;
  className?: string;
}) {
  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Portal>
        <Dialog.Overlay className="motion-scrim fixed inset-0 z-40 bg-scrim" />
        <Dialog.Content
          aria-describedby={undefined}
          className={cx(
            "motion-sheet fixed inset-x-0 bottom-0 z-50 flex max-h-[85dvh] flex-col rounded-t-2xl border-t border-border-float bg-surface-raised px-safe pb-safe text-13 text-fg shadow-[var(--shadow-float)] outline-none",
            className,
          )}
        >
          <span aria-hidden className="mx-auto mt-2 h-1 w-9 flex-none rounded-full bg-border-strong" />
          <div className="flex flex-none items-center gap-2 px-4 pb-2 pt-1.5">
            <Dialog.Title className="flex-1 type-heading">{title}</Dialog.Title>
            <Dialog.Close aria-label="Close" className="grid size-[44px] place-items-center rounded-full text-fg-secondary hover:bg-hover">
              <Icon name="x" size={16} />
            </Dialog.Close>
          </div>
          <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain px-2 pb-3">{children}</div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}

export type SheetOption<T extends string> = { value: T; label: string; icon?: ReactNode; detail?: ReactNode };

/**
 * A sheet's choices: 48px rows with the current one checked (aria-current). Choosing one reports it;
 * the caller closes the sheet.
 */
export function SheetOptions<T extends string>({
  label,
  options,
  value,
  onPick,
}: {
  label: string;
  options: SheetOption<T>[];
  value?: T | null;
  onPick: (value: T) => void;
}) {
  return (
    <ul aria-label={label} className="flex flex-col">
      {options.map((o) => {
        const on = o.value === value;
        return (
          <li key={o.value}>
            <button
              type="button"
              aria-current={on || undefined}
              onClick={() => onPick(o.value)}
              className={cx("flex min-h-[48px] w-full items-center gap-3 rounded-lg px-3 text-left text-15 hover:bg-hover", on && "bg-selected")}
            >
              {o.icon}
              <span className="flex min-w-0 flex-1 flex-col leading-tight">
                <span className="truncate">{o.label}</span>
                {o.detail && <span className="truncate text-12 text-fg-tertiary">{o.detail}</span>}
              </span>
              {on && <Icon name="check" size={16} strokeWidth={1.8} className="flex-none text-accent" />}
            </button>
          </li>
        );
      })}
    </ul>
  );
}

"use client";

import { useState, type ReactNode } from "react";
import { Dialog } from "radix-ui";
import type { ComposerOption } from "@/lib/composer-options";
import { cx } from "@/lib/cx";
import { Icon } from "@/components/ui/icon";
import { composerIconButton } from "./reply-tools";

type SheetCounts = { context: number; attach: number; code?: number };

/** How far the sheet has to be dragged down by its grabber before letting go closes it. */
const SWIPE_CLOSE = 64;

/** A row's current value, as the sheet shows it under the label. */
function shownValue(o: ComposerOption, counts: SheetCounts): string | undefined {
  if (o.opens === "toggle") return o.on ? "On" : "Off";
  if (o.id === "context") return counts.context ? `${counts.context} added` : "None";
  if (o.id === "code") return counts.code ? `${counts.code} ${counts.code === 1 ? "snippet" : "snippets"}` : "None";
  if (o.id === "attach") return counts.attach ? `${counts.attach} ${counts.attach === 1 ? "file" : "files"}` : "None";
  return o.value ?? "None";
}

function RowIcon({ icon }: { icon: ComposerOption["icon"] }) {
  if (icon !== "paperclip") return <Icon name={icon} size={18} />;
  return (
    <svg aria-hidden viewBox="0 0 16 16" width={18} height={18} fill="none" stroke="currentColor" strokeWidth={1.5} strokeLinecap="round" strokeLinejoin="round">
      <path d="m13 7.5-4.8 4.8a3 3 0 0 1-4.2-4.2l5-5a2 2 0 0 1 2.8 2.8L6.8 10.9a1 1 0 0 1-1.4-1.4L9.7 5.2" />
    </svg>
  );
}

/**
 * The compact composer's + button and the bottom sheet it opens ("3 Composer + sheet"): one 56px
 * row per composer option, in composerOptions order, with its current value. Each row holds the
 * option's own control, the one the desktop row shows, and that control's trigger is stretched
 * over the row so a tap anywhere on it opens the same picker (or flips the same toggle).
 * Closes on Escape, a scrim tap, or a swipe down from the grabber; padded for the bottom safe area.
 */
export function ComposerSheet({
  open,
  onOpenChange,
  options,
  control,
  counts,
  disabled,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  options: ComposerOption[];
  /** The option's control, as the desktop composer renders it. */
  control: (o: ComposerOption) => ReactNode;
  counts: SheetCounts;
  disabled?: boolean;
}) {
  // Where the drag started, while the grabber is held, and how far down the sheet follows it.
  const [drag, setDrag] = useState<number | null>(null);
  const [dy, setDy] = useState(0);
  const endDrag = (close: boolean) => {
    setDrag(null);
    setDy(0);
    if (close) onOpenChange(false);
  };
  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Trigger asChild>
        <button type="button" aria-label="Composer options" title="Context, files and options" disabled={disabled} className={composerIconButton}>
          <Icon name="plus" size={15} strokeWidth={1.7} />
        </button>
      </Dialog.Trigger>
      <Dialog.Portal>
        <Dialog.Overlay data-testid="composer-sheet-scrim" className="motion-scrim fixed inset-0 z-40 bg-scrim" />
        <Dialog.Content
          aria-describedby={undefined}
          data-testid="composer-sheet"
          style={dy ? { transform: `translateY(${dy}px)` } : undefined}
          className={cx(
            "fixed inset-x-0 bottom-0 z-50 flex max-h-[85dvh] flex-col rounded-t-2xl border-t border-border-float bg-surface-raised pb-safe shadow-[var(--shadow-float)] outline-none",
            drag === null && "transition-transform duration-200",
          )}
        >
          <div
            data-testid="sheet-grabber"
            className="flex h-7 flex-none cursor-grab touch-none items-center justify-center"
            onPointerDown={(e) => {
              setDrag(e.clientY);
              e.currentTarget.setPointerCapture(e.pointerId);
            }}
            onPointerMove={(e) => {
              if (drag !== null) setDy(Math.max(0, e.clientY - drag));
            }}
            onPointerUp={(e) => drag !== null && endDrag(e.clientY - drag >= SWIPE_CLOSE)}
            onPointerCancel={() => endDrag(false)}
          >
            <span aria-hidden className="h-1 w-9 rounded-full bg-border-strong" />
          </div>
          <Dialog.Title className="px-4 pb-1 text-13 font-semibold text-fg-secondary">Options</Dialog.Title>
          <ul className="flex min-h-0 flex-col overflow-y-auto pb-2">
            {options.map((o) => {
              const value = shownValue(o, counts);
              return (
                <li
                  key={o.id}
                  data-testid="sheet-option"
                  data-option={o.id}
                  className={cx(
                    "relative flex min-h-[56px] flex-none items-center gap-3 px-4 active:bg-hover",
                    // Stretch the control's trigger over the row; a segmented choice keeps its own buttons.
                    o.opens !== "segmented" && "[&_.relative]:static [&_button]:after:absolute [&_button]:after:inset-0 [&_button]:after:content-['']",
                  )}
                >
                  <span className="flex-none text-fg-secondary">
                    <RowIcon icon={o.icon} />
                  </span>
                  <span className="flex min-w-0 flex-1 flex-col">
                    <span className="text-15 text-fg">{o.label}</span>
                    {value && (
                      <span data-testid="sheet-option-value" className="truncate text-13 text-fg-tertiary">
                        {value}
                      </span>
                    )}
                  </span>
                  <span className="flex max-w-[45%] flex-none items-center justify-end">{control(o)}</span>
                </li>
              );
            })}
          </ul>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}

"use client";

import { useState, type KeyboardEvent } from "react";
import { cx } from "@/lib/cx";
import { SlideSvg } from "@/lib/slides/render";
import { DropdownMenu } from "radix-ui";
import { slideTitle, type Deck, type Layout } from "@/lib/slides/schema";
import { floatPanel } from "@/components/domain/pickers";
import { LayoutGrid } from "./layout-picker";

/**
 * The deck's slides in order (mockup 2/6). Click selects; drag, or Alt+↑/↓,
 * reorders; ⌘D duplicates; Delete removes. Amber “!” marks a slide whose text
 * doesn't fit; an “Ada” badge marks slides with a persona's pending change.
 */
export function Thumbnails({ deck, current, onSelect, onMove, onDuplicate, onRemove, onAdd, onAddLayout, assetUrl, fontsReady, overflowing, changed, persona, readOnly }: {
  deck: Deck;
  current: string;
  onSelect: (id: string) => void;
  onMove: (id: string, to: number) => void;
  onDuplicate: (id: string) => void;
  onRemove: (id: string) => void;
  onAdd: () => void;
  /** Adds a slide with a chosen layout after the current one. */
  onAddLayout: (l: Layout) => void;
  assetUrl: (id: string) => string;
  fontsReady: boolean;
  overflowing: Set<string>;
  changed?: Set<string>;
  persona: string;
  readOnly: boolean;
}) {
  const [dragging, setDragging] = useState<string>();
  const [over, setOver] = useState<number>();
  const onKey = (e: KeyboardEvent<HTMLButtonElement>, id: string, i: number) => {
    const n = deck.slides.length;
    if (e.altKey && (e.key === "ArrowUp" || e.key === "ArrowDown")) {
      e.preventDefault();
      if (!readOnly) onMove(id, Math.max(0, Math.min(n - 1, i + (e.key === "ArrowUp" ? -1 : 1))));
    } else if (e.key === "ArrowUp" || e.key === "ArrowDown") {
      e.preventDefault();
      const next = deck.slides[Math.max(0, Math.min(n - 1, i + (e.key === "ArrowUp" ? -1 : 1)))];
      onSelect(next.id);
      requestAnimationFrame(() => document.querySelector<HTMLButtonElement>(`[data-thumb="${next.id}"]`)?.focus());
    } else if ((e.key === "Delete" || e.key === "Backspace") && !readOnly && n > 1) {
      e.preventDefault();
      onRemove(id);
    } else if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "d" && !readOnly) {
      e.preventDefault();
      onDuplicate(id);
    }
  };
  return (
    <nav aria-label="Slides" className="flex min-h-0 w-full flex-1 flex-col bg-panel max-md:flex-none max-md:flex-row max-md:border-t max-md:border-border-subtle">
      <div className="flex items-center justify-between px-3.5 pb-1 pt-3 max-md:hidden">
        <span className="text-[12px] font-medium text-fg-secondary">Slides</span>
        <span className="font-mono text-[11.5px] text-fg-tertiary">{deck.slides.length}</span>
      </div>
      <ol className="flex min-h-0 flex-1 flex-col gap-3 overflow-y-auto px-3 py-2 max-md:flex-row max-md:overflow-x-auto max-md:overflow-y-hidden">
        {deck.slides.map((s, i) => (
          <li
            key={s.id}
            className={cx("flex gap-1.5", over === i && dragging && "rounded-md ring-2 ring-accent/60")}
            onDragOver={(e) => {
              if (!dragging) return;
              e.preventDefault();
              setOver(i);
            }}
            onDrop={(e) => {
              e.preventDefault();
              if (dragging) onMove(dragging, i);
              setDragging(undefined);
              setOver(undefined);
            }}
          >
            <span className={cx("w-3 flex-none pt-0.5 text-right font-mono text-[11px] max-md:hidden", s.id === current ? "font-semibold text-accent" : "text-fg-tertiary")}>{i + 1}</span>
            <button
              type="button"
              data-thumb={s.id}
              draggable={!readOnly}
              onDragStart={() => setDragging(s.id)}
              onDragEnd={() => {
                setDragging(undefined);
                setOver(undefined);
              }}
              aria-current={s.id === current ? "true" : undefined}
              aria-label={`Slide ${i + 1}: ${slideTitle(s)}${s.hidden ? " (hidden)" : ""}${overflowing.has(s.id) ? ", text doesn't fit" : ""}${changed?.has(s.id) ? `, changed by ${persona}` : ""}`}
              onClick={() => onSelect(s.id)}
              onKeyDown={(e) => onKey(e, s.id, i)}
              className={cx(
                "relative block w-[128px] flex-none overflow-hidden rounded-[4px] border outline-none focus-visible:ring-2 focus-visible:ring-accent/50",
                s.id === current ? "border-2 border-accent" : "border-border hover:border-border-strong",
                s.hidden && "opacity-50",
              )}
            >
              <SlideSvg deck={deck} slide={s} assetUrl={assetUrl} fontsReady={fontsReady} className="block aspect-video w-full" label="" />
              {overflowing.has(s.id) && <span aria-hidden className="absolute right-1 top-1 grid size-3.5 place-items-center rounded-full bg-state-human text-[9px] font-extrabold text-approve-fg">!</span>}
              {changed?.has(s.id) && <span aria-hidden className="absolute bottom-1 left-1 rounded-[4px] bg-state-human px-1 text-[10px] font-bold text-approve-fg">{persona}</span>}
            </button>
          </li>
        ))}
        <li className="pl-[18px] max-md:pl-0">
          <div className="flex h-[30px] w-[128px] rounded-md border border-dashed border-border-strong text-[12.5px] text-fg-secondary">
            <button type="button" disabled={readOnly} onClick={onAdd} className="flex-1 rounded-l-md hover:bg-hover disabled:opacity-40">
              + New slide
            </button>
            <DropdownMenu.Root modal={false}>
              <DropdownMenu.Trigger asChild disabled={readOnly}>
                <button type="button" aria-label="New slide with layout" className="w-7 rounded-r-md border-l border-dashed border-border-strong hover:bg-hover disabled:opacity-40 data-[state=open]:bg-hover">▾</button>
              </DropdownMenu.Trigger>
              <DropdownMenu.Portal>
                <DropdownMenu.Content side="right" align="end" sideOffset={8} collisionPadding={8} className={cx(floatPanel, "z-50 max-h-[80vh] overflow-y-auto")}>
                  <LayoutGrid theme={deck.theme} onPick={onAddLayout} title="New slide" />
                </DropdownMenu.Content>
              </DropdownMenu.Portal>
            </DropdownMenu.Root>
          </div>
        </li>
      </ol>
    </nav>
  );
}

"use client";

import { Popover } from "radix-ui";
import { useMemo, useRef, useState, type KeyboardEvent } from "react";
import { cx } from "@/lib/cx";
import { SHAPES, SHAPE_CATEGORIES, shapeGeometry, svgPath } from "@/lib/slides/geometry";
import type { ShapeKind } from "@/lib/slides/schema";
import { floatPanel } from "@/components/domain/pickers";

const COLS = 6;

/** A tile's icon: the real outline from the shared geometry, drawn small. Callouts leave room for the tail. */
function Icon({ shape }: { shape: ShapeKind }) {
  const callout = shape.startsWith("wedge");
  const f = callout ? { x: 9, y: 9, w: 26, h: 18 } : { x: 9, y: 11, w: 26, h: 22 };
  return (
    <svg viewBox="0 0 44 44" className="size-9" aria-hidden>
      <path d={svgPath(shapeGeometry(shape, f, 5).segs)} fill="none" stroke="currentColor" strokeWidth={1.5} strokeLinejoin="round" />
    </svg>
  );
}

/**
 * The toolbar's Shape ▾ grid (mockup "Slides toolbar: shape grid picker"):
 * search on top, icon tiles by category, arrow keys move across the grid,
 * Enter or click inserts.
 */
export function ShapePicker({ chip, onPick }: { chip: string; onPick: (s: ShapeKind) => void }) {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [focus, setFocus] = useState<ShapeKind | undefined>();
  const tiles = useRef(new Map<ShapeKind, HTMLButtonElement>());

  // Visible tiles grouped into rows, so ↑/↓ keep the column across categories.
  const groups = useMemo(() => {
    const q = query.trim().toLowerCase();
    const hit = (s: (typeof SHAPES)[number]) => !q || s.label.toLowerCase().includes(q) || s.id.toLowerCase().includes(q) || s.category.toLowerCase().includes(q);
    return SHAPE_CATEGORIES.map((c) => ({ category: c, shapes: SHAPES.filter((s) => s.category === c && hit(s)) })).filter((g) => g.shapes.length);
  }, [query]);
  const rows = useMemo(() => groups.flatMap((g) => Array.from({ length: Math.ceil(g.shapes.length / COLS) }, (_, i) => g.shapes.slice(i * COLS, (i + 1) * COLS).map((s) => s.id))), [groups]);
  const flat = rows.flat();
  const label = (id?: ShapeKind) => SHAPES.find((s) => s.id === id)?.label;

  const move = (id: ShapeKind) => {
    setFocus(id);
    tiles.current.get(id)?.focus();
  };
  const pick = (id: ShapeKind) => {
    setOpen(false);
    onPick(id);
  };
  const onGridKey = (e: KeyboardEvent) => {
    const r = focus ? rows.findIndex((row) => row.includes(focus)) : -1;
    if (!focus || r < 0) return;
    const c = rows[r].indexOf(focus);
    const i = flat.indexOf(focus);
    let next: ShapeKind | undefined;
    if (e.key === "ArrowRight") next = flat[Math.min(i + 1, flat.length - 1)];
    else if (e.key === "ArrowLeft") next = flat[Math.max(i - 1, 0)];
    else if (e.key === "ArrowDown") next = rows[r + 1]?.[Math.min(c, rows[r + 1].length - 1)];
    else if (e.key === "ArrowUp") {
      if (r === 0) {
        e.preventDefault();
        setFocus(undefined);
        document.getElementById("shape-search")?.focus();
        return;
      }
      next = rows[r - 1][Math.min(c, rows[r - 1].length - 1)];
    } else if (e.key === "Home") next = flat[0];
    else if (e.key === "End") next = flat.at(-1);
    else return;
    e.preventDefault();
    if (next) move(next);
  };

  return (
    <Popover.Root
      open={open}
      onOpenChange={(o) => {
        setOpen(o);
        if (o) {
          setQuery("");
          setFocus(undefined);
        }
      }}
    >
      <Popover.Trigger asChild>
        <button type="button" aria-label="Insert shape" className={chip} onMouseDown={(e) => e.preventDefault()}>
          ◻ Shape ▾
        </button>
      </Popover.Trigger>
      <Popover.Portal>
        <Popover.Content align="start" sideOffset={6} className={cx(floatPanel, "w-[332px] p-3")} onOpenAutoFocus={(e) => (e.preventDefault(), document.getElementById("shape-search")?.focus())} onCloseAutoFocus={(e) => e.preventDefault()}>
          <input
            id="shape-search"
            type="search"
            value={query}
            onChange={(e) => {
              setQuery(e.target.value);
              setFocus(undefined);
            }}
            placeholder="Search shapes…"
            aria-label="Search shapes"
            aria-controls="shape-grid"
            autoComplete="off"
            className="mb-2 h-8 w-full rounded-md border border-border-strong bg-transparent px-2.5 text-13 outline-none focus-visible:ring-2 focus-visible:ring-accent"
            onKeyDown={(e) => {
              if (e.key === "ArrowDown" && flat[0]) {
                e.preventDefault();
                move(flat[0]);
              } else if (e.key === "Enter" && flat[0]) {
                e.preventDefault();
                pick(flat[0]);
              }
            }}
          />
          <div id="shape-grid" role="grid" aria-label="Shapes" className="max-h-[60vh] overflow-y-auto" onKeyDown={onGridKey}>
            {groups.map((g) => (
              <div key={g.category} role="rowgroup" aria-label={g.category}>
                <div className="mb-1 px-0.5 text-11 font-semibold uppercase tracking-wide text-fg-3">{g.category}</div>
                {Array.from({ length: Math.ceil(g.shapes.length / COLS) }, (_, r) => g.shapes.slice(r * COLS, (r + 1) * COLS)).map((row, r) => (
                <div key={r} role="row" className="mb-2 grid grid-cols-6 gap-2">
                  {row.map((s) => (
                    <button
                      key={s.id}
                      ref={(b) => {
                        if (b) tiles.current.set(s.id, b);
                        else tiles.current.delete(s.id);
                      }}
                      type="button"
                      role="gridcell"
                      aria-label={s.label}
                      title={s.label}
                      tabIndex={focus === s.id || (!focus && s.id === flat[0]) ? 0 : -1}
                      onFocus={() => setFocus(s.id)}
                      onMouseEnter={() => setFocus(s.id)}
                      onClick={() => pick(s.id)}
                      className="flex size-11 items-center justify-center rounded-md border border-border-subtle text-fg-2 outline-none hover:bg-hover focus-visible:border-accent focus-visible:bg-selected focus-visible:ring-2 focus-visible:ring-accent"
                    >
                      <Icon shape={s.id} />
                    </button>
                  ))}
                </div>
                ))}
              </div>
            ))}
            {!groups.length && <p className="px-1 py-6 text-center text-fg-3">No shapes match “{query}”.</p>}
          </div>
          <div className="mt-1 flex items-center justify-between border-t border-border-subtle pt-2 text-12">
            <span className="text-fg-2" aria-live="polite">{label(focus) ?? (flat.length ? `${flat.length} shapes` : "")}</span>
            <span className="text-11 text-fg-3">↑↓←→ move · Enter insert · Esc close</span>
          </div>
        </Popover.Content>
      </Popover.Portal>
    </Popover.Root>
  );
}

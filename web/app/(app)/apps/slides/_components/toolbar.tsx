"use client";

import { DropdownMenu } from "radix-ui";
import type { ReactNode } from "react";
import { cx } from "@/lib/cx";
import type { AlignOp, ZOrder } from "@/lib/slides/arrange";
import { LAYOUTS, THEME_COLORS, THEMES, resolveColor, textOf, type ColorRef, type Element, type Layout, type Paragraph, type Slot, type Theme, type ThemeKey } from "@/lib/slides/schema";
import { floatPanel } from "@/components/domain/pickers";

export type TextFormat =
  | { kind: "mark"; mark: "bold" | "italic" | "underline" }
  | { kind: "size"; size: number }
  | { kind: "font"; font: "heading" | "body" | Slot }
  | { kind: "color"; color: ColorRef }
  | { kind: "align"; align: NonNullable<Paragraph["align"]> }
  | { kind: "bullet"; bullet: Paragraph["bullet"] | undefined };

export type ToolbarActions = {
  undo: () => void;
  redo: () => void;
  canUndo: boolean;
  canRedo: boolean;
  insertText: () => void;
  insertShape: (shape: "rect" | "roundRect" | "ellipse") => void;
  insertImage: () => void;
  insertLine: () => void;
  setLayout: (l: Layout) => void;
  setTheme: (t: ThemeKey) => void;
  setBackground: (c: ColorRef | undefined) => void;
  format: (f: TextFormat) => void;
  setFill: (c: ColorRef | undefined) => void;
  setStroke: (c: ColorRef | undefined) => void;
  remove: () => void;
  order: (op: ZOrder) => void;
  duplicate: () => void;
  align: (op: AlignOp) => void;
  distribute: (axis: "x" | "y") => void;
};

/** The one value every item has, or MIXED when they differ. */
const MIXED = Symbol("mixed");
function common<T>(xs: T[]): T | typeof MIXED | undefined {
  if (!xs.length) return undefined;
  return xs.every((x) => x === xs[0]) ? xs[0] : MIXED;
}
const mixedSwatch = "repeating-linear-gradient(45deg, #c7c7cc 0 3px, transparent 3px 6px)";

const ORDER: { op: ZOrder; label: string; keys: string }[] = [
  { op: "forward", label: "Bring forward", keys: "⌘]" },
  { op: "backward", label: "Send backward", keys: "⌘[" },
  { op: "front", label: "Bring to front", keys: "⌥⌘]" },
  { op: "back", label: "Send to back", keys: "⌥⌘[" },
];
const ALIGN: { op: AlignOp; label: string }[] = [
  { op: "left", label: "Align left" },
  { op: "center", label: "Align centre" },
  { op: "right", label: "Align right" },
  { op: "top", label: "Align top" },
  { op: "middle", label: "Align middle" },
  { op: "bottom", label: "Align bottom" },
];

const chip = "inline-flex h-7 items-center gap-1 rounded-md border border-border-strong px-2 text-[12.5px] text-fg-2 outline-none hover:bg-hover focus-visible:ring-2 focus-visible:ring-accent disabled:opacity-40 data-[state=open]:bg-hover";
const on = "border-accent bg-selected text-fg";
const item = "flex cursor-pointer select-none items-center gap-2 rounded-md px-2.5 py-1.5 text-[13px] text-fg outline-none data-[highlighted]:bg-hover";
const sep = <span aria-hidden className="mx-1 h-5 w-px bg-border-strong" />;
const SIZES = [24, 32, 40, 44, 48, 56, 64, 72, 80, 96, 112, 128, 160];

/** Keeps focus where it is (the text being edited) when a button is pressed. */
const keep = (e: React.MouseEvent) => e.preventDefault();

function Menu({ label, trigger, children, disabled }: { label: string; trigger: ReactNode; children: ReactNode; disabled?: boolean }) {
  return (
    <DropdownMenu.Root modal={false}>
      <DropdownMenu.Trigger asChild disabled={disabled}>
        <button type="button" aria-label={label} className={chip} onMouseDown={keep}>
          {trigger}
        </button>
      </DropdownMenu.Trigger>
      <DropdownMenu.Portal>
        <DropdownMenu.Content align="start" sideOffset={6} className={cx(floatPanel, "z-50 max-h-[60vh] min-w-40 overflow-y-auto")} onCloseAutoFocus={(e) => e.preventDefault()}>
          {children}
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  );
}

function Swatches({ theme, onPick, none }: { theme: Theme; onPick: (c: ColorRef | undefined) => void; none?: string }) {
  return (
    <>
      {none && (
        <DropdownMenu.Item className={item} onSelect={() => onPick(undefined)}>
          <span className="size-4 rounded-sm border border-dashed border-border-strong" /> {none}
        </DropdownMenu.Item>
      )}
      {THEME_COLORS.map((c) => (
        <DropdownMenu.Item key={c} className={item} onSelect={() => onPick(c)}>
          <span className="size-4 rounded-sm border border-border-strong" style={{ background: resolveColor(theme, c) }} /> {c === "accent2" ? "Accent 2" : c[0].toUpperCase() + c.slice(1)}
        </DropdownMenu.Item>
      ))}
    </>
  );
}

/**
 * The editor's toolbar (mockup 2/6). Undo/redo, insert and layout are always
 * there; text controls show for text, fill and stroke for shapes, arranging
 * for any selection, theme and background when nothing is selected. With
 * several elements selected the controls act on all of them, and a value
 * they don't share shows as mixed.
 */
export function Toolbar({ theme, themeKey, layout, selection, editing, disabled, a }: { theme: Theme; themeKey?: string; layout: Layout; selection: Element[]; editing: boolean; disabled: boolean; a: ToolbarActions }) {
  const selected = selection[0];
  const boxes = selection.flatMap((e) => textOf(e) ?? []);
  const box = boxes[0];
  const run = box?.paragraphs[0]?.runs[0];
  const runs = boxes.flatMap((b) => b.paragraphs.flatMap((p) => p.runs));
  const paras = boxes.flatMap((b) => b.paragraphs);
  const font = common(runs.map((r) => r.font ?? "body"));
  const size = common(runs.map((r) => r.size));
  const color = common(runs.map((r) => r.color ?? "text"));
  const alignment = common(paras.map((p) => p.align ?? "left"));
  const shapes = selection.filter((e) => e.type === "shape");
  const lines = selection.filter((e) => e.type === "line");
  const fill = common(shapes.map((e) => e.fill));
  const multi = selection.length > 1;
  return (
    <div role="toolbar" aria-label="Slide formatting" className={cx("flex min-h-[46px] flex-wrap items-center gap-1.5 border-b border-border-subtle px-3 py-2", disabled && "pointer-events-none opacity-45")} aria-disabled={disabled}>
      <button type="button" className={chip} onMouseDown={keep} onClick={a.undo} disabled={!a.canUndo} aria-label="Undo" title="Undo (⌘Z)">↶</button>
      <button type="button" className={chip} onMouseDown={keep} onClick={a.redo} disabled={!a.canRedo} aria-label="Redo" title="Redo (⇧⌘Z)">↷</button>
      {sep}
      <button type="button" className={chip} onClick={a.insertText}>T Text</button>
      <Menu label="Insert shape" trigger={<>◻ Shape ▾</>}>
        <DropdownMenu.Item className={item} onSelect={() => a.insertShape("rect")}>Rectangle</DropdownMenu.Item>
        <DropdownMenu.Item className={item} onSelect={() => a.insertShape("roundRect")}>Rounded rectangle</DropdownMenu.Item>
        <DropdownMenu.Item className={item} onSelect={() => a.insertShape("ellipse")}>Ellipse</DropdownMenu.Item>
      </Menu>
      <button type="button" className={chip} onClick={a.insertImage}>▨ Image</button>
      <button type="button" className={chip} onClick={a.insertLine}>╱ Line</button>
      {sep}
      <Menu label="Slide layout" trigger={<>▤ {LAYOUTS.find((l) => l.id === layout)?.label ?? "Layout"} ▾</>}>
        {LAYOUTS.map((l) => (
          <DropdownMenu.Item key={l.id} className={cx(item, l.id === layout && "font-semibold")} onSelect={() => a.setLayout(l.id)}>{l.label}</DropdownMenu.Item>
        ))}
      </Menu>
      {sep}
      {box && run ? (
        <>
          <Menu label="Font" trigger={<>{font === MIXED ? "Mixed" : font === "heading" ? "Heading" : font && font !== "body" ? font[0].toUpperCase() + font.slice(1) : "Body"} ▾</>}>
            {(["heading", "body", "sans", "serif", "mono"] as const).map((f) => (
              <DropdownMenu.Item key={f} className={item} onSelect={() => a.format({ kind: "font", font: f })}>{f === "heading" ? "Theme heading font" : f === "body" ? "Theme body font" : f[0].toUpperCase() + f.slice(1)}</DropdownMenu.Item>
            ))}
          </Menu>
          <Menu label="Text size" trigger={<span className="font-mono">{size === MIXED || size === undefined ? "–" : size / 2}</span>}>
            {SIZES.map((s) => (
              <DropdownMenu.Item key={s} className={item} onSelect={() => a.format({ kind: "size", size: s })}>{s / 2} pt</DropdownMenu.Item>
            ))}
          </Menu>
          {(["bold", "italic", "underline"] as const).map((m) => {
            const all = runs.every((r) => r[m]), some = runs.some((r) => r[m]);
            return (
            <button
              key={m}
              type="button"
              aria-label={m[0].toUpperCase() + m.slice(1)}
              aria-pressed={editing ? false : all ? true : some ? "mixed" : false}
              title={`${m[0].toUpperCase() + m.slice(1)} (⌘${m[0].toUpperCase()})`}
              className={cx(chip, "w-7 justify-center", !editing && all && on, !editing && !all && some && "border-dashed border-accent", m === "bold" && "font-bold", m === "italic" && "italic", m === "underline" && "underline")}
              onMouseDown={keep}
              onClick={() => a.format({ kind: "mark", mark: m })}
            >
              {m[0].toUpperCase()}
            </button>
            );
          })}
          <Menu label="Text colour" trigger={<><span className="size-3 rounded-sm border border-border-strong" title={color === MIXED ? "Mixed colours" : undefined} style={{ background: color === MIXED ? mixedSwatch : resolveColor(theme, color) }} />▾</>}>
            <Swatches theme={theme} onPick={(c) => c && a.format({ kind: "color", color: c })} />
          </Menu>
          {!editing && (
            <>
              <Menu label="Alignment" trigger={<>≡ ▾</>}>
                {(["left", "center", "right"] as const).map((al) => (
                  <DropdownMenu.Item key={al} className={cx(item, alignment === al && "font-semibold")} onSelect={() => a.format({ kind: "align", align: al })}>{al[0].toUpperCase() + al.slice(1)}</DropdownMenu.Item>
                ))}
              </Menu>
              <Menu label="Bullets" trigger={<>•≡ ▾</>}>
                <DropdownMenu.Item className={item} onSelect={() => a.format({ kind: "bullet", bullet: undefined })}>No bullets</DropdownMenu.Item>
                <DropdownMenu.Item className={item} onSelect={() => a.format({ kind: "bullet", bullet: "disc" })}>Bullets</DropdownMenu.Item>
                <DropdownMenu.Item className={item} onSelect={() => a.format({ kind: "bullet", bullet: "number" })}>Numbered</DropdownMenu.Item>
              </Menu>
            </>
          )}
        </>
      ) : null}
      {shapes.length > 0 && (
        <>
          <Menu label="Fill" trigger={<><span className="size-3 rounded-sm border border-border-strong" title={fill === MIXED ? "Mixed fills" : undefined} style={{ background: fill === MIXED ? mixedSwatch : fill ? resolveColor(theme, fill) : "transparent" }} />Fill ▾</>}>
            <Swatches theme={theme} onPick={a.setFill} none="No fill" />
          </Menu>
          <Menu label="Outline" trigger={<>Outline ▾</>}>
            <Swatches theme={theme} onPick={a.setStroke} none="No outline" />
          </Menu>
        </>
      )}
      {lines.length > 0 && !shapes.length && (
        <Menu label="Line colour" trigger={<>Colour ▾</>}>
          <Swatches theme={theme} onPick={(c) => c && a.setStroke(c)} />
        </Menu>
      )}
      {!selected && (
        <>
          <Menu label="Deck theme" trigger={<>Theme: {theme.name} ▾</>}>
            {Object.entries(THEMES).map(([k, t]) => (
              <DropdownMenu.Item key={k} className={cx(item, k === themeKey && "font-semibold")} onSelect={() => a.setTheme(k)}>
                <span className="h-3.5 w-5 rounded-[3px] border border-border-strong" style={{ background: t.colors.background }} />
                {t.name}
              </DropdownMenu.Item>
            ))}
          </Menu>
          <Menu label="Slide background" trigger={<>Background ▾</>}>
            <Swatches theme={theme} onPick={a.setBackground} none="Theme background" />
          </Menu>
        </>
      )}
      {selected && !editing && (
        <>
          <Menu label="Arrange" trigger={<>⧉ Arrange ▾</>}>
            {ORDER.map((o) => (
              <DropdownMenu.Item key={o.op} className={item} onSelect={() => a.order(o.op)}>
                {o.label}
                <span className="ml-auto pl-4 font-mono text-[11px] text-fg-tertiary">{o.keys}</span>
              </DropdownMenu.Item>
            ))}
            <DropdownMenu.Item className={item} onSelect={a.duplicate}>
              Duplicate
              <span className="ml-auto pl-4 font-mono text-[11px] text-fg-tertiary">⌘D</span>
            </DropdownMenu.Item>
            <DropdownMenu.Separator className="my-1 h-px bg-border-subtle" />
            <DropdownMenu.Label className="px-2.5 py-1 text-[11px] text-fg-tertiary">{multi ? "Align to selection" : "Align to slide"}</DropdownMenu.Label>
            {ALIGN.map((o) => (
              <DropdownMenu.Item key={o.op} className={item} onSelect={() => a.align(o.op)}>{o.label}</DropdownMenu.Item>
            ))}
            <DropdownMenu.Separator className="my-1 h-px bg-border-subtle" />
            <DropdownMenu.Item className={cx(item, "data-[disabled]:opacity-40")} disabled={selection.length < 3} onSelect={() => a.distribute("x")}>Distribute horizontally</DropdownMenu.Item>
            <DropdownMenu.Item className={cx(item, "data-[disabled]:opacity-40")} disabled={selection.length < 3} onSelect={() => a.distribute("y")}>Distribute vertically</DropdownMenu.Item>
          </Menu>
          <button type="button" className={cx(chip, "ml-auto text-pill-blocked-fg")} onClick={a.remove} aria-label={multi ? `Delete ${selection.length} elements` : "Delete element"} title="Delete (⌫)">
            Delete
          </button>
        </>
      )}
    </div>
  );
}

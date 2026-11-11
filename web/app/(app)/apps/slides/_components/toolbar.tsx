"use client";

import { DropdownMenu } from "radix-ui";
import { useState, type ReactNode } from "react";
import { cx } from "@/lib/cx";
import type { AlignOp, ZOrder } from "@/lib/slides/arrange";
import { LAYOUTS, THEMES, THEME_ORDER, dashArray, resolveColor, textOf, type ColorRef, type Element, type Layout, type Paragraph, type ShapeKind, type Slide, type Slot, type Theme } from "@/lib/slides/schema";
import { floatPanel } from "@/components/domain/pickers";
import { LayoutGrid } from "./layout-picker";
import { STYLES, styleOf, type StyleId } from "@/lib/slides/styles";
import { LIMITS, parseFontSize, parseInRange } from "@/lib/slides/edit";
import { FAMILY } from "@/lib/slides/fonts";
import { ColorMenu, Menu, MenuField, chip, item, keep, on } from "./menu";
import { ShapePicker } from "./shape-picker";
import { BackgroundMenu, FillMenu, ImageOpacityMenu, LineMenu, OutlineMenu, type StyleActions } from "./style-menus";

export type TextFormat =
  | { kind: "mark"; mark: "bold" | "italic" | "underline" | "strike" }
  | { kind: "size"; size: number }
  | { kind: "font"; font: "heading" | "body" | Slot }
  | { kind: "color"; color: ColorRef }
  | { kind: "highlight"; highlight: ColorRef | undefined }
  | { kind: "lineSpacing"; lineSpacing: number }
  | { kind: "align"; align: NonNullable<Paragraph["align"]> }
  | { kind: "bullet"; bullet: Paragraph["bullet"] | undefined }
  | { kind: "indent"; delta: 1 | -1 };

export type ToolbarActions = StyleActions & {
  undo: () => void;
  redo: () => void;
  canUndo: boolean;
  canRedo: boolean;
  insertText: () => void;
  insertShape: (shape: ShapeKind) => void;
  insertImage: () => void;
  insertLine: () => void;
  setLayout: (l: Layout) => void;
  /** Puts a copy of a built-in or saved theme in the deck. */
  setTheme: (t: Theme) => void;
  /** The person's saved themes (My themes). */
  savedThemes: (Theme & { id: string })[];
  /** Opens the theme editor on the deck's theme, or on a saved theme. */
  customiseTheme: (savedId?: string) => void;
  deleteSavedTheme: (id: string) => void;
  format: (f: TextFormat) => void;
  /** Applies a style preset to every selected shape and line, as one undo step. */
  applyStyle: (s: StyleId) => void;
  setStrokeWidth: (w: number) => void;
  setRadius: (r: number) => void;
  toggleArrow: (end: "start" | "end") => void;
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

const sep = <span aria-hidden className="mx-1 h-5 w-px bg-border-strong" />;
const SIZES = [24, 32, 40, 44, 48, 56, 64, 72, 80, 96, 112, 128, 160];
const SPACINGS = [1, 1.15, 1.5, 2];
const MARKS = [
  { m: "bold", label: "Bold", glyph: "B", key: "⌘B", cls: "font-bold" },
  { m: "italic", label: "Italic", glyph: "I", key: "⌘I", cls: "italic" },
  { m: "underline", label: "Underline", glyph: "U", key: "⌘U", cls: "underline" },
  { m: "strike", label: "Strikethrough", glyph: "S", key: "⇧⌘X", cls: "line-through" },
] as const;
const WIDTHS = [1, 2, 4, 6, 8, 12, 16, 24, 32, 40];
const RADII = [8, 16, 24, 32, 48, 64, 96, 160];

/** A theme's chip in the theme menu: its background with its two accents. */
function ThemeSwatch({ theme }: { theme: Theme }) {
  const c = theme.colors;
  return (
    <svg aria-hidden width="34" height="19" viewBox="0 0 34 19" className="shrink-0 rounded-[3px] border border-border-strong" style={{ background: c.background }}>
      <rect x="4" y="12" width="11" height="3" fill={c.accent} />
      <rect x="17" y="12" width="11" height="3" fill={c.accent2} />
      <rect x="4" y="5" width="16" height="3" fill={c.heading} />
    </svg>
  );
}

/** A preset's tile: the style drawn with the deck's theme on its background. */
function StyleTile({ theme, id, line }: { theme: Theme; id: StyleId; line: boolean }) {
  const s = STYLES.find((x) => x.id === id)!;
  const c = (t: string | undefined) => (t ? resolveColor(theme, t) : "none");
  return (
    <svg aria-hidden width="92" height={line ? 28 : 46} viewBox={line ? "0 0 92 28" : "0 0 92 46"} className="rounded-md border border-border-strong" style={{ background: theme.colors.background }}>
      {line ? (
        <line x1="10" y1="14" x2="82" y2="14" stroke={c(s.line.stroke)} strokeWidth={Math.max(1.5, s.line.strokeWidth / 2)} strokeLinecap={s.dash ? undefined : "round"} strokeDasharray={dashArray(s.dash, Math.max(1.5, s.line.strokeWidth / 2)).join(" ") || undefined} />
      ) : (
        <>
          <rect x="10" y="7" width="72" height="32" rx="5" fill={c(s.fill)} stroke={c(s.stroke)} strokeWidth={s.stroke ? Math.max(1.5, (s.strokeWidth ?? 0) / 2) : 0} strokeDasharray={dashArray(s.dash, Math.max(1.5, (s.strokeWidth ?? 0) / 2)).join(" ") || undefined} />
          <text x="46" y="27.5" textAnchor="middle" fontSize="12" fill={c(s.text)}>Abc</text>
        </>
      )}
    </svg>
  );
}

function StyleGallery({ theme, current, line, onPick }: { theme: Theme; current?: StyleId; line: boolean; onPick: (s: StyleId) => void }) {
  return (
    <DropdownMenu.Root modal={false}>
      <DropdownMenu.Trigger asChild>
        <button type="button" aria-label={line ? "Line style" : "Shape style"} className={chip} onMouseDown={keep}>
          <span className="size-3 rounded-sm border border-border-strong" style={{ background: resolveColor(theme, "accent") }} />Style ▾
        </button>
      </DropdownMenu.Trigger>
      <DropdownMenu.Portal>
        <DropdownMenu.Content align="start" sideOffset={6} className={cx(floatPanel, "z-50 w-[348px] p-3")} onCloseAutoFocus={(e) => e.preventDefault()}>
          <DropdownMenu.Label className="mb-2 text-[11px] font-semibold uppercase tracking-wide text-fg-3">{line ? "Line style" : "Shape style"} · theme {theme.name}</DropdownMenu.Label>
          <DropdownMenu.RadioGroup value={current ?? ""} onValueChange={(v) => onPick(v as StyleId)} className="grid grid-cols-3 gap-2">
            {STYLES.map((s) => (
              <DropdownMenu.RadioItem
                key={s.id}
                value={s.id}
                title={s.label}
                className={cx("flex cursor-pointer select-none flex-col items-center gap-1 rounded-lg border border-transparent p-1.5 text-[11.5px] text-fg-2 outline-none data-[highlighted]:bg-hover", s.id === current && "border-accent")}
              >
                <StyleTile theme={theme} id={s.id} line={line} />
                {s.label}
              </DropdownMenu.RadioItem>
            ))}
          </DropdownMenu.RadioGroup>
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  );
}
/** Presets plus a typed value, for sizes and widths. */
function NumberMenu({ label, trigger, presets, show, value, onPick, field: f, disabled }: { label: string; trigger: ReactNode; presets: number[]; show: (v: number) => string; value?: number; onPick: (v: number) => void; field: { item: string; parse: (raw: string) => { value: number } | { error: string } }; disabled?: boolean }) {
  const [open, setOpen] = useState(false);
  return (
    <Menu label={label} trigger={trigger} open={open} onOpenChange={setOpen} disabled={disabled}>
      {presets.map((v) => (
        <DropdownMenu.Item key={v} className={cx(item, v === value && "font-semibold")} onSelect={() => onPick(v)}>{show(v)}</DropdownMenu.Item>
      ))}
      <DropdownMenu.Separator className="my-1 h-px bg-border-subtle" />
      <MenuField
        label={`${label}: custom value`}
        item={f.item}
        parse={f.parse}
        onApply={(v) => {
          onPick(v);
          setOpen(false);
        }}
      />
    </Menu>
  );
}

const px = (v: number) => `${v} px`;
const widthField = { item: "Custom width…", parse: (raw: string) => parseInRange(raw, LIMITS.strokeWidth.min, LIMITS.strokeWidth.max, " px") };

const spacingField = { item: "Custom spacing…", parse: (raw: string) => {
  const r = parseInRange(raw, LIMITS.lineSpacing.min, LIMITS.lineSpacing.max, "×");
  return "value" in r ? { value: Math.round(r.value * 100) / 100 } : r;
} };

/**
 * The editor's toolbar (mockup 2/6). Undo/redo, insert and layout are always
 * there; text controls show for text, fill and stroke for shapes, arranging
 * for any selection, theme and background when nothing is selected. With
 * several elements selected the controls act on all of them, and a value
 * they don't share shows as mixed.
 */
export function Toolbar({ theme, themeKey, layout, slide, selection, editing, disabled, a }: { theme: Theme; themeKey?: string; layout: Layout; slide?: Slide; selection: Element[]; editing: boolean; disabled: boolean; a: ToolbarActions }) {
  const selected = selection[0];
  const boxes = selection.flatMap((e) => textOf(e) ?? []);
  const box = boxes[0];
  const run = box?.paragraphs[0]?.runs[0];
  const runs = boxes.flatMap((b) => b.paragraphs.flatMap((p) => p.runs));
  const paras = boxes.flatMap((b) => b.paragraphs);
  const bulleted = paras.some((p) => p.bullet);
  const font = common(runs.map((r) => r.font ?? "body"));
  const size = common(runs.map((r) => r.size));
  const color = common(runs.map((r) => r.color ?? "text"));
  const highlight = common(runs.map((r) => r.highlight));
  const alignment = common(paras.map((p) => p.align ?? "left"));
  const spacing = common(paras.map((p) => p.lineSpacing ?? 1));
  const shapes = selection.filter((e) => e.type === "shape");
  const lines = selection.filter((e) => e.type === "line");
  const images = selection.filter((e) => e.type === "image");
  const shape = shapes[0], line = lines[0];
  const outlineWidth = common(shapes.map((e) => (e.stroke ? e.strokeWidth : undefined)));
  const rounded = shapes.filter((e) => e.shape === "roundRect");
  const radius = common(rounded.map((e) => e.radius ?? 24));
  const lineWidth = common(lines.map((e) => e.strokeWidth));
  /** The style preset every item shares, if any. */
  const preset = (els: Element[]) => {
    const st = common(els.map((e) => styleOf(e)));
    return st === MIXED ? undefined : st;
  };
  const multi = selection.length > 1;
  return (
    <div role="toolbar" aria-label="Slide formatting" className={cx("flex min-h-[46px] flex-wrap items-center gap-1.5 border-b border-border-subtle px-3 py-2", disabled && "pointer-events-none opacity-45")} aria-disabled={disabled}>
      <button type="button" className={chip} onMouseDown={keep} onClick={a.undo} disabled={!a.canUndo} aria-label="Undo" title="Undo (⌘Z)">↶</button>
      <button type="button" className={chip} onMouseDown={keep} onClick={a.redo} disabled={!a.canRedo} aria-label="Redo" title="Redo (⇧⌘Z)">↷</button>
      {sep}
      <button type="button" className={chip} onClick={a.insertText}>T Text</button>
      <ShapePicker chip={chip} onPick={a.insertShape} />
      <button type="button" className={chip} onClick={a.insertImage}>▨ Image</button>
      <button type="button" className={chip} onClick={a.insertLine}>╱ Line</button>
      {sep}
      <Menu label="Slide layout" trigger={<>▤ {LAYOUTS.find((l) => l.id === layout)?.label ?? "Layout"} ▾</>}>
        <LayoutGrid theme={theme} current={layout} onPick={a.setLayout} title="Slide layout" />
      </Menu>
      {sep}
      {box && run ? (
        <>
          <Menu label="Font" trigger={<>{font === MIXED ? "Mixed" : font === "heading" ? "Heading" : font && font !== "body" ? font[0].toUpperCase() + font.slice(1) : "Body"} ▾</>}>
            {(["heading", "body", "sans", "serif", "mono"] as const).map((f) => (
              <DropdownMenu.Item key={f} className={item} onSelect={() => a.format({ kind: "font", font: f })}>{f === "heading" ? "Theme heading font" : f === "body" ? "Theme body font" : f[0].toUpperCase() + f.slice(1)}</DropdownMenu.Item>
            ))}
          </Menu>
          <NumberMenu label="Text size" trigger={<span className="font-mono">{size === MIXED || size === undefined ? "–" : size / 2}</span>} presets={SIZES} show={(s) => `${s / 2} pt`} value={size === MIXED ? undefined : size} onPick={(size) => a.format({ kind: "size", size })} field={{ item: "Custom size (pt)…", parse: parseFontSize }} />
          {MARKS.map(({ m, label, glyph, key, cls }) => {
            const all = runs.every((r) => r[m]), some = runs.some((r) => r[m]);
            return (
            <button
              key={m}
              type="button"
              aria-label={label}
              aria-pressed={editing ? false : all ? true : some ? "mixed" : false}
              title={`${label} (${key})`}
              className={cx(chip, "w-7 justify-center", !editing && all && on, !editing && !all && some && "border-dashed border-accent", cls)}
              onMouseDown={keep}
              onClick={() => a.format({ kind: "mark", mark: m })}
            >
              {glyph}
            </button>
            );
          })}
          <ColorMenu label="Text colour" trigger={<><span className="size-3 rounded-sm border border-border-strong" title={color === MIXED ? "Mixed colours" : undefined} style={{ background: color === MIXED ? mixedSwatch : resolveColor(theme, color) }} />▾</>} theme={theme} onPick={(c) => c && a.format({ kind: "color", color: c })} recent={a.recentColors} remember={a.rememberColor} />
          <ColorMenu label="Highlight colour" trigger={<><span className="size-3 rounded-sm border border-border-strong" title={highlight === MIXED ? "Mixed highlights" : undefined} style={{ background: highlight === MIXED ? mixedSwatch : highlight ? resolveColor(theme, highlight) : "transparent" }} />Highlight ▾</>} theme={theme} onPick={(c) => a.format({ kind: "highlight", highlight: c })} none="None" recent={a.recentColors} remember={a.rememberColor} />
          {!editing && (
            <>
              <Menu label="Alignment" trigger={<>≡ ▾</>}>
                {(["left", "center", "right", "justify"] as const).map((al) => (
                  <DropdownMenu.Item key={al} className={cx(item, alignment === al && "font-semibold")} onSelect={() => a.format({ kind: "align", align: al })}>{al[0].toUpperCase() + al.slice(1)}</DropdownMenu.Item>
                ))}
              </Menu>
              <NumberMenu label="Line spacing" trigger={<>↕ {spacing === MIXED ? "–" : (spacing ?? 1)} ▾</>} presets={SPACINGS} show={(n) => n.toFixed(n === 1.15 ? 2 : 1)} value={spacing === MIXED ? undefined : spacing} onPick={(lineSpacing) => a.format({ kind: "lineSpacing", lineSpacing })} field={spacingField} />
              <Menu label="Bullets" trigger={<>•≡ ▾</>}>
                <DropdownMenu.Item className={item} onSelect={() => a.format({ kind: "bullet", bullet: undefined })}>No bullets</DropdownMenu.Item>
                <DropdownMenu.Item className={item} onSelect={() => a.format({ kind: "bullet", bullet: "disc" })}>Bullets</DropdownMenu.Item>
                <DropdownMenu.Item className={item} onSelect={() => a.format({ kind: "bullet", bullet: "number" })}>Numbered</DropdownMenu.Item>
              </Menu>
            </>
          )}
          {(["Outdent", "Indent"] as const).map((k) => (
            <button
              key={k}
              type="button"
              aria-label={k}
              title={`${k} bullets (${k === "Indent" ? "Tab" : "⇧Tab"} while typing)`}
              className={cx(chip, "w-7 justify-center")}
              disabled={!bulleted}
              onMouseDown={keep}
              onClick={() => a.format({ kind: "indent", delta: k === "Indent" ? 1 : -1 })}
            >
              {k === "Indent" ? "⇥" : "⇤"}
            </button>
          ))}
        </>
      ) : null}
      {shapes.length > 0 && (
        <>
          <StyleGallery theme={theme} current={preset(shapes)} line={false} onPick={a.applyStyle} />
          <FillMenu theme={theme} shapes={shapes} a={a} />
          <OutlineMenu theme={theme} shapes={shapes} a={a} />
          <NumberMenu label="Outline width" trigger={<span className="font-mono">{outlineWidth === MIXED ? "Mixed" : outlineWidth ? px(outlineWidth) : "Width"} ▾</span>} presets={WIDTHS} show={px} value={outlineWidth === MIXED ? undefined : outlineWidth} onPick={a.setStrokeWidth} field={widthField} />
          {rounded.length > 0 && (
            <NumberMenu label="Corner radius" trigger={<span className="font-mono">◜ {radius === MIXED ? "–" : radius} ▾</span>} presets={RADII} show={px} value={radius === MIXED ? undefined : radius} onPick={a.setRadius} field={{ item: "Custom radius…", parse: (raw) => parseInRange(raw, LIMITS.radius.min, LIMITS.radius.max, " px") }} />
          )}
        </>
      )}
      {line && (
        <>
          {!shape && <StyleGallery theme={theme} current={preset(lines)} line onPick={a.applyStyle} />}
          <LineMenu theme={theme} lines={lines} a={a} />
          <NumberMenu label="Line width" trigger={<span className="font-mono">{lineWidth === MIXED || lineWidth === undefined ? "Mixed" : px(lineWidth)} ▾</span>} presets={WIDTHS} show={px} value={lineWidth === MIXED ? undefined : lineWidth} onPick={a.setStrokeWidth} field={widthField} />
          {(["start", "end"] as const).map((end) => {
            const has = (e: (typeof lines)[number]) => !!(end === "start" ? e.startArrow : e.endArrow);
            const all = lines.every(has), some = lines.some(has);
            return (
              <button key={end} type="button" aria-label={end === "start" ? "Start arrowhead" : "End arrowhead"} aria-pressed={all ? true : some ? "mixed" : false} title={end === "start" ? "Arrowhead at the start" : "Arrowhead at the end"} className={cx(chip, all && on, !all && some && "border-dashed border-accent")} onClick={() => a.toggleArrow(end)}>
                {end === "start" ? "◀─" : "─▶"}
              </button>
            );
          })}
        </>
      )}
      {images.length > 0 && <ImageOpacityMenu images={images} a={a} />}
      {!selected && (
        <>
          <Menu label="Deck theme" trigger={<>Theme: {theme.name} ▾</>}>
            <DropdownMenu.Label className="px-2.5 pb-1 pt-1.5 text-[11px] font-semibold uppercase tracking-wide text-fg-3">Built-in</DropdownMenu.Label>
            {THEME_ORDER.map((k) => (
              <DropdownMenu.Item key={k} className={cx(item, k === themeKey && "font-semibold")} onSelect={() => a.setTheme(THEMES[k])}>
                <ThemeSwatch theme={THEMES[k]} />
                <span style={{ fontFamily: `"${FAMILY[THEMES[k].fonts.heading]}"` }}>{THEMES[k].name}</span>
                {k === themeKey && <span aria-hidden className="ml-auto">✓</span>}
              </DropdownMenu.Item>
            ))}
            <DropdownMenu.Separator className="my-1 h-px bg-border" />
            <DropdownMenu.Label className="px-2.5 pb-1 pt-1.5 text-[11px] font-semibold uppercase tracking-wide text-fg-3">My themes</DropdownMenu.Label>
            {a.savedThemes.length === 0 && <p className="max-w-60 px-2.5 pb-1.5 text-[12px] text-fg-3">Save a customised theme to reuse it in every deck.</p>}
            {a.savedThemes.map((t) => {
              const current = !themeKey && t.name === theme.name;
              return (
                <div key={t.id} className="flex items-center">
                  <DropdownMenu.Item className={cx(item, "min-w-0 flex-1", current && "font-semibold")} onSelect={() => a.setTheme(t)}>
                    <ThemeSwatch theme={t} />
                    <span className="truncate">{t.name}</span>
                    {current && <span aria-hidden className="ml-auto">✓</span>}
                  </DropdownMenu.Item>
                  <DropdownMenu.Item className={cx(item, "px-1.5 text-fg-3")} aria-label={`Edit or rename ${t.name}`} title="Edit or rename" onSelect={() => a.customiseTheme(t.id)}>✎</DropdownMenu.Item>
                  <DropdownMenu.Item className={cx(item, "px-1.5 text-fg-3")} aria-label={`Delete ${t.name}`} title="Delete" onSelect={() => a.deleteSavedTheme(t.id)}>🗑</DropdownMenu.Item>
                </div>
              );
            })}
            <DropdownMenu.Separator className="my-1 h-px bg-border" />
            <DropdownMenu.Item className={item} onSelect={() => a.customiseTheme()}>Customise current theme…</DropdownMenu.Item>
          </Menu>
          {slide && <BackgroundMenu theme={theme} slide={slide} a={a} />}
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

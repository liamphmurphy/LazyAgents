"use client";

import { DropdownMenu } from "radix-ui";
import { useId, useRef, useState, type ReactNode } from "react";
import { cx } from "@/lib/cx";
import { LAYOUTS, THEME_COLORS, THEMES, resolveColor, textOf, type ColorRef, type Element, type Layout, type Paragraph, type Slot, type Theme, type ThemeKey } from "@/lib/slides/schema";
import { floatPanel } from "@/components/domain/pickers";
import { LIMITS, parseFontSize, parseHex, parseInRange } from "@/lib/slides/edit";

export type TextFormat =
  | { kind: "mark"; mark: "bold" | "italic" | "underline" }
  | { kind: "size"; size: number }
  | { kind: "font"; font: "heading" | "body" | Slot }
  | { kind: "color"; color: ColorRef }
  | { kind: "align"; align: NonNullable<Paragraph["align"]> }
  | { kind: "bullet"; bullet: Paragraph["bullet"] | undefined }
  | { kind: "indent"; delta: 1 | -1 };

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
  setStrokeWidth: (w: number) => void;
  setRadius: (r: number) => void;
  toggleArrow: (end: "start" | "end") => void;
  /** Custom colours used in this deck, newest first. */
  recentColors: string[];
  rememberColor: (c: string) => void;
  remove: () => void;
};

const chip = "inline-flex h-7 items-center gap-1 rounded-md border border-border-strong px-2 text-[12.5px] text-fg-2 outline-none hover:bg-hover focus-visible:ring-2 focus-visible:ring-accent disabled:opacity-40 data-[state=open]:bg-hover";
const on = "border-accent bg-selected text-fg";
const item = "flex cursor-pointer select-none items-center gap-2 rounded-md px-2.5 py-1.5 text-[13px] text-fg outline-none data-[highlighted]:bg-hover";
const sep = <span aria-hidden className="mx-1 h-5 w-px bg-border-strong" />;
const SIZES = [24, 32, 40, 44, 48, 56, 64, 72, 80, 96, 112, 128, 160];
const WIDTHS = [1, 2, 4, 6, 8, 12, 16, 24, 32, 40];
const RADII = [8, 16, 24, 32, 48, 64, 96, 160];
const field = "h-7 min-w-0 flex-1 rounded-md border border-border-strong bg-transparent px-2 text-[12.5px] text-fg outline-none focus-visible:ring-2 focus-visible:ring-accent";

/** Keeps focus where it is (the text being edited) when a button is pressed. */
const keep = (e: React.MouseEvent) => e.preventDefault();

function Menu({ label, trigger, children, disabled, open, onOpenChange }: { label: string; trigger: ReactNode; children: ReactNode; disabled?: boolean; open?: boolean; onOpenChange?: (o: boolean) => void }) {
  return (
    <DropdownMenu.Root modal={false} open={open} onOpenChange={onOpenChange}>
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

/**
 * A typed value inside a menu: the menu's item focuses the field, and the
 * field keeps its keys (Radix would otherwise use them for typeahead and stop
 * Tab), so it's reachable and usable from the keyboard. Enter applies; a value
 * outside the limits is refused with a message and the menu stays open.
 */
function MenuField<T>({ label, item: itemLabel, placeholder, parse, onApply, extra }: { label: string; item: string; placeholder?: string; parse: (raw: string) => { value: T } | { error: string }; onApply: (v: T) => void; extra?: (set: (v: string) => void) => ReactNode }) {
  const [raw, setRaw] = useState("");
  const [error, setError] = useState<string>();
  const input = useRef<HTMLInputElement>(null);
  const id = useId();
  const apply = () => {
    const r = parse(raw);
    if ("error" in r) return setError(r.error);
    setError(undefined);
    onApply(r.value);
  };
  return (
    <>
      <DropdownMenu.Item className={item} onSelect={(e) => { e.preventDefault(); input.current?.focus(); }}>{itemLabel}</DropdownMenu.Item>
      <div
        className="flex flex-col gap-1 px-2.5 pb-1.5"
        onKeyDown={(e) => {
          if (e.key === "Escape") return;
          e.stopPropagation();
          if (e.key === "Enter" && e.target === input.current) {
            e.preventDefault();
            apply();
          }
        }}
      >
        <div className="flex items-center gap-1.5">
          <input ref={input} className={field} aria-label={label} aria-invalid={!!error} aria-describedby={error ? id : undefined} placeholder={placeholder} value={raw} onChange={(e) => { setRaw(e.target.value); setError(undefined); }} />
          {extra?.(setRaw)}
          <button type="button" className={chip} onClick={apply}>Apply</button>
        </div>
        {error && <p id={id} role="alert" className="max-w-52 text-[11.5px] text-pill-blocked-fg">{error}</p>}
      </div>
    </>
  );
}

/**
 * A colour menu: "none" (if given), the theme colours, the deck's recent
 * custom colours and a field for any #RRGGBB.
 */
function ColorMenu({ label, trigger, theme, onPick, none, recent, remember }: { label: string; trigger: ReactNode; theme: Theme; onPick: (c: ColorRef | undefined) => void; none?: string; recent: string[]; remember: (c: string) => void }) {
  const [open, setOpen] = useState(false);
  const custom = (c: string) => {
    onPick(c);
    remember(c);
    setOpen(false);
  };
  return (
    <Menu label={label} trigger={trigger} open={open} onOpenChange={setOpen}>
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
      <DropdownMenu.Separator className="my-1 h-px bg-border-subtle" />
      {recent.length > 0 && (
        <DropdownMenu.Group aria-label="Recent colours" className="flex max-w-52 flex-wrap gap-1 px-2.5 py-1">
          {recent.map((c) => (
            <DropdownMenu.Item key={c} aria-label={`Recent colour ${c}`} title={c} className="size-5 cursor-pointer rounded-sm border border-border-strong outline-none data-[highlighted]:ring-2 data-[highlighted]:ring-accent" style={{ background: c }} onSelect={() => custom(c)} />
          ))}
        </DropdownMenu.Group>
      )}
      <MenuField
        label={`${label}: custom hex`}
        item="Custom colour…"
        placeholder="#RRGGBB"
        parse={(raw) => {
          const c = parseHex(raw);
          return c ? { value: c } : { error: "Enter a colour as #RRGGBB, for example #C2410C." };
        }}
        onApply={custom}
        extra={(set) => (
          <input type="color" aria-label={`${label}: pick a colour`} className="h-7 w-8 flex-none cursor-pointer rounded-md border border-border-strong bg-transparent" onChange={(e) => set(e.target.value.toUpperCase())} />
        )}
      />
    </Menu>
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

/**
 * The editor's toolbar (mockup 2/6). Undo/redo, insert and layout are always
 * there; text controls show for text, fill and stroke for shapes, theme and
 * background when nothing is selected.
 */
export function Toolbar({ theme, themeKey, layout, selected, editing, disabled, a }: { theme: Theme; themeKey?: string; layout: Layout; selected?: Element; editing: boolean; disabled: boolean; a: ToolbarActions }) {
  const box = selected && textOf(selected);
  const run = box?.paragraphs[0]?.runs[0];
  const para = box?.paragraphs[0];
  const bulleted = !!box?.paragraphs.some((p) => p.bullet);
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
          <Menu label="Font" trigger={<>{run.font === "heading" ? "Heading" : run.font && run.font !== "body" ? run.font[0].toUpperCase() + run.font.slice(1) : "Body"} ▾</>}>
            {(["heading", "body", "sans", "serif", "mono"] as const).map((f) => (
              <DropdownMenu.Item key={f} className={item} onSelect={() => a.format({ kind: "font", font: f })}>{f === "heading" ? "Theme heading font" : f === "body" ? "Theme body font" : f[0].toUpperCase() + f.slice(1)}</DropdownMenu.Item>
            ))}
          </Menu>
          <NumberMenu label="Text size" trigger={<span className="font-mono">{run.size / 2}</span>} presets={SIZES} show={(s) => `${s / 2} pt`} value={run.size} onPick={(size) => a.format({ kind: "size", size })} field={{ item: "Custom size (pt)…", parse: parseFontSize }} />
          {(["bold", "italic", "underline"] as const).map((m) => (
            <button
              key={m}
              type="button"
              aria-label={m[0].toUpperCase() + m.slice(1)}
              aria-pressed={!editing && !!run[m]}
              title={`${m[0].toUpperCase() + m.slice(1)} (⌘${m[0].toUpperCase()})`}
              className={cx(chip, "w-7 justify-center", !editing && run[m] && on, m === "bold" && "font-bold", m === "italic" && "italic", m === "underline" && "underline")}
              onMouseDown={keep}
              onClick={() => a.format({ kind: "mark", mark: m })}
            >
              {m[0].toUpperCase()}
            </button>
          ))}
          <ColorMenu label="Text colour" trigger={<><span className="size-3 rounded-sm border border-border-strong" style={{ background: resolveColor(theme, run.color) }} />▾</>} theme={theme} onPick={(c) => c && a.format({ kind: "color", color: c })} recent={a.recentColors} remember={a.rememberColor} />
          {!editing && (
            <>
              <Menu label="Alignment" trigger={<>≡ ▾</>}>
                {(["left", "center", "right"] as const).map((al) => (
                  <DropdownMenu.Item key={al} className={cx(item, (para?.align ?? "left") === al && "font-semibold")} onSelect={() => a.format({ kind: "align", align: al })}>{al[0].toUpperCase() + al.slice(1)}</DropdownMenu.Item>
                ))}
              </Menu>
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
      {selected?.type === "shape" && (
        <>
          <ColorMenu label="Fill" trigger={<><span className="size-3 rounded-sm border border-border-strong" style={{ background: selected.fill ? resolveColor(theme, selected.fill) : "transparent" }} />Fill ▾</>} theme={theme} onPick={a.setFill} none="No fill" recent={a.recentColors} remember={a.rememberColor} />
          <ColorMenu label="Outline" trigger={<>Outline ▾</>} theme={theme} onPick={a.setStroke} none="No outline" recent={a.recentColors} remember={a.rememberColor} />
          <NumberMenu label="Outline width" trigger={<span className="font-mono">{selected.stroke && selected.strokeWidth ? px(selected.strokeWidth) : "Width"} ▾</span>} presets={WIDTHS} show={px} value={selected.stroke ? selected.strokeWidth : undefined} onPick={a.setStrokeWidth} field={widthField} />
          {selected.shape === "roundRect" && (
            <NumberMenu label="Corner radius" trigger={<span className="font-mono">◜ {selected.radius ?? 24} ▾</span>} presets={RADII} show={px} value={selected.radius ?? 24} onPick={a.setRadius} field={{ item: "Custom radius…", parse: (raw) => parseInRange(raw, LIMITS.radius.min, LIMITS.radius.max, " px") }} />
          )}
        </>
      )}
      {selected?.type === "line" && (
        <>
          <ColorMenu label="Line colour" trigger={<>Colour ▾</>} theme={theme} onPick={(c) => c && a.setStroke(c)} recent={a.recentColors} remember={a.rememberColor} />
          <NumberMenu label="Line width" trigger={<span className="font-mono">{px(selected.strokeWidth)} ▾</span>} presets={WIDTHS} show={px} value={selected.strokeWidth} onPick={a.setStrokeWidth} field={widthField} />
          {(["start", "end"] as const).map((end) => {
            const pressed = !!(end === "start" ? selected.startArrow : selected.endArrow);
            return (
              <button key={end} type="button" aria-label={end === "start" ? "Start arrowhead" : "End arrowhead"} aria-pressed={pressed} title={end === "start" ? "Arrowhead at the start" : "Arrowhead at the end"} className={cx(chip, pressed && on)} onClick={() => a.toggleArrow(end)}>
                {end === "start" ? "◀─" : "─▶"}
              </button>
            );
          })}
        </>
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
          <ColorMenu label="Slide background" trigger={<>Background ▾</>} theme={theme} onPick={a.setBackground} none="Theme background" recent={a.recentColors} remember={a.rememberColor} />
        </>
      )}
      {selected && !editing && (
        <button type="button" className={cx(chip, "ml-auto text-pill-blocked-fg")} onClick={a.remove} aria-label="Delete element" title="Delete (⌫)">
          Delete
        </button>
      )}
    </div>
  );
}

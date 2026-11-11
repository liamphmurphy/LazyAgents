"use client";

import { DropdownMenu } from "radix-ui";
import { useState, type ReactNode } from "react";
import { cx } from "@/lib/cx";
import { LAYOUTS, THEME_COLORS, THEMES, resolveColor, textOf, type ColorRef, type Element, type Layout, type Paragraph, type Slot, type Theme, type ThemeKey } from "@/lib/slides/schema";
import { floatPanel } from "@/components/domain/pickers";

export type TextFormat =
  | { kind: "mark"; mark: "bold" | "italic" | "underline" | "strike" }
  | { kind: "size"; size: number }
  | { kind: "font"; font: "heading" | "body" | Slot }
  | { kind: "color"; color: ColorRef }
  | { kind: "highlight"; highlight: ColorRef | undefined }
  | { kind: "lineSpacing"; lineSpacing: number }
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
};

const chip = "inline-flex h-7 items-center gap-1 rounded-md border border-border-strong px-2 text-[12.5px] text-fg-2 outline-none hover:bg-hover focus-visible:ring-2 focus-visible:ring-accent disabled:opacity-40 data-[state=open]:bg-hover";
const on = "border-accent bg-selected text-fg";
const item = "flex cursor-pointer select-none items-center gap-2 rounded-md px-2.5 py-1.5 text-[13px] text-fg outline-none data-[highlighted]:bg-hover";
const sep = <span aria-hidden className="mx-1 h-5 w-px bg-border-strong" />;
const SIZES = [24, 32, 40, 44, 48, 56, 64, 72, 80, 96, 112, 128, 160];
const SPACINGS = [1, 1.15, 1.5, 2];
/** The server's lineSpacing bounds (server/internal/deck). */
const MIN_SPACING = 0.8, MAX_SPACING = 3;
const HIGHLIGHTS = ["#FDE68A", "#BBF7D0", "#BFDBFE", "#FBCFE8"];
const MARKS = [
  { m: "bold", label: "Bold", glyph: "B", key: "⌘B", cls: "font-bold" },
  { m: "italic", label: "Italic", glyph: "I", key: "⌘I", cls: "italic" },
  { m: "underline", label: "Underline", glyph: "U", key: "⌘U", cls: "underline" },
  { m: "strike", label: "Strikethrough", glyph: "S", key: "⇧⌘X", cls: "line-through" },
] as const;

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

/** Line spacing presets plus a custom multiple, applied on Enter. */
function LineSpacingMenu({ value, onPick }: { value: number; onPick: (n: number) => void }) {
  const [custom, setCustom] = useState("");
  const apply = () => {
    const n = Number(custom);
    if (Number.isFinite(n) && n >= MIN_SPACING && n <= MAX_SPACING) onPick(Math.round(n * 100) / 100);
  };
  return (
    <Menu label="Line spacing" trigger={<>↕ {value} ▾</>}>
      {SPACINGS.map((n) => (
        <DropdownMenu.Item key={n} className={cx(item, value === n && "font-semibold")} onSelect={() => onPick(n)}>{n.toFixed(n === 1.15 ? 2 : 1)}</DropdownMenu.Item>
      ))}
      <DropdownMenu.Separator className="my-1 h-px bg-border-subtle" />
      <form
        className="flex items-center gap-2 px-2.5 py-1.5 text-[13px] text-fg"
        onSubmit={(e) => {
          e.preventDefault();
          apply();
        }}
      >
        <label htmlFor="slides-line-spacing">Custom</label>
        <input
          id="slides-line-spacing"
          type="number"
          min={MIN_SPACING}
          max={MAX_SPACING}
          step={0.05}
          placeholder={String(value)}
          value={custom}
          onChange={(e) => setCustom(e.target.value)}
          // Keep the menu's typeahead and arrow keys out of the field.
          onKeyDown={(e) => e.key !== "Escape" && e.stopPropagation()}
          className="h-7 w-16 rounded-md border border-border-strong bg-transparent px-1.5 font-mono outline-none focus-visible:ring-2 focus-visible:ring-accent"
        />
      </form>
    </Menu>
  );
}

/**
 * The editor's toolbar (mockup 2/6). Undo/redo, insert and layout are always
 * there; text controls show for text, fill and stroke for shapes, theme and
 * background when nothing is selected.
 */
export function Toolbar({ theme, themeKey, layout, selected, editing, disabled, a }: { theme: Theme; themeKey?: string; layout: Layout; selected?: Element; editing: boolean; disabled: boolean; a: ToolbarActions }) {
  const box = selected && textOf(selected);
  const run = box?.paragraphs[0]?.runs[0];
  const para = box?.paragraphs[0];
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
          <Menu label="Text size" trigger={<span className="font-mono">{run.size / 2}</span>}>
            {SIZES.map((s) => (
              <DropdownMenu.Item key={s} className={item} onSelect={() => a.format({ kind: "size", size: s })}>{s / 2} pt</DropdownMenu.Item>
            ))}
          </Menu>
          {MARKS.map(({ m, label, glyph, key, cls }) => (
            <button
              key={m}
              type="button"
              aria-label={label}
              aria-pressed={!editing && !!run[m]}
              title={`${label} (${key})`}
              className={cx(chip, "w-7 justify-center", !editing && run[m] && on, cls)}
              onMouseDown={keep}
              onClick={() => a.format({ kind: "mark", mark: m })}
            >
              {glyph}
            </button>
          ))}
          <Menu label="Text colour" trigger={<><span className="size-3 rounded-sm border border-border-strong" style={{ background: resolveColor(theme, run.color) }} />▾</>}>
            <Swatches theme={theme} onPick={(c) => c && a.format({ kind: "color", color: c })} />
          </Menu>
          <Menu label="Highlight colour" trigger={<><span className="size-3 rounded-sm border border-border-strong" style={{ background: run.highlight ? resolveColor(theme, run.highlight) : "transparent" }} />Highlight ▾</>}>
            <Swatches theme={theme} onPick={(c) => a.format({ kind: "highlight", highlight: c })} none="None" />
            <DropdownMenu.Separator className="my-1 h-px bg-border-subtle" />
            {HIGHLIGHTS.map((c) => (
              <DropdownMenu.Item key={c} className={item} onSelect={() => a.format({ kind: "highlight", highlight: c })}>
                <span className="size-4 rounded-sm border border-border-strong" style={{ background: c }} /> {c}
              </DropdownMenu.Item>
            ))}
          </Menu>
          {!editing && (
            <>
              <Menu label="Alignment" trigger={<>≡ ▾</>}>
                {(["left", "center", "right", "justify"] as const).map((al) => (
                  <DropdownMenu.Item key={al} className={cx(item, (para?.align ?? "left") === al && "font-semibold")} onSelect={() => a.format({ kind: "align", align: al })}>{al[0].toUpperCase() + al.slice(1)}</DropdownMenu.Item>
                ))}
              </Menu>
              <LineSpacingMenu value={para?.lineSpacing ?? 1} onPick={(n) => a.format({ kind: "lineSpacing", lineSpacing: n })} />
              <Menu label="Bullets" trigger={<>•≡ ▾</>}>
                <DropdownMenu.Item className={item} onSelect={() => a.format({ kind: "bullet", bullet: undefined })}>No bullets</DropdownMenu.Item>
                <DropdownMenu.Item className={item} onSelect={() => a.format({ kind: "bullet", bullet: "disc" })}>Bullets</DropdownMenu.Item>
                <DropdownMenu.Item className={item} onSelect={() => a.format({ kind: "bullet", bullet: "number" })}>Numbered</DropdownMenu.Item>
              </Menu>
            </>
          )}
        </>
      ) : null}
      {selected?.type === "shape" && (
        <>
          <Menu label="Fill" trigger={<><span className="size-3 rounded-sm border border-border-strong" style={{ background: selected.fill ? resolveColor(theme, selected.fill) : "transparent" }} />Fill ▾</>}>
            <Swatches theme={theme} onPick={a.setFill} none="No fill" />
          </Menu>
          <Menu label="Outline" trigger={<>Outline ▾</>}>
            <Swatches theme={theme} onPick={a.setStroke} none="No outline" />
          </Menu>
        </>
      )}
      {selected?.type === "line" && (
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
        <button type="button" className={cx(chip, "ml-auto text-pill-blocked-fg")} onClick={a.remove} aria-label="Delete element" title="Delete (⌫)">
          Delete
        </button>
      )}
    </div>
  );
}

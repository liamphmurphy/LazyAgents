"use client";

import { DropdownMenu } from "radix-ui";
import type { ReactNode } from "react";
import { cx } from "@/lib/cx";
import { LAYOUTS, THEME_COLORS, THEMES, resolveColor, textOf, type ColorRef, type Element, type Layout, type Paragraph, type Slot, type Theme, type ThemeKey } from "@/lib/slides/schema";
import { floatPanel } from "@/components/domain/pickers";
import { STYLES, styleOf, type StyleId } from "@/lib/slides/styles";

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
  /** Applies a style preset to every selected shape and line, as one undo step. */
  applyStyle: (s: StyleId) => void;
  remove: () => void;
};

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

/** A preset's tile: the style drawn with the deck's theme on its background. */
function StyleTile({ theme, id, line }: { theme: Theme; id: StyleId; line: boolean }) {
  const s = STYLES.find((x) => x.id === id)!;
  const c = (t: string | undefined) => (t ? resolveColor(theme, t) : "none");
  return (
    <svg aria-hidden width="92" height={line ? 28 : 46} viewBox={line ? "0 0 92 28" : "0 0 92 46"} className="rounded-md border border-border-strong" style={{ background: theme.colors.background }}>
      {line ? (
        <line x1="10" y1="14" x2="82" y2="14" stroke={c(s.line.stroke)} strokeWidth={Math.max(1.5, s.line.strokeWidth / 2)} strokeLinecap="round" />
      ) : (
        <>
          <rect x="10" y="7" width="72" height="32" rx="5" fill={c(s.fill)} stroke={c(s.stroke)} strokeWidth={s.stroke ? Math.max(1.5, (s.strokeWidth ?? 0) / 2) : 0} />
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
          <Menu label="Text colour" trigger={<><span className="size-3 rounded-sm border border-border-strong" style={{ background: resolveColor(theme, run.color) }} />▾</>}>
            <Swatches theme={theme} onPick={(c) => c && a.format({ kind: "color", color: c })} />
          </Menu>
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
        </>
      ) : null}
      {selected?.type === "shape" && (
        <>
          <StyleGallery theme={theme} current={styleOf(selected)} line={false} onPick={a.applyStyle} />
          <Menu label="Fill" trigger={<><span className="size-3 rounded-sm border border-border-strong" style={{ background: selected.fill ? resolveColor(theme, selected.fill) : "transparent" }} />Fill ▾</>}>
            <Swatches theme={theme} onPick={a.setFill} none="No fill" />
          </Menu>
          <Menu label="Outline" trigger={<>Outline ▾</>}>
            <Swatches theme={theme} onPick={a.setStroke} none="No outline" />
          </Menu>
        </>
      )}
      {selected?.type === "line" && <StyleGallery theme={theme} current={styleOf(selected)} line onPick={a.applyStyle} />}
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

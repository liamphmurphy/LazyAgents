"use client";

import { DropdownMenu } from "radix-ui";
import { cx } from "@/lib/cx";
import { LAYOUTS, THEMES, resolveColor, textOf, type ColorRef, type Element, type Layout, type Paragraph, type Slide, type Slot, type Theme, type ThemeKey } from "@/lib/slides/schema";
import { Menu, Swatches, chip, item, keep, on } from "./menu";
import { BackgroundMenu, FillMenu, ImageOpacityMenu, LineMenu, OutlineMenu, type StyleActions } from "./style-menus";

export type TextFormat =
  | { kind: "mark"; mark: "bold" | "italic" | "underline" }
  | { kind: "size"; size: number }
  | { kind: "font"; font: "heading" | "body" | Slot }
  | { kind: "color"; color: ColorRef }
  | { kind: "align"; align: NonNullable<Paragraph["align"]> }
  | { kind: "bullet"; bullet: Paragraph["bullet"] | undefined };

export type ToolbarActions = StyleActions & {
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
  format: (f: TextFormat) => void;
  remove: () => void;
};

const sep = <span aria-hidden className="mx-1 h-5 w-px bg-border-strong" />;
const SIZES = [24, 32, 40, 44, 48, 56, 64, 72, 80, 96, 112, 128, 160];

/**
 * The editor's toolbar (mockup 2/6). Undo/redo, insert and layout are always
 * there; text controls show for text, fill and stroke for shapes, theme and
 * background when nothing is selected.
 */
export function Toolbar({ theme, themeKey, layout, slide, selected, editing, disabled, a }: { theme: Theme; themeKey?: string; layout: Layout; slide?: Slide; selected?: Element; editing: boolean; disabled: boolean; a: ToolbarActions }) {
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
          <FillMenu theme={theme} el={selected} a={a} />
          <OutlineMenu theme={theme} el={selected} a={a} />
        </>
      )}
      {selected?.type === "line" && <LineMenu theme={theme} el={selected} a={a} />}
      {selected?.type === "image" && <ImageOpacityMenu el={selected} a={a} />}
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
          {slide && <BackgroundMenu theme={theme} slide={slide} a={a} />}
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

"use client";

import { Dialog } from "radix-ui";
import { useState } from "react";
import { cx } from "@/lib/cx";
import { Button, IconButton } from "@/components/ui/button";
import { SlideSvg } from "@/lib/slides/render";
import { parseHex } from "@/lib/slides/edit";
import { FAMILY, SLOTS } from "@/lib/slides/fonts";
import { THEME_COLORS, contrastIssues, type Deck, type Slide, type Slot, type Theme, type ThemeColor } from "@/lib/slides/schema";

const LABEL: Record<ThemeColor, string> = { background: "Background", text: "Text", heading: "Heading", muted: "Muted", accent: "Accent", accent2: "Accent 2", surface: "Surface" };
// Two columns as in the mockup: the text colours on the left, the rest on the right.
const LEFT: ThemeColor[] = ["background", "text", "heading", "muted"];
const RIGHT: ThemeColor[] = THEME_COLORS.filter((c) => !LEFT.includes(c));

/** What the editor is changing: this deck's theme, or one of the person's saved themes. */
export type ThemeEditTarget = { kind: "deck" } | { kind: "saved"; id: string };

function ColorField({ name, value, onChange }: { name: ThemeColor; value: string; onChange: (hex: string) => void }) {
  const [raw, setRaw] = useState(value);
  const [seen, setSeen] = useState(value);
  if (value !== seen) {
    setSeen(value);
    setRaw(value);
  }
  const bad = parseHex(raw) === undefined;
  return (
    <label className="flex items-center gap-2 text-[13px] text-fg">
      <span className="w-[86px]">{LABEL[name]}</span>
      <input type="color" aria-label={`${LABEL[name]} colour picker`} value={value.toLowerCase()} onChange={(e) => onChange(e.target.value.toUpperCase())} className="size-6 cursor-pointer rounded border border-border-strong bg-transparent p-0" />
      <input
        aria-label={`${LABEL[name]} hex`}
        aria-invalid={bad}
        value={raw}
        spellCheck={false}
        onChange={(e) => {
          setRaw(e.target.value);
          const hex = parseHex(e.target.value);
          if (hex) onChange(hex);
        }}
        onBlur={() => setRaw(value)}
        className={cx("h-6 w-24 rounded border bg-surface px-1.5 font-mono text-[12.5px] outline-none focus-visible:ring-2 focus-visible:ring-accent", bad ? "border-pill-blocked-fg" : "border-border-strong")}
      />
    </label>
  );
}

function SlotPicker({ label, value, onChange }: { label: string; value: Slot; onChange: (s: Slot) => void }) {
  return (
    <div role="radiogroup" aria-label={`${label} font`} className="flex items-center gap-2 text-[13px] text-fg">
      <span className="w-[86px]">{label}</span>
      <div className="flex rounded-md border border-border-strong p-0.5">
        {SLOTS.map((s) => (
          <button key={s} type="button" role="radio" aria-checked={value === s} onClick={() => onChange(s)} style={{ fontFamily: `"${FAMILY[s]}"` }} className={cx("rounded px-2.5 py-0.5 text-[12.5px]", value === s ? "bg-selected font-semibold text-fg" : "text-fg-2 hover:bg-hover")}>
            {s[0].toUpperCase() + s.slice(1)}
          </button>
        ))}
      </div>
    </div>
  );
}

/**
 * Customise theme (mockup ②): the seven colours (picker + hex) and the two font
 * slots, previewed on the current slide, with a warning when text falls below
 * WCAG AA. Apply changes this deck only; Save as theme also keeps it in My
 * themes. Editing a saved theme updates it instead (renaming it too).
 */
export function ThemeEditor({ start, target, deck, slide, assetUrl, fontsReady, busy, error, onClose, onApply, onSave, onUpdate }: {
  start: Theme;
  target: ThemeEditTarget;
  deck: Deck;
  slide: Slide;
  assetUrl: (id: string) => string;
  fontsReady: boolean;
  busy: boolean;
  error?: string;
  onClose: () => void;
  onApply: (t: Theme) => void;
  onSave: (t: Theme) => void;
  onUpdate: (t: Theme) => void;
}) {
  const [theme, setTheme] = useState<Theme>(() => ({
    ...structuredClone(start),
    name: target.kind === "saved" ? start.name : `${start.name} (custom)`.slice(0, 60),
  }));
  const setColor = (c: ThemeColor, hex: string) => setTheme((t) => ({ ...t, colors: { ...t.colors, [c]: hex } }));
  const issues = contrastIssues(theme);
  const name = theme.name.trim();
  const named = { ...theme, name };
  return (
    <Dialog.Root open onOpenChange={(o) => !o && onClose()}>
      <Dialog.Portal>
        <Dialog.Overlay className="motion-scrim fixed inset-0 z-40 bg-scrim" />
        <Dialog.Content
          aria-describedby={undefined}
          data-testid="theme-editor"
          className="motion-dialog fixed left-1/2 top-[4vh] z-50 flex max-h-[92vh] w-[min(780px,calc(100%-32px))] -translate-x-1/2 flex-col overflow-y-auto rounded-2xl border border-border-float bg-surface-raised shadow-[var(--shadow-float)] outline-none"
        >
          <div className="flex items-start gap-2 px-6 pt-5">
            <div>
              <Dialog.Title className="type-heading">{target.kind === "saved" ? "Edit theme" : "Customise theme"}</Dialog.Title>
              <p className="mt-1 text-[13px] text-fg-3">
                {target.kind === "saved" ? `Changes update ${start.name} in My themes; decks already using it keep their copy.` : `Starting from ${start.name}. Changes preview on the current slide until you apply.`}
              </p>
            </div>
            <Dialog.Close asChild>
              <IconButton icon="x" label="Close" className="ml-auto" />
            </Dialog.Close>
          </div>
          <div className="px-6 pt-4">
            <SlideSvg deck={{ ...deck, theme }} slide={slide} assetUrl={assetUrl} fontsReady={fontsReady} className="block aspect-video w-full rounded-md border border-border-strong" label="Theme preview" />
          </div>
          <div className="px-6 pt-5">
            <h3 className="text-[11px] font-semibold uppercase tracking-wide text-fg-3">Colours</h3>
            <div className="mt-2 grid grid-cols-2 gap-x-8 gap-y-2.5">
              <div className="flex flex-col gap-2.5">{LEFT.map((c) => <ColorField key={c} name={c} value={theme.colors[c]} onChange={(h) => setColor(c, h)} />)}</div>
              <div className="flex flex-col gap-2.5">{RIGHT.map((c) => <ColorField key={c} name={c} value={theme.colors[c]} onChange={(h) => setColor(c, h)} />)}</div>
            </div>
            <h3 className="mt-5 text-[11px] font-semibold uppercase tracking-wide text-fg-3">Fonts</h3>
            <div className="mt-2 grid grid-cols-2 gap-x-8">
              <SlotPicker label="Heading" value={theme.fonts.heading} onChange={(s) => setTheme((t) => ({ ...t, fonts: { ...t.fonts, heading: s } }))} />
              <SlotPicker label="Body" value={theme.fonts.body} onChange={(s) => setTheme((t) => ({ ...t, fonts: { ...t.fonts, body: s } }))} />
            </div>
            {issues.length > 0 && (
              <div role="status" data-testid="contrast-warning" className="mt-4 rounded-md border border-[#E8A33D] bg-[#FFF7E6] px-3 py-2 text-[13px] text-[#7A4A00]">
                ⚠ {issues.map((i) => `${LABEL[i.fg]} on ${LABEL[i.bg]} is ${i.ratio.toFixed(1)}:1`).join(", ")}: below WCAG AA (4.5:1).
              </div>
            )}
          </div>
          {error && <p role="alert" className="px-6 pt-3 text-[13px] text-pill-blocked-fg">{error}</p>}
          <div className="mt-5 flex flex-wrap items-center gap-2 border-t border-border px-6 py-4">
            <label className="flex items-center gap-2 text-[13px] text-fg">
              Name
              <input aria-label="Theme name" value={theme.name} maxLength={60} onChange={(e) => setTheme((t) => ({ ...t, name: e.target.value }))} className="h-7 w-52 rounded-md border border-border-strong bg-surface px-2 outline-none focus-visible:ring-2 focus-visible:ring-accent" />
            </label>
            <div className="ml-auto flex gap-2">
              <Dialog.Close asChild>
                <Button variant="ghost">Cancel</Button>
              </Dialog.Close>
              {target.kind === "saved" ? (
                <Button disabled={busy || !name} onClick={() => onUpdate(named)}>Update theme</Button>
              ) : (
                <Button disabled={busy || !name} onClick={() => onSave(named)}>Save as theme</Button>
              )}
              <Button variant="primary" disabled={busy || !name} onClick={() => onApply(named)}>Apply to this deck</Button>
            </div>
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}

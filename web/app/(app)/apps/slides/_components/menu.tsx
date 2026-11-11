"use client";

/** Menu primitives the slide toolbar and its style menus share. */
import { DropdownMenu } from "radix-ui";
import { useId, useRef, useState, type ReactNode } from "react";
import { cx } from "@/lib/cx";
import { parseHex } from "@/lib/slides/edit";
import { THEME_COLORS, resolveColor, type ColorRef, type Theme } from "@/lib/slides/schema";
import { floatPanel } from "@/components/domain/pickers";

export const chip = "inline-flex h-7 items-center gap-1 rounded-md border border-border-strong px-2 text-[12.5px] text-fg-2 outline-none hover:bg-hover focus-visible:ring-2 focus-visible:ring-accent disabled:opacity-40 data-[state=open]:bg-hover";
export const on = "border-accent bg-selected text-fg";
export const item = "flex cursor-pointer select-none items-center gap-2 rounded-md px-2.5 py-1.5 text-[13px] text-fg outline-none data-[highlighted]:bg-hover";
export const field = "h-7 min-w-0 flex-1 rounded-md border border-border-strong bg-transparent px-2 text-[12.5px] text-fg outline-none focus-visible:ring-2 focus-visible:ring-accent";
export const menuSep = <DropdownMenu.Separator className="my-1 h-px bg-border-subtle" />;

/** Keeps focus where it is (the text being edited) when a button is pressed. */
export const keep = (e: React.MouseEvent) => e.preventDefault();

/** Keys typed into a control inside a menu stay there (no typeahead or arrow navigation); Escape still closes it. */
export const ownKeys = (e: React.KeyboardEvent) => {
  if (e.key !== "Escape") e.stopPropagation();
};

export function Menu({ label, trigger, children, disabled, open, onOpenChange, className }: { label: string; trigger: ReactNode; children: ReactNode; disabled?: boolean; open?: boolean; onOpenChange?: (o: boolean) => void; className?: string }) {
  return (
    <DropdownMenu.Root modal={false} open={open} onOpenChange={onOpenChange}>
      <DropdownMenu.Trigger asChild disabled={disabled}>
        <button type="button" aria-label={label} className={chip} onMouseDown={keep}>
          {trigger}
        </button>
      </DropdownMenu.Trigger>
      <DropdownMenu.Portal>
        <DropdownMenu.Content align="start" sideOffset={6} className={cx(floatPanel, "z-50 max-h-[70vh] min-w-40 overflow-y-auto", className)} onCloseAutoFocus={(e) => e.preventDefault()}>
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
export function MenuField<T>({ label, item: itemLabel, placeholder, parse, onApply, extra }: { label: string; item: string; placeholder?: string; parse: (raw: string) => { value: T } | { error: string }; onApply: (v: T) => void; extra?: (set: (v: string) => void) => ReactNode }) {
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

export const colorName = (c: ColorRef) => (c.startsWith("#") ? c.toUpperCase() : c === "accent2" ? "Accent 2" : c[0].toUpperCase() + c.slice(1));

/**
 * A colour list for a menu: "none" (if given), the theme colours, the deck's
 * recent custom colours and a field for any #RRGGBB. current (null for none)
 * is ticked. A custom colour is remembered, then onDone runs (to close the menu).
 */
export function ColorItems({ label, theme, onPick, none, current, recent, remember, onDone }: { label: string; theme: Theme; onPick: (c: ColorRef | undefined) => void; none?: string; current?: ColorRef | null; recent: string[]; remember: (c: string) => void; onDone?: () => void }) {
  const custom = (c: string) => {
    onPick(c);
    remember(c);
    onDone?.();
  };
  const tick = (on: boolean) => on && <span aria-hidden className="ml-auto pl-3">✓</span>;
  return (
    <>
      {none && (
        <DropdownMenu.Item className={cx(item, current === null && "font-semibold")} onSelect={() => onPick(undefined)}>
          <span className="size-4 rounded-sm border border-dashed border-border-strong" /> {none}
          {tick(current === null)}
        </DropdownMenu.Item>
      )}
      {THEME_COLORS.map((c) => (
        <DropdownMenu.Item key={c} className={cx(item, current === c && "font-semibold")} onSelect={() => onPick(c)}>
          <span className="size-4 rounded-sm border border-border-strong" style={{ background: resolveColor(theme, c) }} /> {colorName(c)}
          {tick(current === c)}
        </DropdownMenu.Item>
      ))}
      {menuSep}
      {recent.length > 0 && (
        <DropdownMenu.Group aria-label="Recent colours" className="flex max-w-52 flex-wrap gap-1 px-2.5 py-1">
          {recent.map((c) => (
            <DropdownMenu.Item key={c} aria-label={`Recent colour ${c}`} title={c} className={cx("size-5 cursor-pointer rounded-sm border border-border-strong outline-none data-[highlighted]:ring-2 data-[highlighted]:ring-accent", current?.toUpperCase() === c.toUpperCase() && "ring-2 ring-accent")} style={{ background: c }} onSelect={() => custom(c)} />
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
    </>
  );
}

/** A colour menu: ColorItems behind a chip, closed once a custom colour applies. */
export function ColorMenu({ label, trigger, theme, onPick, none, current, recent, remember }: { label: string; trigger: ReactNode; theme: Theme; onPick: (c: ColorRef | undefined) => void; none?: string; current?: ColorRef | null; recent: string[]; remember: (c: string) => void }) {
  const [open, setOpen] = useState(false);
  return (
    <Menu label={label} trigger={trigger} open={open} onOpenChange={setOpen}>
      <ColorItems label={label} theme={theme} onPick={onPick} none={none} current={current} recent={recent} remember={remember} onDone={() => setOpen(false)} />
    </Menu>
  );
}

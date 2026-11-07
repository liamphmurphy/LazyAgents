"use client";

import Link from "next/link";
import { useRef, type KeyboardEvent, type ReactNode } from "react";
import { cx } from "@/lib/cx";

/** Moves focus between sibling radios/tabs with the arrow keys. */
function useRovingKeys(count: number, current: number, select: (i: number) => void) {
  const refs = useRef<Array<HTMLElement | null>>([]);
  const onKeyDown = (e: KeyboardEvent) => {
    const delta = e.key === "ArrowRight" || e.key === "ArrowDown" ? 1 : e.key === "ArrowLeft" || e.key === "ArrowUp" ? -1 : 0;
    if (!delta) return;
    e.preventDefault();
    const next = (current + delta + count) % count;
    select(next);
    refs.current[next]?.focus();
  };
  return { refs, onKeyDown };
}

export function Segmented<T extends string>({
  options,
  value,
  onChange,
  label,
  small,
}: {
  options: ReadonlyArray<{ value: T; label: ReactNode }>;
  /** undefined (or a value not in options) leaves every option unchecked. */
  value: T | undefined;
  onChange: (v: T) => void;
  label: string;
  small?: boolean;
}) {
  const checked = options.findIndex((o) => o.value === value);
  const current = Math.max(0, checked);
  const { refs, onKeyDown } = useRovingKeys(options.length, current, (i) => onChange(options[i].value));
  return (
    <div
      role="radiogroup"
      aria-label={label}
      onKeyDown={onKeyDown}
      className="inline-flex gap-0.5 rounded-md border border-border bg-surface p-0.5"
    >
      {options.map((o, i) => {
        const on = i === checked;
        return (
          <button
            key={o.value}
            ref={(el) => {
              refs.current[i] = el;
            }}
            type="button"
            role="radio"
            aria-checked={on}
            tabIndex={i === current ? 0 : -1}
            onClick={() => onChange(o.value)}
            className={cx(
              "whitespace-nowrap rounded-[5px]",
              small ? "h-6 px-[9px] text-xs" : "h-[26px] px-[11px] text-[12.5px]",
              on ? "bg-segment-on font-medium text-fg" : "text-fg-secondary hover:text-fg",
            )}
          >
            {o.label}
          </button>
        );
      })}
    </div>
  );
}

export function Toggle({
  checked,
  onChange,
  label,
  disabled,
}: {
  checked: boolean;
  onChange: (v: boolean) => void;
  label: ReactNode;
  disabled?: boolean;
}) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      disabled={disabled}
      onClick={() => onChange(!checked)}
      className="inline-flex items-center gap-2 text-[12.5px] text-fg-secondary disabled:opacity-50"
    >
      <span className={cx("relative h-4 w-7 flex-none rounded-[8px] transition-colors", checked ? "bg-accent" : "bg-border-float")}>
        <span
          className={cx(
            "absolute top-0.5 size-3 rounded-full transition-[left]",
            checked ? "left-3.5 bg-on-accent" : "left-0.5 bg-fg-secondary",
          )}
        />
      </span>
      {label}
    </button>
  );
}

export type TabItem = { key: string; label: ReactNode; count?: number; href?: string };

/** View tabs. With hrefs they're links (each view is a URL); otherwise a tablist. */
export function Tabs({
  items,
  value,
  onChange,
  label,
}: {
  items: ReadonlyArray<TabItem>;
  value: string;
  onChange?: (key: string) => void;
  label: string;
}) {
  const current = Math.max(0, items.findIndex((t) => t.key === value));
  const { refs, onKeyDown } = useRovingKeys(items.length, current, (i) => onChange?.(items[i].key));
  const cls = (on: boolean) =>
    cx(
      "inline-flex h-7 items-center gap-1.5 whitespace-nowrap rounded-md border px-[11px] text-[12.5px]",
      on ? "border-border-float bg-selected font-medium text-fg" : "border-transparent text-fg-secondary hover:bg-hover hover:text-fg",
    );
  const inner = (t: TabItem) => (
    <>
      {t.label}
      {t.count != null && <span className="font-mono text-[11px] text-fg-tertiary">{t.count}</span>}
    </>
  );

  if (items.every((t) => t.href)) {
    return (
      <nav aria-label={label} className="flex flex-wrap gap-1">
        {items.map((t) => (
          <Link key={t.key} href={t.href!} aria-current={t.key === value ? "page" : undefined} className={cls(t.key === value)}>
            {inner(t)}
          </Link>
        ))}
      </nav>
    );
  }
  return (
    <div role="tablist" aria-label={label} onKeyDown={onKeyDown} className="flex flex-wrap gap-1">
      {items.map((t, i) => (
        <button
          key={t.key}
          ref={(el) => {
            refs.current[i] = el;
          }}
          type="button"
          role="tab"
          aria-selected={i === current}
          tabIndex={i === current ? 0 : -1}
          onClick={() => onChange?.(t.key)}
          className={cls(i === current)}
        >
          {inner(t)}
        </button>
      ))}
    </div>
  );
}

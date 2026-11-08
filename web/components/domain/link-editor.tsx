"use client";

import { useId, useRef, useState, type ComponentProps, type KeyboardEvent } from "react";
import type { LinkTarget } from "@/lib/api/client";
import { useLinkSuggest } from "@/lib/api/hooks";
import { cx } from "@/lib/cx";
import { linkTextFor } from "@/lib/wikilinks";
import { TextArea } from "@/components/ui/primitives";
import { floatPanel } from "./pickers";
import { TargetMark } from "./wikilink";

/** The open "[[query" before the caret on the current line, if any. */
function openLink(text: string, caret: number): { start: number; query: string } | null {
  const lineStart = text.lastIndexOf("\n", caret - 1) + 1;
  const before = text.slice(lineStart, caret);
  const at = before.lastIndexOf("[[");
  if (at < 0) return null;
  const query = before.slice(at + 2);
  if (query.includes("]]") || query.includes("|") || query.length > 80) return null;
  return { start: lineStart + at + 2, query };
}

export function kindLabel(t: LinkTarget) {
  if (t.kind === "artifact") return t.artifact_type === "note" ? "Page" : (t.artifact_type ?? "").toUpperCase();
  return t.kind === "ticket" ? "Ticket" : t.kind === "agent" ? "Agent" : "Epic";
}

/** The suggestion listbox, shared by the [[ autocomplete and the "Add context" picker. */
export function SuggestionList({
  id,
  items,
  active,
  onActive,
  onPick,
  className,
  label = "Link to",
}: {
  id: string;
  items: LinkTarget[];
  active: number;
  onActive: (i: number) => void;
  onPick: (t: LinkTarget) => void;
  className?: string;
  label?: string;
}) {
  return (
    <div id={id} role="listbox" aria-label={label} hidden={items.length === 0} className={cx(floatPanel, "max-h-72 overflow-y-auto", className)}>
      {items.map((t, i) => (
        <div
          key={`${t.kind}:${t.id}:${t.anchor ?? ""}`}
          id={`${id}-${i}`}
          role="option"
          aria-selected={i === active}
          onMouseEnter={() => onActive(i)}
          onMouseDown={(e) => {
            e.preventDefault();
            onPick(t);
          }}
          className={cx("flex h-9 cursor-default items-center gap-2.5 rounded-md px-2", i === active && "bg-hover")}
        >
          <TargetMark t={t} size={13} />
          <span className="min-w-0 flex-1 truncate">
            {t.display_id && <span className="mr-1.5 font-mono text-xs text-fg-secondary">{t.display_id}</span>}
            {t.kind === "artifact" && t.artifact_type === "note" ? t.path : t.title}
          </span>
          <span className="text-[11px] text-fg-tertiary">{kindLabel(t)}</span>
        </div>
      ))}
    </div>
  );
}

/**
 * Markdown textarea with [[ autocomplete over pages, artifacts, tickets,
 * epics and agents (docs/dev/plan.md §12.6). ↑↓ to move, ↵ or Tab to insert.
 */
export function LinkEditor({
  value,
  onChange,
  onKeyDown,
  className,
  mono = true,
  onPick,
  accept,
  inputRef,
  onBlur,
  ...rest
}: {
  value: string;
  onChange: (v: string) => void;
  mono?: boolean;
  /** Pick a suggestion as a chip instead of inserting [[link]] text: the "[[query" is removed. */
  onPick?: (t: LinkTarget) => void;
  /** Limits which suggestions are offered. */
  accept?: (t: LinkTarget) => boolean;
  inputRef?: (el: HTMLTextAreaElement | null) => void;
} & Omit<ComponentProps<"textarea">, "value" | "onChange">) {
  const ref = useRef<HTMLTextAreaElement | null>(null);
  const listId = useId();
  const [open, setOpen] = useState<{ start: number; query: string } | null>(null);
  const [active, setActive] = useState(0);
  const suggest = useLinkSuggest(open?.query ?? "", !!open);
  const items = open ? (suggest.data ?? []).filter((t) => !accept || accept(t)) : [];
  const shown = open && items.length > 0;

  const sync = (text: string, caret: number) => {
    const o = openLink(text, caret);
    setOpen(o);
    if (o?.query !== open?.query) setActive(0);
  };

  const insert = (t: LinkTarget) => {
    const el = ref.current;
    if (!el || !open) return;
    const caret = el.selectionStart;
    const after = value.slice(caret);
    const closing = after.startsWith("]]") ? 2 : 0;
    if (onPick) {
      const cut = open.start - 2;
      onChange(value.slice(0, cut) + after.slice(closing));
      setOpen(null);
      onPick(t);
      requestAnimationFrame(() => {
        el.focus();
        el.setSelectionRange(cut, cut);
      });
      return;
    }
    const link = linkTextFor(t);
    const next = value.slice(0, open.start) + link + "]]" + after.slice(closing);
    onChange(next);
    setOpen(null);
    const pos = open.start + link.length + 2;
    requestAnimationFrame(() => {
      el.focus();
      el.setSelectionRange(pos, pos);
    });
  };

  const keys = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (shown) {
      if (e.key === "ArrowDown" || e.key === "ArrowUp") {
        e.preventDefault();
        setActive((a) => (a + (e.key === "ArrowDown" ? 1 : items.length - 1)) % items.length);
        return;
      }
      if (e.key === "Enter" || e.key === "Tab") {
        e.preventDefault();
        insert(items[active]);
        return;
      }
      if (e.key === "Escape") {
        e.preventDefault();
        e.stopPropagation();
        setOpen(null);
        return;
      }
    }
    onKeyDown?.(e);
  };

  return (
    <div className="relative">
      <TextArea
        ref={(el) => {
          ref.current = el;
          inputRef?.(el);
        }}
        mono={mono}
        value={value}
        role="combobox"
        aria-expanded={!!shown}
        aria-controls={listId}
        aria-autocomplete="list"
        aria-activedescendant={shown ? `${listId}-${active}` : undefined}
        onChange={(e) => {
          onChange(e.target.value);
          sync(e.target.value, e.target.selectionStart);
        }}
        onKeyDown={keys}
        onKeyUp={(e) => {
          if (["ArrowLeft", "ArrowRight", "Home", "End"].includes(e.key)) sync(value, e.currentTarget.selectionStart);
        }}
        onClick={(e) => sync(value, e.currentTarget.selectionStart)}
        onBlur={(e) => {
          setTimeout(() => setOpen(null), 120);
          onBlur?.(e);
        }}
        className={className}
        {...rest}
      />
      <SuggestionList
        id={listId}
        items={shown ? items : []}
        active={active}
        onActive={setActive}
        onPick={insert}
        className="absolute inset-x-2 bottom-2"
      />
    </div>
  );
}

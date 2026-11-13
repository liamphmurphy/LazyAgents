"use client";

import { useRouter } from "next/navigation";
import { useDeferredValue, useState, type KeyboardEvent } from "react";
import { cx } from "@/lib/cx";
import { pageHref } from "@/lib/knowledge";
import { useCreatePage, usePages, type PageRow } from "@/lib/api/knowledge-hooks";
import { Icon } from "@/components/ui/icon";
import { Kbd } from "@/components/ui/primitives";
import { AUDIENCE, Modal } from "./kit";

/** The title with the typed text emphasised where it first matches. */
function Match({ text, q }: { text: string; q: string }) {
  const i = q ? text.toLowerCase().indexOf(q.toLowerCase()) : -1;
  if (i < 0) return <>{text}</>;
  return (
    <>
      {text.slice(0, i)}
      <span className="font-bold text-[#B9AEFF] underline">{text.slice(i, i + q.length)}</span>
      {text.slice(i + q.length)}
    </>
  );
}

/**
 * ⌘O: go to a page by name, path, alias or #tag, or create it. A query
 * starting with # searches tags.
 */
export function QuickSwitcher({ open, onOpenChange, includeAgent }: { open: boolean; onOpenChange: (o: boolean) => void; includeAgent: boolean }) {
  return (
    <Modal open={open} onOpenChange={onOpenChange} title="Go to a page" hideTitle>
      {/* Closing unmounts the search, so it opens empty each time. */}
      <Switcher onClose={() => onOpenChange(false)} includeAgent={includeAgent} />
    </Modal>
  );
}

function Switcher({ onClose, includeAgent }: { onClose: () => void; includeAgent: boolean }) {
  const router = useRouter();
  const [q, setQ] = useState("");
  const typed = useDeferredValue(q.trim());
  // The highlighted row, for the query it was picked on: typing starts again at the top.
  const [pick, setPick] = useState({ q: "", i: 0 });
  const at = pick.q === typed ? pick.i : 0;
  const setAt = (f: (i: number) => number) => setPick({ q: typed, i: f(at) });
  const tag = typed.startsWith("#") ? typed.slice(1) : "";
  const results = usePages({ text: tag ? undefined : typed || undefined, tag: tag || undefined, include_agent: includeAgent, limit: 8 });
  const rows = results.data?.rows ?? [];
  const create = useCreatePage();
  const canCreate = !!typed && !tag && !rows.some((r) => r.title.toLowerCase() === typed.toLowerCase());
  const count = rows.length + (canCreate ? 1 : 0);

  const go = (r: PageRow, newTab = false) => {
    onClose();
    if (newTab) window.open(pageHref(r.id, r.title), "_blank");
    else router.push(pageHref(r.id, r.title));
  };
  const make = () =>
    create.mutate(
      { title: typed, path: `Inbox/${typed.replace(/[[\]#|^/\\]/g, " ").trim()}`, body_md: "" },
      {
        onSuccess: (p) => {
          onClose();
          router.push(pageHref(p.id, p.title));
        },
      },
    );
  const onKey = (e: KeyboardEvent) => {
    if (e.key === "ArrowDown") {
      e.preventDefault();
      setAt((i) => Math.min(count - 1, i + 1));
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      setAt((i) => Math.max(0, i - 1));
    } else if (e.key === "Enter") {
      e.preventDefault();
      if (e.shiftKey && canCreate) make();
      else if (rows[at]) go(rows[at], e.metaKey || e.ctrlKey);
      else if (canCreate) make();
    }
  };

  return (
    <>
      <div className="flex items-center gap-3 border-b border-border px-4">
        <Icon name="search" size={17} className="text-fg-secondary" />
        <input
          autoFocus
          value={q}
          onChange={(e) => setQ(e.target.value)}
          onKeyDown={onKey}
          placeholder="Find or create a page…  (#tag searches tags)"
          aria-label="Find or create a page"
          className="h-14 flex-1 bg-transparent text-[17px] text-fg outline-none placeholder:text-fg-tertiary"
        />
        <Kbd>esc</Kbd>
      </div>
      <ul role="listbox" aria-label="Pages" className="max-h-[50vh] overflow-y-auto p-2">
        {rows.map((r, i) => (
          <li key={r.id} role="option" aria-selected={i === at}>
            <button
              onMouseEnter={() => setAt(() => i)}
              onClick={() => go(r)}
              className={cx("flex w-full items-center gap-3 rounded-lg px-3 py-2 text-left", i === at && "bg-selected")}
            >
              <Icon name={r.properties?.query ? "board" : "artifacts"} size={15} className={i === at ? "text-[#B9AEFF]" : "text-fg-tertiary"} />
              <span className="min-w-0 flex-1">
                <span className="block truncate text-sm text-fg">
                  <Match text={r.title} q={tag ? "" : typed} />
                </span>
                <span className="block truncate text-xs text-fg-tertiary">{r.path.split("/").slice(0, -1).join(" / ") || "Top level"}</span>
              </span>
              {i === at && <Kbd>↵</Kbd>}
              <Icon name={AUDIENCE[r.audience].icon} size={13} className="text-fg-tertiary" />
            </button>
          </li>
        ))}
        {canCreate && (
          <li role="option" aria-selected={at === rows.length}>
            <button
              onMouseEnter={() => setAt(() => rows.length)}
              onClick={make}
              disabled={create.isPending}
              className={cx("mt-1 flex w-full items-center gap-3 rounded-lg border border-dashed border-border px-3 py-2.5 text-left text-sm", at === rows.length && "bg-selected")}
            >
              <Icon name="plus" size={15} className="text-[#B9AEFF]" />
              <span className="flex-1 text-fg-2">
                Create <b className="text-fg">“{typed}”</b> <span className="text-fg-tertiary">in Inbox</span>
              </span>
              <Kbd>⇧↵</Kbd>
            </button>
          </li>
        )}
        {!rows.length && !canCreate && <li className="px-3 py-6 text-center text-xs text-fg-tertiary">{results.isPending ? "Searching…" : "Nothing matches."}</li>}
      </ul>
      <div className="flex items-center gap-4 border-t border-border px-4 py-2 text-[11px] text-fg-tertiary">
        <span><Kbd>↑↓</Kbd> navigate</span>
        <span><Kbd>↵</Kbd> open</span>
        <span><Kbd>⌘↵</Kbd> new tab</span>
        <span><Kbd>⇧↵</Kbd> create</span>
        <span><Kbd>#</Kbd> tags</span>
      </div>
    </>
  );
}

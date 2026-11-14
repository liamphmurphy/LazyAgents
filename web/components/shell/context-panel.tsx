"use client";

// The right-hand Context panel shared by the run page and the epic page
// (LAZYAGENTS-163, LAZYAGENTS-171): a titled aside of sections, closable, its
// open/closed state remembered per browser and per page kind.

import { useEffect, useState, useSyncExternalStore, type ReactNode } from "react";
import { Button } from "@/components/ui/button";
import { Icon } from "@/components/ui/icon";
import { Tabs } from "@/components/ui/controls";
import { cx } from "@/lib/cx";

const CHANGED = "lazyagents:context-panel";

function subscribe(onChange: () => void) {
  window.addEventListener("storage", onChange);
  window.addEventListener(CHANGED, onChange);
  return () => {
    window.removeEventListener("storage", onChange);
    window.removeEventListener(CHANGED, onChange);
  };
}

/**
 * Open (the default) or closed, remembered in localStorage under `storageKey`
 * ("closed" when closed, absent when open).
 */
export function useContextPanel(storageKey: string): [boolean, (open: boolean) => void] {
  const read = () => {
    try {
      return window.localStorage.getItem(storageKey) !== "closed";
    } catch {
      return true;
    }
  };
  const open = useSyncExternalStore(subscribe, read, () => true);
  const set = (next: boolean) => {
    try {
      if (next) window.localStorage.removeItem(storageKey);
      else window.localStorage.setItem(storageKey, "closed");
    } catch {
      /* Not remembered. */
    }
    window.dispatchEvent(new Event(CHANGED));
  };
  return [open, set];
}

/**
 * `header` replaces the "Context" title, e.g. with a switch between the facts and the work chat.
 * `fill` gives a child that scrolls itself (a conversation) the panel's height, and room for it.
 */
export function ContextPanel({ onClose, header, fill, children }: { onClose: () => void; header?: ReactNode; fill?: boolean; children: ReactNode }) {
  return (
    <aside aria-label="Context" className={cx("flex flex-none flex-col border-l border-border-subtle bg-sidebar/40", fill ? "w-[360px] overflow-hidden max-lg:w-[300px]" : "w-[320px] overflow-y-auto max-lg:w-[280px]")}>
      <div className="flex items-center justify-between gap-2 px-4 pb-1 pt-3">
        {header ?? <span className="text-13 font-semibold">Context</span>}
        <button type="button" onClick={onClose} aria-label="Hide context" title="Hide context" className="grid size-7 place-items-center rounded-md text-fg-tertiary hover:bg-hover hover:text-fg">
          <Icon name="sidebar" size={14} />
        </button>
      </div>
      {children}
    </aside>
  );
}

export function ContextSection({ title, aside, children }: { title: string; aside?: ReactNode; children: ReactNode }) {
  return (
    <section aria-label={title} className="flex flex-col gap-2 border-b border-border-subtle px-4 py-3.5 last:border-b-0">
      <h2 className="flex items-center justify-between text-12 font-medium text-fg-tertiary">
        {title}
        {aside && <span className="font-mono">{aside}</span>}
      </h2>
      {children}
    </section>
  );
}

/** Shown in the top bar when the panel is closed. */
export function ContextButton({ onOpen }: { onOpen: () => void }) {
  return <Button variant="outline" icon="sidebar" onClick={onOpen}>Context</Button>;
}

/**
 * Whether a side panel shows its work chat (true) or its own facts (false), remembered per browser
 * under `storageKey`: a run's and a ticket's panels each remember theirs.
 */
export function useChatTab(storageKey: string): [boolean, (chat: boolean) => void] {
  const [chat, setChat] = useState(false);
  useEffect(() => {
    try {
      // eslint-disable-next-line react-hooks/set-state-in-effect -- reading browser-only storage after hydration
      if (window.localStorage.getItem(storageKey) === "chat") setChat(true);
    } catch {
      /* No storage: the facts. */
    }
  }, [storageKey]);
  const set = (next: boolean) => {
    setChat(next);
    try {
      if (next) window.localStorage.setItem(storageKey, "chat");
      else window.localStorage.removeItem(storageKey);
    } catch {
      /* Not remembered. */
    }
  };
  return [chat, set];
}

/** The switch at the top of a side panel: its own facts (`first`), or Chat with your persona. */
export function PanelTabs({ first, chat, onChat }: { first: string; chat: boolean; onChat: (chat: boolean) => void }) {
  return <Tabs label="Side panel" value={chat ? "chat" : "facts"} onChange={(k) => onChat(k === "chat")} items={[{ key: "facts", label: first }, { key: "chat", label: "Chat" }]} />;
}

"use client";

// The right-hand Context panel shared by the run page and the epic page
// (LAZYAGENTS-163, LAZYAGENTS-171): a titled aside of sections, closable, its
// open/closed state remembered per browser and per page kind.

import { useSyncExternalStore, type ReactNode } from "react";
import { Button } from "@/components/ui/button";
import { Icon } from "@/components/ui/icon";

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

export function ContextPanel({ onClose, children }: { onClose: () => void; children: ReactNode }) {
  return (
    <aside aria-label="Context" className="flex w-[320px] flex-none flex-col overflow-y-auto border-l border-border-subtle bg-sidebar/40 max-lg:w-[280px]">
      <div className="flex items-center justify-between px-4 pb-1 pt-3">
        <span className="text-13 font-semibold">Context</span>
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

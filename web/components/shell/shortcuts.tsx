"use client";

import { Dialog } from "radix-ui";
import { Fragment, useEffect, useState } from "react";
import { IconButton } from "@/components/ui/button";
import { Kbd } from "@/components/ui/primitives";

const OPEN = "la:shortcuts";

/** Open the overlay from a button (the Inbox's "Shortcuts"). */
export function openShortcuts() {
  window.dispatchEvent(new Event(OPEN));
}

type Row = { keys: string[]; label: string };

const SECTIONS: ReadonlyArray<{ title: string; rows: Row[] }> = [
  {
    title: "Inbox",
    rows: [
      { keys: ["J", "K"], label: "Next / previous item" },
      { keys: ["↵"], label: "Open the item’s page" },
      { keys: ["A"], label: "Approve (approvals, mockups)" },
      { keys: ["D"], label: "Deny (approvals)" },
      { keys: ["R"], label: "Reply (questions) or focus the note" },
      { keys: ["E"], label: "Mark done: hide until it changes" },
      { keys: ["⌘", "↵"], label: "Send the reply or note" },
      { keys: ["esc"], label: "Leave the text box, back to the list" },
    ],
  },
  {
    title: "Global",
    rows: [
      { keys: ["⌘", "K"], label: "Command menu" },
      { keys: ["?"], label: "This overlay" },
      { keys: ["C"], label: "New ticket" },
    ],
  },
];

function isTyping(el: EventTarget | null) {
  return el instanceof HTMLElement && (el.isContentEditable || ["INPUT", "TEXTAREA", "SELECT"].includes(el.tagName));
}

/** Keyboard shortcut overlay, opened with "?" anywhere outside a text box. */
export function ShortcutsOverlay() {
  const [open, setOpen] = useState(false);
  useEffect(() => {
    const show = () => setOpen(true);
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== "?" || e.metaKey || e.ctrlKey || e.altKey || isTyping(e.target)) return;
      if (document.querySelector('[role="dialog"]') && !open) return;
      e.preventDefault();
      setOpen((v) => !v);
    };
    window.addEventListener(OPEN, show);
    window.addEventListener("keydown", onKey);
    return () => {
      window.removeEventListener(OPEN, show);
      window.removeEventListener("keydown", onKey);
    };
  }, [open]);
  return (
    <Dialog.Root open={open} onOpenChange={setOpen}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-40 bg-scrim" />
        <Dialog.Content
          aria-describedby="shortcuts-desc"
          className="fixed left-1/2 top-[10vh] z-50 flex max-h-[80vh] w-[min(420px,calc(100%-32px))] -translate-x-1/2 flex-col overflow-hidden rounded-xl border border-border-float bg-surface-raised shadow-[var(--shadow-float)] outline-none"
        >
          <div className="flex items-center justify-between border-b border-border-subtle px-5 py-3">
            <Dialog.Title className="type-heading">Keyboard shortcuts</Dialog.Title>
            <Dialog.Close asChild>
              <IconButton icon="x" label="Close" />
            </Dialog.Close>
          </div>
          <div className="flex flex-col gap-5 overflow-y-auto px-5 py-4">
            {SECTIONS.map((s) => (
              <section key={s.title} aria-label={`${s.title} shortcuts`}>
                <h3 className="mb-2 text-[10px] font-semibold uppercase tracking-[0.06em] text-fg-tertiary">{s.title}</h3>
                <dl className="grid grid-cols-[64px_1fr] items-center gap-x-3 gap-y-2 text-[13px]">
                  {s.rows.map((r) => (
                    <Fragment key={r.label}>
                      <dt className="flex gap-1">
                        {r.keys.map((k) => (
                          <Kbd key={k}>{k}</Kbd>
                        ))}
                      </dt>
                      <dd className="text-fg-2">{r.label}</dd>
                    </Fragment>
                  ))}
                </dl>
              </section>
            ))}
            <p id="shortcuts-desc" className="text-xs text-fg-tertiary">
              Inbox keys are off while typing in a text box.
            </p>
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}

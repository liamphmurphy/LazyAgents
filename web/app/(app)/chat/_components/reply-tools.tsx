"use client";

import { useEffect, useRef, useState, type RefObject } from "react";
import { Button } from "@/components/ui/button";
import { Icon } from "@/components/ui/icon";

/** Copies text, and says so for a moment. */
export function CopyButton({ text, label = "Copy" }: { text: string; label?: string }) {
  const [copied, setCopied] = useState(false);
  return (
    <Button
      variant="ghost"
      icon={copied ? "check" : "link"}
      onClick={() => {
        void navigator.clipboard?.writeText(text).then(() => {
          setCopied(true);
          setTimeout(() => setCopied(false), 1500);
        });
      }}
    >
      {copied ? "Copied" : label}
    </Button>
  );
}

const QUOTE_MAX = 1500;

/** The composer text that quotes a selection, routed to its author when someone else would answer. */
export function quoteForComposer(selected: string, mention?: string): string {
  const text = selected.trim().slice(0, QUOTE_MAX);
  return `${mention ? `@${mention} ` : ""}${mention ? "\n\n" : ""}${text.split("\n").map((l) => `> ${l}`).join("\n")}\n\n`;
}

type Picked = { x: number; y: number; text: string; author?: string; authorId?: string };

/** A floating "Ask about this" over text selected in a reply. */
export function AskAboutSelection({ root, onAsk }: { root: RefObject<HTMLElement | null>; onAsk: (text: string, authorId?: string, author?: string) => void }) {
  const [picked, setPicked] = useState<Picked | null>(null);
  const button = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    const update = () => {
      const sel = document.getSelection();
      const text = sel?.toString().trim();
      const from = sel && sel.rangeCount > 0 ? sel.getRangeAt(0).commonAncestorContainer : null;
      const el = from instanceof Element ? from : from?.parentElement;
      const reply = el?.closest<HTMLElement>('[data-testid="assistant-message"]');
      if (!text || !reply || !root.current?.contains(reply)) return setPicked(null);
      const r = sel!.getRangeAt(0).getBoundingClientRect();
      setPicked({ x: r.left + r.width / 2, y: r.top, text, author: reply.dataset.author, authorId: reply.dataset.authorId });
    };
    document.addEventListener("selectionchange", update);
    return () => document.removeEventListener("selectionchange", update);
  }, [root]);
  if (!picked) return null;
  return (
    <button
      ref={button}
      type="button"
      data-testid="ask-about-selection"
      style={{ left: picked.x, top: Math.max(picked.y - 36, 8) }}
      // Keep the selection: a press would otherwise clear it before the click.
      onMouseDown={(e) => e.preventDefault()}
      onClick={() => {
        onAsk(picked.text, picked.authorId, picked.author);
        document.getSelection()?.removeAllRanges();
      }}
      className="fixed z-30 flex -translate-x-1/2 items-center gap-1.5 rounded-lg border border-border-float bg-surface-raised px-2.5 py-1 text-xs font-medium text-fg shadow-[var(--shadow-float)] hover:bg-hover"
    >
      <Icon name="msg" size={13} />
      Ask about this
    </button>
  );
}

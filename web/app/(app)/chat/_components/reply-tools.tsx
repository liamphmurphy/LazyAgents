"use client";

import { useEffect, useRef, useState, type RefObject } from "react";
import { Button } from "@/components/ui/button";
import { Icon } from "@/components/ui/icon";
import { cx } from "@/lib/cx";

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

/** On for one message: asks for a research plan to approve instead of an answer. */
export function ResearchToggle({ on, onChange, disabled }: { on: boolean; onChange: (on: boolean) => void; disabled?: boolean }) {
  return (
    <button
      type="button"
      aria-pressed={on}
      disabled={disabled}
      title="Plan a multi-source research report instead of answering right away"
      onClick={() => onChange(!on)}
      className={cx("inline-flex h-7 items-center gap-1.5 rounded-full border px-2.5 text-xs font-medium disabled:opacity-50", on ? "border-accent bg-accent-soft text-accent" : "border-border text-fg-secondary hover:text-fg")}
    >
      <Icon name="search" size={13} />
      Research
    </button>
  );
}

const QUOTE_MAX = 1500;

/** The composer text that quotes a selection, routed to its author when someone else would answer. */
export function quoteForComposer(selected: string, mention?: string): string {
  const text = selected.trim().slice(0, QUOTE_MAX);
  return `${mention ? `@${mention} ` : ""}${mention ? "\n\n" : ""}${text.split("\n").map((l) => `> ${l}`).join("\n")}\n\n`;
}

type Picked = { x: number; y: number; text: string; author?: string; authorId?: string };

/** A floating "Ask about this" over text selected in a reply, which opens a small box to ask right there. */
export function AskAboutSelection({ root, onAsk }: { root: RefObject<HTMLElement | null>; onAsk: (text: string, question: string, authorId?: string, author?: string) => boolean }) {
  const [picked, setPicked] = useState<Picked | null>(null);
  const [asking, setAsking] = useState(false);
  const [question, setQuestion] = useState("");
  const box = useRef<HTMLDivElement>(null);
  const close = () => {
    setPicked(null);
    setAsking(false);
    setQuestion("");
    document.getSelection()?.removeAllRanges();
  };
  useEffect(() => {
    // Focusing the box clears the page selection, which mustn't close it.
    if (asking) return;
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
  }, [root, asking]);
  useEffect(() => {
    if (!asking) return;
    const outside = (e: MouseEvent) => {
      if (box.current?.contains(e.target as Node)) return;
      setPicked(null);
      setAsking(false);
      setQuestion("");
    };
    document.addEventListener("mousedown", outside);
    return () => document.removeEventListener("mousedown", outside);
  }, [asking]);
  if (!picked) return null;
  if (asking) {
    return (
      <div
        ref={box}
        data-testid="ask-about-selection-box"
        // Half the box's width from either edge, so it stays on screen.
        style={{ left: Math.min(Math.max(picked.x, 168), window.innerWidth - 168), top: Math.max(picked.y - 44, 8) }}
        className="fixed z-30 flex w-80 -translate-x-1/2 items-center gap-1.5 rounded-lg border border-border-float bg-surface-raised px-2.5 py-1.5 shadow-[var(--shadow-float)]"
      >
        <Icon name="msg" size={13} />
        <input
          autoFocus
          aria-label="Ask about this"
          placeholder="Ask about this…"
          value={question}
          onChange={(e) => setQuestion(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Escape") {
              e.preventDefault();
              close();
            }
            if (e.key === "Enter" && !e.nativeEvent.isComposing) {
              e.preventDefault();
              if (question.trim() && onAsk(picked.text, question.trim(), picked.authorId, picked.author)) close();
            }
          }}
          className="min-w-0 flex-1 bg-transparent text-[13px] text-fg outline-none placeholder:text-fg-tertiary"
        />
      </div>
    );
  }
  return (
    <button
      type="button"
      data-testid="ask-about-selection"
      style={{ left: picked.x, top: Math.max(picked.y - 36, 8) }}
      // Keep the selection: a press would otherwise clear it before the click.
      onMouseDown={(e) => e.preventDefault()}
      onClick={() => setAsking(true)}
      className="fixed z-30 flex -translate-x-1/2 items-center gap-1.5 rounded-lg border border-border-float bg-surface-raised px-2.5 py-1 text-xs font-medium text-fg shadow-[var(--shadow-float)] hover:bg-hover"
    >
      <Icon name="msg" size={13} />
      Ask about this
    </button>
  );
}

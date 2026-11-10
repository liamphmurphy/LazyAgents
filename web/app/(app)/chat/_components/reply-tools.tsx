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

/** The composer's frame: the message box and its options share one border, which takes the focus ring. */
export const composerFrame =
  "flex flex-col gap-2.5 rounded-[18px] border border-border-strong bg-surface p-2.5 focus-within:border-accent focus-within:ring-2 focus-within:ring-accent/30";
/** The message box inside the frame: the frame draws the border and focus, so the box doesn't. */
export const composerInput = "!border-0 !bg-transparent !px-2 !py-1 !text-[15px] focus-visible:!outline-none";
/** A round 30px icon button in the composer's option row (add context, attach). */
export const composerIconButton =
  "grid size-[30px] flex-none place-items-center rounded-full border border-border-strong text-fg-secondary hover:bg-hover hover:text-fg disabled:cursor-not-allowed disabled:opacity-50";
/** A pill in the option row; `on` marks a toggle that's set. */
export const composerPill = (on = false) =>
  cx(
    "inline-flex h-[30px] max-w-full items-center gap-1.5 rounded-full border px-3 text-[12.5px] font-medium disabled:cursor-not-allowed disabled:opacity-50",
    on ? "border-accent bg-accent-soft text-accent" : "border-border text-fg-secondary hover:bg-hover hover:text-fg",
  );
/** Model and effort: plain text in the option row, since they rarely change. */
export const composerGhost =
  "flex h-[30px] max-w-full items-center gap-1 rounded-full px-2 text-[12.5px] text-fg-secondary hover:bg-hover hover:text-fg disabled:cursor-not-allowed disabled:opacity-50";

/** The round send button. Its name says what pressing it does: Send, Plan it or Interrupt. */
export function SendButton({ label, disabled, title }: { label: string; disabled?: boolean; title?: string }) {
  return (
    <button
      type="submit"
      aria-label={label}
      title={title ?? label}
      disabled={disabled}
      className="grid size-[34px] flex-none place-items-center rounded-full bg-accent text-on-accent hover:brightness-110 disabled:cursor-not-allowed disabled:bg-border-strong disabled:text-fg-tertiary"
    >
      <svg aria-hidden viewBox="0 0 16 16" width={16} height={16} fill="none" stroke="currentColor" strokeWidth={1.9} strokeLinecap="round" strokeLinejoin="round">
        <path d="M8 13V3M3.5 7.5 8 3l4.5 4.5" />
      </svg>
    </button>
  );
}

/** What the message box understands, under it rather than in its placeholder. */
export function ComposerHints({ commands = true }: { commands?: boolean }) {
  return (
    <p className="text-center text-xs text-fg-tertiary" data-testid="composer-hints">
      {commands ? "/ commands  ·  @ personas  ·  [[ context  ·  " : ""}Shift+Enter for a new line
    </p>
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
      className={composerPill(on)}
    >
      <Icon name="search" size={13} />
      Research
    </button>
  );
}

/** Before the first message: whether the chat runs without the person's memory and past chats, and is deleted once idle. */
export function TemporaryToggle({ on, onChange, disabled }: { on: boolean; onChange: (on: boolean) => void; disabled?: boolean }) {
  return (
    <button
      type="button"
      aria-pressed={on}
      disabled={disabled}
      title="Temporary chat: no memory, no past chats, and it's deleted after a day without messages"
      onClick={() => onChange(!on)}
      className={composerPill(on)}
    >
      <Icon name="clock" size={13} />
      Temporary
    </button>
  );
}

/** A temporary chat's marker, in its header and the chat list. */
export function TemporaryBadge() {
  return (
    <span title="Temporary chat: no memory, no past chats, and it's deleted after a day without messages" className="inline-flex flex-none items-center gap-1 rounded-full bg-selected px-2 py-0.5 text-xs text-fg-secondary">
      <Icon name="clock" size={11} />
      Temporary
    </span>
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

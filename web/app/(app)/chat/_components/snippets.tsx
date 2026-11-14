"use client";

import { useRef, useState, type RefObject, type UIEvent } from "react";
import { Popover } from "radix-ui";
import { langFromFence } from "@/lib/code-diff";
import { cx } from "@/lib/cx";
import { useHighlighted } from "@/lib/highlight";
import { detectLanguage, isBigPaste, lineCount, pastedSnippetName, SNIPPET_LANGS, snippetLangLabel, type Snippet } from "@/lib/snippets";
import { Icon } from "@/components/ui/icon";
import { composerPill } from "./reply-tools";

/**
 * Code and Markdown snippets for the next message. Each waits as a chip and is sent as a
 * fenced block after the typed text (messageWithSnippets). The parent owns them, like files,
 * so it can clear them once the message is sent.
 */
export function useSnippets() {
  const [items, setItems] = useState<Snippet[]>([]);
  const next = useRef(0);
  return {
    items,
    add: (text: string, lang: string) => setItems((xs) => [...xs, { key: next.current++, text, lang }]),
    update: (key: number, text: string, lang: string) => setItems((xs) => xs.map((x) => (x.key === key ? { ...x, text, lang } : x))),
    remove: (key: number) => setItems((xs) => xs.filter((x) => x.key !== key)),
    clear: () => setItems([]),
  };
}

export type Snippets = ReturnType<typeof useSnippets>;

/** What the bubble is editing: a new snippet, or the chip with `key`. `picked` stops detection overriding the person's choice. */
export type SnippetDraft = { key?: number; text: string; lang: string; picked: boolean };

export const newSnippetDraft = (text = ""): SnippetDraft => ({ text, lang: detectLanguage(text), picked: false });

/** A File for a paste too long for the message, named with an extension attachments accept. */
export function snippetFile(text: string, lang: string, n: number): File {
  return new File([text], pastedSnippetName(lang, n), { type: lang === "json" ? "application/json" : lang === "markdown" ? "text/markdown" : "text/plain" });
}

/** The option row's { } Code pill: opens and closes the bubble over the composer. */
export function CodeButton({ open, onToggle, disabled, count }: { open: boolean; onToggle: () => void; disabled?: boolean; count: number }) {
  return (
    <button
      type="button"
      aria-label="Add code or Markdown"
      aria-expanded={open}
      title="Paste code or Markdown as a block"
      disabled={disabled}
      onClick={onToggle}
      data-snippet-trigger=""
      data-testid="snippet-button"
      className={composerPill(open)}
    >
      <Icon name="code" size={14} />
      Code{count > 0 && <span className="text-fg-tertiary">· {count}</span>}
    </button>
  );
}

/** Snippets waiting above the composer: a click reopens the bubble to edit one. */
export function SnippetChips({ snippets, onEdit, disabled }: { snippets: Snippets; onEdit: (s: Snippet) => void; disabled?: boolean }) {
  if (snippets.items.length === 0) return null;
  return (
    <ul aria-label="Code snippets" className="flex flex-wrap gap-1.5" data-testid="snippet-chips">
      {snippets.items.map((s) => {
        const lines = lineCount(s.text);
        const name = s.lang === "text" ? "Text snippet" : s.lang === "markdown" ? "Markdown (raw)" : `${snippetLangLabel(s.lang)} snippet`;
        return (
          <li key={s.key} data-testid="snippet-chip" data-lang={s.lang} className="flex items-center gap-2 rounded-lg border border-border bg-surface py-1.5 pl-1.5 pr-2">
            <button
              type="button"
              disabled={disabled}
              onClick={() => onEdit(s)}
              aria-label={`Edit ${name}`}
              className="flex min-w-0 items-center gap-2 text-left disabled:cursor-default"
            >
              <span aria-hidden className="grid size-7 flex-none place-items-center rounded-md bg-accent-soft font-mono text-11 font-semibold text-accent">
                {s.lang === "markdown" ? "M↓" : "{ }"}
              </span>
              <span className="flex min-w-0 flex-col">
                <span className="truncate text-13 font-medium text-fg">{name}</span>
                <span className="text-11 text-fg-tertiary">
                  {lines} {lines === 1 ? "line" : "lines"} · click to edit
                </span>
              </span>
            </button>
            <button
              type="button"
              aria-label={`Remove ${name}`}
              disabled={disabled}
              onClick={() => snippets.remove(s.key)}
              className="grid size-4 flex-none place-items-center self-start rounded-xs text-fg-tertiary hover:bg-hover hover:text-fg"
            >
              <Icon name="x" size={11} />
            </button>
          </li>
        );
      })}
    </ul>
  );
}

const editorText = "font-mono text-12 leading-relaxed p-3 whitespace-pre";

/**
 * A textarea over its own highlighted copy: the textarea's text is transparent (the caret and
 * selection stay), so the person types in a plain field and sees the colours underneath.
 * Both scroll together and never wrap, so their lines stay aligned.
 */
function HighlightedEditor({ value, lang, onChange }: { value: string; lang: string; onChange: (v: string) => void }) {
  const under = useRef<HTMLPreElement>(null);
  const lines = value.split("\n");
  const toks = useHighlighted([lines], langFromFence(lang), value.length > 0)?.[0];
  const follow = (e: UIEvent<HTMLTextAreaElement>) => {
    if (!under.current) return;
    under.current.scrollTop = e.currentTarget.scrollTop;
    under.current.scrollLeft = e.currentTarget.scrollLeft;
  };
  return (
    <div className="relative h-56 overflow-hidden rounded-lg border border-border bg-surface-sunken focus-within:border-accent">
      <pre ref={under} aria-hidden className={cx(editorText, "pointer-events-none absolute inset-0 m-0 overflow-hidden text-fg")}>
        {lines.map((l, i) => (
          <span key={i} className="block">
            {toks?.[i]
              ? toks[i].map((t, j) => (
                  <span key={j} className="shiki-tok" style={t.style}>
                    {t.content}
                  </span>
                ))
              : l}
            {l === "" && " "}
          </span>
        ))}
      </pre>
      <textarea
        autoFocus
        aria-label="Snippet"
        data-testid="snippet-input"
        spellCheck={false}
        wrap="off"
        value={value}
        placeholder="Paste code, a log or Markdown"
        onChange={(e) => onChange(e.target.value)}
        onScroll={follow}
        className={cx(
          editorText,
          "absolute inset-0 size-full resize-none overflow-auto border-0 bg-transparent text-transparent caret-fg outline-none placeholder:text-fg-tertiary selection:bg-accent/30",
        )}
      />
    </div>
  );
}

/**
 * The bubble over the composer (anchored to `anchor`, the composer's frame) where a snippet
 * is pasted or edited. The language is guessed as the text changes until the person picks one.
 * A paste too long for a message is offered as a file instead.
 */
export function SnippetBubble({
  anchor,
  draft,
  onDraft,
  onSave,
  onAttach,
  onClosed,
}: {
  anchor: RefObject<HTMLDivElement | null>;
  draft: SnippetDraft | null;
  onDraft: (d: SnippetDraft | null) => void;
  onSave: (d: SnippetDraft) => void;
  onAttach: (d: SnippetDraft) => void;
  /** After it closes, by any route: focus goes back to the message box rather than the pill. */
  onClosed?: () => void;
}) {
  const big = !!draft && isBigPaste(draft.text);
  const lines = draft ? lineCount(draft.text) : 0;
  const empty = !draft?.text.trim();
  return (
    <Popover.Root open={!!draft} onOpenChange={(open) => !open && onDraft(null)}>
      <Popover.Anchor virtualRef={anchor} />
      <Popover.Portal>
        <Popover.Content
          side="top"
          align="start"
          sideOffset={8}
          collisionPadding={12}
          data-testid="snippet-bubble"
          aria-label={draft?.key === undefined ? "Add code or Markdown" : "Edit snippet"}
          // Only Cancel, ✕, Escape or a click outside close it: focus moving away (the
          // compact options sheet handing focus back) shouldn't throw the paste away.
          onFocusOutside={(e) => e.preventDefault()}
          // The Code pill toggles the bubble itself; closing here too would reopen it.
          onPointerDownOutside={(e) => {
            if (e.target instanceof Element && e.target.closest("[data-snippet-trigger]")) e.preventDefault();
          }}
          onOpenAutoFocus={(e) => e.preventDefault()}
          onCloseAutoFocus={(e) => {
            e.preventDefault();
            onClosed?.();
          }}
          className="motion-float z-50 flex w-[min(560px,calc(100vw-24px))] flex-col gap-3 rounded-xl border border-border-float bg-surface-raised p-4 shadow-[var(--shadow-float)] outline-none"
          onKeyDown={(e) => {
            if (draft && !empty && e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
              e.preventDefault();
              (big ? onAttach : onSave)(draft);
            }
          }}
        >
          {draft && (
            <>
              <div className="flex items-center gap-2">
                <h2 className="flex-1 text-13 font-semibold text-fg">{draft.key === undefined ? "Paste code or Markdown" : "Edit snippet"}</h2>
                <label className="flex items-center gap-1.5 text-12 text-fg-tertiary">
                  <span className="sr-only">Language</span>
                  <select
                    data-testid="snippet-lang"
                    value={draft.lang}
                    onChange={(e) => onDraft({ ...draft, lang: e.target.value, picked: true })}
                    className="h-7 rounded-md border border-border bg-selected px-2 text-12 font-medium text-fg outline-none focus-visible:border-accent"
                  >
                    {SNIPPET_LANGS.map((l) => (
                      <option key={l.tag} value={l.tag}>
                        {l.label}
                      </option>
                    ))}
                  </select>
                  {!draft.picked && !empty && <span data-testid="snippet-detected">detected</span>}
                </label>
                <Popover.Close aria-label="Close" className="grid size-6 place-items-center rounded-md text-fg-tertiary hover:bg-hover hover:text-fg">
                  <Icon name="x" size={13} />
                </Popover.Close>
              </div>
              <HighlightedEditor
                value={draft.text}
                lang={draft.lang}
                onChange={(text) => onDraft({ ...draft, text, lang: draft.picked ? draft.lang : detectLanguage(text) })}
              />
              {big && (
                <p role="status" data-testid="snippet-big" className="rounded-lg border border-pill-human-fg/40 bg-pill-human-bg px-3 py-2 text-12 text-pill-human-fg">
                  {lines.toLocaleString()} {lines === 1 ? "line" : "lines"} ({Math.ceil(draft.text.length / 1024).toLocaleString()} KB). That’s long for a message: as a file it
                  won’t flood the chat, and the agent can still search it.
                </p>
              )}
              <div className="flex items-center gap-2">
                <span className="flex-1 text-12 text-fg-tertiary">
                  {empty ? "Sent as a fenced block after your message" : `${lines} ${lines === 1 ? "line" : "lines"} · ⌘Enter to ${big ? "attach" : "add"}`}
                </span>
                <button
                  type="button"
                  onClick={() => (big ? onSave(draft) : onDraft(null))}
                  className="h-8 rounded-md bg-selected px-3 text-13 text-fg-secondary hover:bg-hover hover:text-fg"
                >
                  {big ? "Add anyway" : "Cancel"}
                </button>
                <button
                  type="button"
                  data-testid="snippet-save"
                  disabled={empty}
                  onClick={() => (big ? onAttach : onSave)(draft)}
                  className="h-8 rounded-md bg-accent px-3 text-13 font-semibold text-on-accent hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50"
                >
                  {big ? "Attach as file" : draft.key === undefined ? "Add to message" : "Save"}
                </button>
              </div>
            </>
          )}
        </Popover.Content>
      </Popover.Portal>
    </Popover.Root>
  );
}

/** A paste into the message box that could be a snippet or a file: where it sits, so the offer can take it back out. */
export type PasteOfferState = { text: string; start: number; lang: string; big: boolean };

/**
 * Offered after pasting code-like or very long text into the message box. The paste has
 * already happened; ignoring this leaves it exactly as pasted.
 */
export function PasteOffer({ offer, onSnippet, onAttach, onDismiss }: { offer: PasteOfferState; onSnippet: () => void; onAttach: () => void; onDismiss: () => void }) {
  const lines = lineCount(offer.text);
  return (
    <div role="status" data-testid="paste-offer" className="flex flex-wrap items-center gap-2 rounded-xl border border-accent bg-surface-raised px-3 py-2">
      <p className="min-w-0 flex-1 text-13 text-fg">
        {offer.big ? (
          <>
            {lines.toLocaleString()} {lines === 1 ? "line" : "lines"} pasted. That’s long for a message: attach it as a file?
          </>
        ) : (
          <>
            Looks like <span className="font-semibold text-accent">{snippetLangLabel(offer.lang)}</span>. Send it as a code block?
          </>
        )}
      </p>
      {offer.big ? (
        <button type="button" data-testid="paste-offer-attach" onClick={onAttach} className="h-7 rounded-md bg-accent px-3 text-13 font-semibold text-on-accent hover:opacity-90">
          Attach as file
        </button>
      ) : (
        <button type="button" data-testid="paste-offer-snippet" onClick={onSnippet} className="h-7 rounded-md bg-accent px-3 text-13 font-semibold text-on-accent hover:opacity-90">
          Code block
        </button>
      )}
      <button type="button" onClick={onDismiss} className="h-7 rounded-md bg-selected px-3 text-13 text-fg-secondary hover:bg-hover hover:text-fg">
        Keep as text
      </button>
    </div>
  );
}

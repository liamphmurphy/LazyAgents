"use client";

import { useEffect, useMemo, useState } from "react";
import { cx } from "@/lib/cx";
import { SlideSvg } from "@/lib/slides/render";
import { deckChanges, resolveDeck, reviewNote, type Decision } from "@/lib/slides/review";
import type { Deck } from "@/lib/slides/schema";
import { Button } from "@/components/ui/button";

/**
 * A persona's changes, one decision per changed slide, in the editor dock's
 * Review tab (mockup "Slides editor on the shared EditorLayout"). Nothing
 * is written until every change is decided; then one version records the
 * outcome, or, if everything was accepted, the review is just marked done.
 */
export function DeckReview({ base, head, persona, assetUrl, fontsReady, busy, error, onAcceptAll, onResolve }: {
  base: Deck;
  head: Deck;
  persona: string;
  assetUrl: (id: string) => string;
  fontsReady: boolean;
  busy: boolean;
  error?: string;
  onAcceptAll: () => void;
  onResolve: (deck: Deck, note: string) => void;
}) {
  const changes = useMemo(() => deckChanges(base, head), [base, head]);
  const [decisions, setDecisions] = useState<Record<number, Decision>>({});
  const left = changes.filter((c) => !decisions[c.id]).length;

  const finish = (all: Record<number, Decision>) => {
    if (changes.every((c) => all[c.id] === "accept")) onAcceptAll();
    else onResolve(resolveDeck(base, head, changes, all), reviewNote(changes, all, persona));
  };
  const decide = (id: number, d: Decision) => {
    const next = { ...decisions, [id]: d };
    setDecisions(next);
    if (changes.every((c) => next[c.id])) finish(next);
  };
  const everything = (d: Decision) => {
    const all = Object.fromEntries(changes.map((c) => [c.id, d]));
    setDecisions(all);
    finish(all);
  };

  // J/K move between changes, A accepts and R rejects the focused one (mockup note 3).
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (busy || e.target instanceof HTMLInputElement || e.target instanceof HTMLTextAreaElement || (e.target as HTMLElement).isContentEditable) return;
      const cards = [...document.querySelectorAll<HTMLElement>("[data-change]")];
      const i = cards.findIndex((c) => c.contains(document.activeElement));
      if (e.key === "j" || e.key === "k") {
        e.preventDefault();
        cards[Math.max(0, Math.min(cards.length - 1, i + (e.key === "j" ? 1 : -1)))]?.focus();
      } else if ((e.key === "a" || e.key === "r") && i >= 0 && !e.metaKey && !e.ctrlKey) {
        e.preventDefault();
        if (e.shiftKey && e.key === "a") everything("accept");
        else decide(Number(cards[i].dataset.change), e.key === "a" ? "accept" : "reject");
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  });

  const thumb = (deck: Deck, slide: NonNullable<(typeof changes)[number]["before"]>, marks: Record<string, "added" | "removed" | "changed">) => (
    <span className="block overflow-hidden rounded-[4px] border border-border">
      <SlideSvg deck={deck} slide={slide} assetUrl={assetUrl} fontsReady={fontsReady} marks={marks} className="block aspect-video w-full" />
    </span>
  );
  const none = (text: string) => <span className="grid aspect-video w-full place-items-center rounded-[4px] border border-dashed border-border-strong bg-surface-sunken text-center text-[11px] text-fg-tertiary">{text}</span>;

  return (
    <div className="flex min-h-0 flex-1 flex-col" data-testid="deck-review">
      <div role="status" className="flex flex-none flex-col gap-2 border-b border-border-subtle px-3 py-2.5 text-[13px]">
        <div className="flex items-center gap-2">
          <span className="min-w-0 flex-1 font-semibold text-fg">
            {persona} suggested {changes.length === 1 ? "1 change" : `${changes.length} changes`}
          </span>
          <Button variant="secondary" disabled={busy} onClick={() => everything("reject")}>Reject all</Button>
          <Button variant="primary" disabled={busy} onClick={() => everything("accept")}>Accept all</Button>
        </div>
        <span className="text-[12px] text-fg-tertiary">{left === changes.length ? "Accept or reject each one, or all at once. Editing is paused until you finish." : `${left} left to decide.`}</span>
      </div>
      {error && <p role="alert" className="px-3 pt-2 text-xs text-pill-blocked-fg">{error}</p>}
      <ol className="flex min-h-0 flex-col gap-3 overflow-y-auto p-3">
        {changes.map((c) => {
          const d = decisions[c.id];
          const label = { added: "Added", removed: "Removed", moved: "Moved", modified: "Modified", theme: "Theme" }[c.kind];
          return (
            <li key={c.id} data-change={c.id} data-change-slide={(c.after ?? c.before)?.id} tabIndex={-1} className={cx("rounded-xl border bg-surface p-3 outline-none focus-visible:ring-2 focus-visible:ring-accent/50", d ? "border-border-subtle" : "border-border")}>
              <div className="flex flex-wrap items-center gap-2">
                <span className={cx("rounded-[5px] px-1.5 py-px text-[11.5px] font-medium", d ? "bg-pill-neutral-bg text-pill-neutral-fg" : "bg-pill-human-bg text-pill-human-fg")}>{label}</span>
                <span className={cx("text-[13.5px] font-semibold", d ? "text-fg-secondary" : "text-fg")}>{c.kind === "theme" ? "Deck theme" : `Slide ${c.position} · ${c.title}`}</span>
                {d && <span className={cx("rounded-[5px] px-1.5 py-px text-[11.5px] font-medium", d === "accept" ? "bg-pill-done-bg text-pill-done-fg" : "bg-pill-blocked-bg text-pill-blocked-fg")}>{d === "accept" ? "✓ Accepted" : "Rejected"}</span>}
                <span className="flex-1" />
                {d && (
                  <button type="button" className="text-[12.5px] text-accent hover:underline" disabled={busy} onClick={() => setDecisions((cur) => Object.fromEntries(Object.entries(cur).filter(([k]) => Number(k) !== c.id)))}>
                    Undo
                  </button>
                )}
              </div>
              {d ? (
                <p className="mt-1.5 text-[12.5px] text-fg-tertiary">{c.details.join(" · ")}</p>
              ) : (
                <div className="mt-2.5 flex flex-col gap-2.5">
                  {c.kind !== "theme" && (
                    <div className="grid grid-cols-[1fr_auto_1fr] items-center gap-2">
                      <div>
                        <p className="mb-1 text-[11px] font-medium text-fg-tertiary">Before</p>
                        {c.before ? thumb(base, c.before, Object.fromEntries(Object.keys(c.marksBefore).map((k) => [k, "removed" as const]))) : none("No slide here before")}
                      </div>
                      <span aria-hidden className="pt-4 text-fg-tertiary">→</span>
                      <div>
                        <p className="mb-1 text-[11px] font-medium text-fg-tertiary">After</p>
                        {c.after ? thumb(head, c.after, c.kind === "added" ? Object.fromEntries(c.after.elements.map((e) => [e.id, "added" as const])) : c.marksAfter) : none("Slide removed")}
                      </div>
                    </div>
                  )}
                  <ul className="flex flex-col gap-1 text-[12.5px] text-fg-secondary" aria-label="What changed">
                    {c.details.map((x, i) => <li key={i}>{x}</li>)}
                  </ul>
                  <div className="flex justify-end gap-2">
                    <Button variant="approve-outline" disabled={busy} onClick={() => decide(c.id, "reject")}>Reject</Button>
                    <Button variant="approve" disabled={busy} onClick={() => decide(c.id, "accept")}>Accept</Button>
                  </div>
                </div>
              )}
            </li>
          );
        })}
      </ol>
    </div>
  );
}

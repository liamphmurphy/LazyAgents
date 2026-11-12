"use client";

import { useMemo, useState } from "react";
import { reviewSegments, resolveReview, reviewNote, type Decision, type Segment } from "@/lib/doc-review";
import { cx } from "@/lib/cx";
import { Markdown } from "@/components/domain/markdown";
import { Button } from "@/components/ui/button";

type Change = Extract<Segment, { kind: "change" }>;

/**
 * A persona's changes since the person's last edit, as suggestions: each run
 * of changed blocks is accepted or rejected, in place on the page or from the
 * Review tab's list, which share these decisions. Once every one is decided,
 * the outcome is recorded: accepting them all marks the review done;
 * rejecting any writes the document back without those as the person's edit.
 * The decisions start over whenever `base` or `head` changes.
 */
export function useDocReview({ base, head, persona, onAcceptAll, onResolve }: {
  base?: string;
  head?: string;
  persona: string;
  onAcceptAll: () => void;
  onResolve: (body: string, note: string) => void;
}) {
  const segments = useMemo(() => (base === undefined || head === undefined ? [] : reviewSegments(base, head)), [base, head]);
  const changes = useMemo(() => segments.filter((s): s is Change => s.kind === "change"), [segments]);
  const [state, setState] = useState<{ segments: Segment[]; decisions: Record<number, Decision> }>({ segments, decisions: {} });
  const decisions = state.segments === segments ? state.decisions : {};
  const set = (d: Record<number, Decision>) => setState({ segments, decisions: d });
  const left = changes.filter((s) => !decisions[s.id]).length;

  const finish = (all: Record<number, Decision>) => {
    if (changes.every((s) => all[s.id] === "accept")) onAcceptAll();
    else onResolve(resolveReview(segments, all), reviewNote(segments, all, persona));
  };
  const decide = (id: number, d: Decision) => {
    const next = { ...decisions, [id]: d };
    set(next);
    if (changes.every((s) => next[s.id])) finish(next);
  };
  const undo = (id: number) => set(Object.fromEntries(Object.entries(decisions).filter(([k]) => Number(k) !== id)));
  const everything = (d: Decision) => finish(Object.fromEntries(changes.map((s) => [s.id, d])));
  return { segments, changes, decisions, left, decide, undo, everything };
}

export type DocReviewState = ReturnType<typeof useDocReview>;

const changeAnchor = (id: number) => `doc-change-${id}`;

/** The suggestions in place on the page, each with its Accept and Reject. */
export function DocReview({ review, busy, error }: { review: DocReviewState; busy: boolean; error?: string }) {
  const { segments, changes, decisions, decide, undo } = review;
  return (
    <div className="flex min-h-0 flex-col" data-testid="doc-review">
      {error && <p role="alert" className="px-4 pt-2 text-xs text-pill-blocked-fg">{error}</p>}
      <div className="doc-page mx-auto w-full max-w-[720px] px-8 pb-24 pt-10 max-sm:px-5">
        {segments.map((s, i) =>
          s.kind === "same" ? (
            <Markdown key={i} className="type-reading text-fg-2">{s.blocks.join("\n\n")}</Markdown>
          ) : (
            <section key={i} id={changeAnchor(s.id)} aria-label={`Change ${s.id + 1} of ${changes.length}`} data-testid="doc-change" className={cx("relative my-3 scroll-mt-6 rounded-md transition-opacity", decisions[s.id] && "opacity-60")}>
              {s.before.length > 0 && decisions[s.id] !== "accept" && (
                <div className={cx("rounded-md px-2", decisions[s.id] !== "reject" && "doc-change-before")}>
                  <span className="sr-only">Removed: </span>
                  <Markdown className="type-reading">{s.before.join("\n\n")}</Markdown>
                </div>
              )}
              {s.after.length > 0 && decisions[s.id] !== "reject" && (
                <div className={cx("rounded-md px-2", decisions[s.id] !== "accept" && "doc-change-after")}>
                  <span className="sr-only">Added: </span>
                  <Markdown className="type-reading text-fg-2">{s.after.join("\n\n")}</Markdown>
                </div>
              )}
              <div className="mt-1 flex justify-end gap-1.5 text-[12px]">
                {decisions[s.id] ? (
                  <button type="button" className="text-fg-tertiary hover:text-fg" disabled={busy} onClick={() => undo(s.id)}>
                    {decisions[s.id] === "accept" ? "Accepted" : "Rejected"} · Undo
                  </button>
                ) : (
                  <>
                    <button type="button" disabled={busy} onClick={() => decide(s.id, "reject")} className="rounded-md border border-border-strong px-2 py-0.5 text-fg-2 hover:bg-hover">Reject</button>
                    <button type="button" disabled={busy} onClick={() => decide(s.id, "accept")} className="rounded-md border border-pill-done-fg bg-pill-done-fg px-2 py-0.5 font-semibold text-bg hover:opacity-90">Accept</button>
                  </>
                )}
              </div>
            </section>
          ),
        )}
      </div>
    </div>
  );
}

const plain = (md: string) => md.replace(/^#{1,6}\s+|^\s*(?:[-*+]|\d+\.)\s+(?:\[[ xX]\]\s+)?|^>\s?|[*_`~]|\[([^\]]*)\]\([^)]*\)/gm, "$1").replace(/\s+/g, " ").trim();
const clip = (s: string, n = 60) => (s.length > n ? `${s.slice(0, n)}…` : s);

/** The section a change sits in: the nearest heading above it, or the change's own first heading. */
function sectionOf(segments: Segment[], at: number): string | undefined {
  const own = segments[at] as Change;
  const first = [...own.after, ...own.before].find((b) => /^#{1,6}\s/.test(b));
  if (first) return plain(first);
  for (let i = at - 1; i >= 0; i--) {
    const s = segments[i];
    const blocks = s.kind === "same" ? s.blocks : s.after;
    const h = blocks.findLast((b) => /^#{1,6}\s/.test(b));
    if (h) return plain(h);
  }
  return undefined;
}

/** The Review tab: the suggestions as a list, all-at-once buttons, and a link from each to its place on the page. */
export function DocReviewList({ review, persona, busy, error }: { review: DocReviewState; persona: string; busy: boolean; error?: string }) {
  const { segments, changes, decisions, left, undo, everything } = review;
  return (
    <div data-testid="doc-review-list" className="flex min-h-0 flex-1 flex-col gap-3 overflow-y-auto p-4 text-[13px]">
      <div role="status" className="flex flex-col gap-0.5">
        <span className="font-semibold text-fg">{persona} suggested {changes.length === 1 ? "a change" : `${changes.length} changes`}</span>
        <span className="text-[12px] text-fg-tertiary">{left === changes.length ? "Accept or reject each one, or all at once." : `${left} left to decide.`}</span>
      </div>
      <div className="flex gap-2">
        <Button variant="secondary" disabled={busy} onClick={() => everything("reject")}>Reject all</Button>
        <Button variant="primary" disabled={busy} onClick={() => everything("accept")}>Accept all</Button>
      </div>
      {error && <p role="alert" className="text-xs text-pill-blocked-fg">{error}</p>}
      <ol aria-label="Suggestions" className="flex flex-col gap-2">
        {segments.map((s, i) => {
          if (s.kind !== "change") return null;
          const section = sectionOf(segments, i);
          const d = decisions[s.id];
          return (
            <li key={s.id} className={cx("rounded-lg bg-surface-raised", d && "opacity-60")}>
              <button
                type="button"
                className="flex w-full flex-col items-start gap-1 rounded-lg px-3 py-2 text-left hover:bg-hover"
                onClick={() => document.getElementById(changeAnchor(s.id))?.scrollIntoView({ behavior: "smooth", block: "start" })}
              >
                <span className="text-[12px] text-fg-tertiary">Change {s.id + 1}{section ? ` · ${section}` : ""}</span>
                <span className="text-fg">
                  {s.after.length ? `“${clip(plain(s.after.join(" ")))}”` : <>Removes “{clip(plain(s.before.join(" ")))}”</>}
                </span>
              </button>
              {d && (
                <button type="button" disabled={busy} onClick={() => undo(s.id)} className="px-3 pb-2 text-[12px] text-fg-tertiary hover:text-fg">
                  {d === "accept" ? "Accepted" : "Rejected"} · Undo
                </button>
              )}
            </li>
          );
        })}
      </ol>
    </div>
  );
}

"use client";

import { useMemo, useState } from "react";
import { reviewSegments, resolveReview, reviewNote, type Decision } from "@/lib/doc-review";
import { cx } from "@/lib/cx";
import { Markdown } from "@/components/domain/markdown";
import { Button } from "@/components/ui/button";

/**
 * A persona's changes since the person's last edit, as suggestions: each run
 * of changed blocks is accepted or rejected in place. Once every one is
 * decided, the outcome is recorded: accepting them all marks the review done;
 * rejecting any writes the document back without those as the person's edit.
 */
export function DocReview({ base, head, persona, busy, error, onAcceptAll, onResolve }: {
  base: string;
  head: string;
  persona: string;
  busy: boolean;
  error?: string;
  onAcceptAll: () => void;
  onResolve: (body: string, note: string) => void;
}) {
  const segments = useMemo(() => reviewSegments(base, head), [base, head]);
  const changes = segments.filter((s) => s.kind === "change");
  const [decisions, setDecisions] = useState<Record<number, Decision>>({});
  const left = changes.filter((s) => !decisions[s.id]).length;

  const finish = (all: Record<number, Decision>) => {
    if (changes.every((s) => all[s.id] === "accept")) onAcceptAll();
    else onResolve(resolveReview(segments, all), reviewNote(segments, all, persona));
  };
  const decide = (id: number, d: Decision) => {
    const next = { ...decisions, [id]: d };
    setDecisions(next);
    if (changes.every((s) => next[s.id])) finish(next);
  };
  const everything = (d: Decision) => finish(Object.fromEntries(changes.map((s) => [s.id, d])));

  return (
    <div className="flex min-h-0 flex-col" data-testid="doc-review">
      <div role="status" className="sticky top-0 z-10 flex flex-wrap items-center gap-2 border-b border-border-subtle bg-bg/95 px-4 py-2 text-[13px] backdrop-blur">
        <span className="font-semibold text-fg">
          {persona} suggested {changes.length === 1 ? "a change" : `${changes.length} changes`}
        </span>
        <span className="text-fg-tertiary">{left === changes.length ? "Accept or reject each one, or all at once." : `${left} left to decide.`}</span>
        <span className="flex-1" />
        <Button variant="secondary" disabled={busy} onClick={() => everything("reject")}>Reject all</Button>
        <Button variant="primary" disabled={busy} onClick={() => everything("accept")}>Accept all</Button>
      </div>
      {error && <p role="alert" className="px-4 pt-2 text-xs text-pill-blocked-fg">{error}</p>}
      <div className="doc-page mx-auto w-full max-w-[720px] px-8 pb-24 pt-10 max-sm:px-5">
        {segments.map((s, i) =>
          s.kind === "same" ? (
            <Markdown key={i} className="type-reading text-fg-2">{s.blocks.join("\n\n")}</Markdown>
          ) : (
            <section key={i} aria-label={`Change ${s.id + 1} of ${changes.length}`} data-testid="doc-change" className={cx("relative my-3 rounded-md transition-opacity", decisions[s.id] && "opacity-60")}>
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
                  <button type="button" className="text-fg-tertiary hover:text-fg" disabled={busy} onClick={() => setDecisions((all) => Object.fromEntries(Object.entries(all).filter(([k]) => Number(k) !== s.id)))}>
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

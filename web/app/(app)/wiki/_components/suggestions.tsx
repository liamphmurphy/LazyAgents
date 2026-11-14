"use client";

import Link from "next/link";
import { useState } from "react";
import type { WikiSuggestion } from "@/lib/api/client";
import { useDecideSuggestion } from "@/lib/api/hooks";
import { cx } from "@/lib/cx";
import { diffLines, type DiffLine } from "@/lib/diff";
import { timelineTime } from "@/lib/format";
import { wikiHref } from "@/lib/wikilinks";
import { Button } from "@/components/ui/button";
import { ApiActorAvatar } from "@/components/domain/avatars";

type Shown = DiffLine | { op: "skip"; text: string };

/** Keeps 2 lines of context around changes; longer unchanged runs fold. */
function collapse(lines: DiffLine[], ctx = 2): Shown[] {
  const near = lines.map((_, i) => lines.slice(Math.max(0, i - ctx), i + ctx + 1).some((l) => l.op !== "same"));
  const out: Shown[] = [];
  let run = 0;
  lines.forEach((l, i) => {
    if (l.op === "same" && !near[i]) {
      run++;
      return;
    }
    if (run) out.push({ op: "skip", text: String(run) });
    run = 0;
    out.push(l);
  });
  if (run) out.push({ op: "skip", text: String(run) });
  return out;
}

/** Line diff of a suggestion against the page as it is now. */
export function SuggestionDiff({ s }: { s: WikiSuggestion }) {
  const lines = collapse(diffLines(s.current_body_md, s.body_md));
  return (
    <pre className="max-h-[50vh] overflow-auto rounded-lg border border-border bg-surface-sunken py-2 font-mono text-12 leading-relaxed">
      {lines.map((l, i) =>
        l.op === "skip" ? (
          <div key={i} className="select-none px-3 text-fg-tertiary">
            ⋯ {l.text} unchanged lines
          </div>
        ) : (
        <div
          key={i}
          className={cx(
            "whitespace-pre-wrap px-3",
            l.op === "add" && "bg-pill-done-bg text-pill-done-fg",
            l.op === "del" && "bg-pill-blocked-bg text-pill-blocked-fg line-through decoration-1",
            l.op === "same" && "text-fg-secondary",
          )}
        >
          <span aria-hidden className="mr-2 inline-block w-3 select-none opacity-70">
            {l.op === "add" ? "+" : l.op === "del" ? "−" : " "}
          </span>
          <span className="sr-only">{l.op === "add" ? "Added: " : l.op === "del" ? "Removed: " : ""}</span>
          {l.text || " "}
        </div>
        ),
      )}
    </pre>
  );
}

/**
 * One agent suggestion with Accept / Reject (docs/dev/plan.md §12.5). Amber: a
 * person needs to act.
 */
export function SuggestionCard({ s, showPage }: { s: WikiSuggestion; showPage?: boolean }) {
  const decide = useDecideSuggestion();
  const [open, setOpen] = useState(!showPage);
  return (
    <div
      className="flex flex-col gap-3 rounded-xl border px-4 py-3"
      style={{
        borderColor: "color-mix(in srgb, var(--state-human) 25%, transparent)",
        background: "color-mix(in srgb, var(--state-human) 5%, transparent)",
      }}
    >
      <div className="flex flex-wrap items-center gap-3">
        <ApiActorAvatar actor={s.author} />
        <div className="flex min-w-[200px] flex-1 flex-col gap-0.5">
          <span className="font-semibold text-pill-human-fg">
            {s.author.name} {s.new_page ? "suggested a new page" : "suggested an edit"}
            {showPage && (
              <>
                {" "}
                to{" "}
                {s.new_page ? (
                  <span className="font-mono text-13">{s.path}</span>
                ) : (
                  <Link href={wikiHref(s.path)} className="font-mono text-13 underline-offset-2 hover:underline">
                    {s.path}
                  </Link>
                )}
              </>
            )}
          </span>
          <span className="text-13 text-fg-secondary">
            {s.note || "No note"} · {timelineTime(s.created_at)}
            {s.stale && " · the page changed since; accepting replaces those changes"}
          </span>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          {showPage && (
            <Button variant="ghost" onClick={() => setOpen(!open)} aria-expanded={open}>
              {open ? "Hide changes" : "View changes"}
            </Button>
          )}
          <Button variant="approve-outline" disabled={decide.isPending} onClick={() => decide.mutate({ id: s.id, decision: "reject" })}>
            Reject
          </Button>
          <Button variant="approve" disabled={decide.isPending} onClick={() => decide.mutate({ id: s.id, decision: "accept" })}>
            Accept
          </Button>
        </div>
      </div>
      {open && <SuggestionDiff s={s} />}
      {decide.isError && <p className="text-12 text-pill-blocked-fg">{decide.error.message}</p>}
    </div>
  );
}

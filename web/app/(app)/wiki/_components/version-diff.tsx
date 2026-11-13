"use client";

import { useRouter } from "next/navigation";
import { Fragment, useEffect, useMemo, useState } from "react";
import type { Artifact } from "@/lib/api/client";
import { useVersionText } from "@/lib/api/hooks";
import { cx } from "@/lib/cx";
import { diffLines } from "@/lib/diff";
import { timelineTime } from "@/lib/format";
import { diffStats, foldDiff, numberLines, type NumberedLine } from "@/lib/wiki";
import { wikiHref } from "@/lib/wikilinks";
import { Button } from "@/components/ui/button";
import { Banner, Skeleton } from "@/components/ui/primitives";
import { compareHref } from "./page-rail";

function Line({ l }: { l: NumberedLine }) {
  return (
    <div
      className={cx(
        "grid grid-cols-[2.5rem_2.5rem_1fr] whitespace-pre-wrap",
        l.op === "add" && "bg-pill-done-bg text-pill-done-fg",
        l.op === "del" && "bg-pill-blocked-bg text-pill-blocked-fg",
        l.op === "same" && "text-fg-secondary",
      )}
    >
      <span aria-hidden className="select-none pr-2 text-right text-fg-tertiary">
        {l.a ?? ""}
      </span>
      <span aria-hidden className="select-none pr-2 text-right text-fg-tertiary">
        {l.b ?? ""}
      </span>
      <span className="pr-3">
        <span aria-hidden className="mr-2 inline-block w-3 select-none opacity-70">
          {l.op === "add" ? "+" : l.op === "del" ? "−" : " "}
        </span>
        <span className="sr-only">{l.op === "add" ? "Added: " : l.op === "del" ? "Removed: " : ""}</span>
        {l.text || " "}
      </span>
    </div>
  );
}

/**
 * Line diff between two versions of a page, in the article column
 * (?compare=v2..v3). Unchanged runs fold with 3 lines of context; Esc or
 * "Back to page" returns to the rendered page.
 */
export function VersionDiff({ page, from, to }: { page: Artifact; from: string; to: string }) {
  const router = useRouter();
  const path = page.path ?? "";
  const vs = page.versions;
  const a = vs.find((v) => v.label === from);
  const b = vs.find((v) => v.label === to);
  const left = useVersionText(page.id, a?.id);
  const right = useVersionText(page.id, b?.id);
  const [unfolded, setUnfolded] = useState<Set<number>>(() => new Set());
  const lines = useMemo(() => (left.data !== undefined && right.data !== undefined ? numberLines(diffLines(left.data, right.data)) : undefined), [left.data, right.data]);
  const folded = useMemo(() => (lines ? foldDiff(lines) : []), [lines]);
  const stats = lines ? diffStats(lines) : undefined;
  const back = () => router.push(wikiHref(path));

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape" && !(e.target as HTMLElement).closest("input, textarea, select, [contenteditable]")) router.push(wikiHref(path));
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [router, path]);

  const pick = (label: string, side: "from" | "to") => {
    router.replace(side === "from" ? compareHref(path, label, to) : compareHref(path, from, label));
    setUnfolded(new Set());
  };
  const select = (value: string, side: "from" | "to") => (
    <select
      aria-label={side === "from" ? "Older version" : "Newer version"}
      value={value}
      onChange={(e) => pick(e.target.value, side)}
      className="h-8 rounded-md border border-border bg-surface px-2 text-13 text-fg-2"
    >
      {!vs.some((v) => v.label === value) && <option value={value}>{value} (not found)</option>}
      {vs.map((v, i) => (
        <option key={v.id} value={v.label}>
          {v.label} · {v.created_by.name} · {i === 0 ? "current" : timelineTime(v.created_at)}
        </option>
      ))}
    </select>
  );

  return (
    <section aria-label="Compare versions" className="flex flex-col gap-3">
      <div className="flex flex-wrap items-center gap-2 text-13 text-fg-secondary">
        <span>Compare</span>
        {select(from, "from")}
        <span aria-hidden className="text-fg-tertiary">
          →
        </span>
        {select(to, "to")}
        {stats && (
          <span className="font-mono">
            <span className="text-pill-done-fg">+{stats.added}</span> <span className="text-pill-blocked-fg">−{stats.removed}</span>
          </span>
        )}
        <Button variant="outline" className="ml-auto" onClick={back}>
          ← Back to page
        </Button>
      </div>
      {!a || !b ? (
        <Banner tone="blocked" title="Can't compare these versions">
          {!a ? from : to} isn&apos;t a version of this page. Pick another one above.
        </Banner>
      ) : left.isError || right.isError ? (
        <Banner tone="blocked" title="Couldn't load a version">
          {(left.error ?? right.error)?.message}
        </Banner>
      ) : !lines ? (
        <div aria-busy="true" className="flex flex-col gap-2">
          <Skeleton className="h-4 w-full" />
          <Skeleton className="h-4 w-5/6" />
          <Skeleton className="h-4 w-2/3" />
        </div>
      ) : stats && stats.added + stats.removed === 0 ? (
        <p className="rounded-lg border border-border bg-surface px-3.5 py-3 text-12 text-fg-tertiary">
          {from} and {to} have the same content.
        </p>
      ) : (
        <div className="overflow-auto rounded-lg border border-border bg-surface-sunken py-1 font-mono text-12 leading-relaxed">
          {folded.map((l, i) =>
            l.op === "skip" ? (
              unfolded.has(l.from) ? (
                <Fragment key={i}>
                  {l.lines.map((x, j) => (
                    <Line key={j} l={x} />
                  ))}
                </Fragment>
              ) : (
                <button
                  key={i}
                  type="button"
                  onClick={() => setUnfolded(new Set(unfolded).add(l.from))}
                  className="block w-full py-0.5 text-center font-sans text-fg-tertiary hover:bg-hover hover:text-fg-secondary"
                >
                  ⋯ {l.count} unchanged {l.count === 1 ? "line" : "lines"} (show)
                </button>
              )
            ) : (
              <Line key={i} l={l} />
            ),
          )}
        </div>
      )}
    </section>
  );
}

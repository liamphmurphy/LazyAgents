"use client";

import Link from "next/link";
import { Fragment, useState } from "react";
import type { WikiADR } from "@/lib/api/client";
import { useWikiADRs } from "@/lib/api/hooks";
import { cx } from "@/lib/cx";
import { defaultLabel, parseWikiLinks } from "@/lib/wikilinks";
import { Banner, Skeleton } from "@/components/ui/primitives";
import { SmallPill } from "@/components/domain/labels";

type Filter = "all" | "open" | "accepted";

/** Markdown inline text with [[links]] shown as their labels. */
function Plain({ text }: { text: string }) {
  const parts: Array<string | { label: string }> = [];
  let rest = text;
  for (const r of parseWikiLinks(text)) {
    const i = rest.indexOf(r.raw);
    if (i < 0) continue;
    parts.push(rest.slice(0, i), { label: defaultLabel(r) });
    rest = rest.slice(i + r.raw.length);
  }
  parts.push(rest);
  return (
    <>
      {parts.map((p, i) =>
        typeof p === "string" ? <Fragment key={i}>{p.replace(/\*\*|__|`/g, "")}</Fragment> : <span key={i} className="text-fg">{p.label}</span>,
      )}
    </>
  );
}

function StatusPill({ a }: { a: WikiADR }) {
  if (a.has_open_questions) return <SmallPill tone="human">{a.status || "No status"} · open questions</SmallPill>;
  if (a.state === "accepted") return <SmallPill tone="done">{a.status}</SmallPill>;
  return <SmallPill tone="neutral">{a.status || "No status"}</SmallPill>;
}

function ADRRow({ a }: { a: WikiADR }) {
  const [all, setAll] = useState(false);
  const qs = a.open_questions;
  const shown = all ? qs : qs.slice(0, 2);
  const open = a.has_open_questions;
  return (
    <li className="relative flex flex-col gap-1.5 border-b border-border-subtle px-4 py-3 last:border-0">
      {open && <span aria-hidden className="absolute top-0 bottom-0 left-0 w-[3px] bg-pill-human-fg" />}
      <div className="flex flex-wrap items-center gap-2">
        <Link href={a.href} className="min-w-0 flex-1 truncate text-14 font-medium hover:underline">
          {a.display_id && <span className="mr-2 font-mono text-12 text-fg-tertiary">{a.display_id}</span>}
          {a.title}
        </Link>
        <StatusPill a={a} />
      </div>
      {(a.date || a.ticket || open) && (
        <span className="text-12 text-fg-tertiary">
          {[
            a.date,
            a.ticket && (
              <Link key="t" href={`/tickets/${a.ticket}`} className="font-mono text-accent hover:underline">
                {a.ticket}
              </Link>
            ),
            qs.length > 0 && `${qs.length} open ${qs.length === 1 ? "question" : "questions"}`,
          ]
            .filter(Boolean)
            .map((x, i) => (
              <Fragment key={i}>
                {i > 0 && " · "}
                {x}
              </Fragment>
            ))}
        </span>
      )}
      {qs.length > 0 && (
        <ol aria-label={`Open questions in ${a.title}`} className="flex flex-col gap-1 text-13 text-fg-2">
          {shown.map((q, i) => (
            <li key={i} className="flex gap-1.5">
              <span className="flex-none text-fg-tertiary">{i + 1}.</span>
              <span>
                <Plain text={q} />
              </span>
            </li>
          ))}
          {qs.length > 2 && (
            <li>
              <button type="button" className="text-12 text-accent hover:underline" onClick={() => setAll(!all)}>
                {all ? "Show fewer" : `+ ${qs.length - 2} more`}
              </button>
            </li>
          )}
        </ol>
      )}
    </li>
  );
}

/**
 * The ADR index (/wiki?index=adrs): each ADR's status from its "Status:"
 * line, and the ADRs with open questions first, with their questions.
 */
export function ADRIndex() {
  const { data, isError, error } = useWikiADRs();
  const [filter, setFilter] = useState<Filter>("all");
  const adrs = data ?? [];
  const open = adrs.filter((a) => a.has_open_questions);
  const accepted = adrs.filter((a) => a.state === "accepted" && !a.has_open_questions);
  const list = filter === "open" ? open : filter === "accepted" ? accepted : adrs;
  const tabs: Array<[Filter, string, number]> = [
    ["all", "All", adrs.length],
    ["open", "Open questions", open.length],
    ["accepted", "Accepted", accepted.length],
  ];
  return (
    <section aria-labelledby="adr-index" className="flex flex-col gap-3">
      <div className="flex flex-col gap-1">
        <h1 id="adr-index" className="type-display">
          Architecture decisions
        </h1>
        {data && (
          <p className="text-13 text-fg-secondary">
            {adrs.length} {adrs.length === 1 ? "ADR" : "ADRs"}
            {open.length > 0 && (
              <>
                {" "}
                · <span className="text-pill-human-fg">{open.length} with open questions</span>
              </>
            )}
          </p>
        )}
      </div>
      <div role="group" aria-label="Filter ADRs" className="flex w-fit gap-0.5 rounded-md border border-border bg-surface p-0.5">
        {tabs.map(([k, label, n]) => (
          <button
            key={k}
            type="button"
            aria-pressed={filter === k}
            onClick={() => setFilter(k)}
            className={cx("h-6 rounded-[5px] px-2.5 text-12", filter === k ? "bg-selected text-fg" : "text-fg-secondary hover:bg-hover")}
          >
            {label} <span className={cx("font-mono", k === "open" && n > 0 ? "text-pill-human-fg" : "text-fg-tertiary")}>{n}</span>
          </button>
        ))}
      </div>
      {isError ? (
        <Banner tone="blocked" title="Couldn't load ADRs">
          {error.message}
        </Banner>
      ) : !data ? (
        <div aria-busy="true" className="flex flex-col gap-2">
          <Skeleton className="h-16 w-full" />
          <Skeleton className="h-16 w-full" />
        </div>
      ) : list.length === 0 ? (
        <p className="rounded-xl border border-border bg-surface px-3.5 py-3 text-12 text-fg-tertiary">
          {adrs.length === 0 ? (
            <>
              No ADRs yet. Pages titled <code className="inline">ADR - …</code> (usually under Architecture/) and ADR documents show up here.
            </>
          ) : filter === "open" ? (
            "No ADR has open questions."
          ) : (
            "No accepted ADRs without open questions."
          )}
        </p>
      ) : (
        <ul aria-label="ADRs" className="flex flex-col overflow-hidden rounded-xl border border-border bg-surface">
          {list.map((a) => (
            <ADRRow key={a.id} a={a} />
          ))}
        </ul>
      )}
      <p className="text-12 text-fg-tertiary">
        Status, date and ticket come from each ADR&apos;s <code className="inline">Status: …</code> line; questions from its Open questions section.
      </p>
    </section>
  );
}

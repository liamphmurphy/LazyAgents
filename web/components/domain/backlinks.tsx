"use client";

import Link from "next/link";
import { Fragment } from "react";
import type { Mention, NodeKind } from "@/lib/api/client";
import { useBacklinks, useLinkMention } from "@/lib/api/hooks";
import { defaultLabel, parseWikiLinks } from "@/lib/wikilinks";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/primitives";
import { TargetMark } from "./wikilink";

/** A snippet with its [[links]] shown as their labels, emphasised. */
function Snippet({ text }: { text: string }) {
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
    <p className="line-clamp-3 text-xs leading-relaxed text-fg-secondary">
      {parts.map((p, i) =>
        typeof p === "string" ? <Fragment key={i}>{p}</Fragment> : <span key={i} className="font-medium text-fg-2">{p.label}</span>,
      )}
    </p>
  );
}

function SourceLine({ m }: { m: Mention }) {
  const s = m.source;
  const label = s.kind === "artifact" && s.artifact_type === "note" ? (s.path ?? s.title) : s.title;
  return (
    <Link href={s.href ?? "#"} className="flex min-w-0 items-center gap-2 text-[13px] hover:underline">
      <TargetMark t={s} size={12} />
      {s.display_id && s.kind !== "agent" && <span className="flex-none font-mono text-xs text-fg-tertiary">{s.display_id}</span>}
      <span className="truncate font-medium">{s.kind === "comment" ? "Comment" : label}</span>
    </Link>
  );
}

/**
 * Backlinks (PLAN.md §12.2): linked mentions with their snippets, then
 * unlinked mentions with a one-click "Link it".
 */
export function BacklinksPanel({ kind, id, compact }: { kind: NodeKind; id: string | undefined; compact?: boolean }) {
  const { data, isError } = useBacklinks(kind, id);
  const link = useLinkMention();
  if (isError) return <p className="text-xs text-fg-tertiary">Couldn&apos;t load backlinks.</p>;
  if (!data) {
    return (
      <div aria-busy="true" className="flex flex-col gap-2">
        <Skeleton className="h-3 w-2/3" />
        <Skeleton className="h-3 w-1/2" />
      </div>
    );
  }
  return (
    <div className="flex flex-col gap-4">
      <section className="flex flex-col gap-2">
        <h3 className="text-[12.5px] text-fg-tertiary">
          Linked mentions <span className="font-mono">{data.linked.length || ""}</span>
        </h3>
        {data.linked.length ? (
          <ul className="flex flex-col gap-2.5">
            {data.linked.map((m) => (
              <li key={`${m.source_kind}:${m.source_id}`} className="flex flex-col gap-1">
                <SourceLine m={m} />
                {!compact && m.snippets.map((s, i) => <Snippet key={i} text={s} />)}
                {m.anchors?.length ? (
                  <span className="font-mono text-[11px] text-fg-tertiary">cites #{m.anchors.join(", #")}</span>
                ) : null}
              </li>
            ))}
          </ul>
        ) : (
          <p className="text-xs text-fg-tertiary">Nothing links here yet. Link it from any page, ticket or comment with [[…]].</p>
        )}
      </section>
      {data.unlinked.length > 0 && (
        <section className="flex flex-col gap-2">
          <h3 className="text-[12.5px] text-fg-tertiary">
            Unlinked mentions <span className="font-mono">{data.unlinked.length}</span>
          </h3>
          <ul className="flex flex-col gap-2.5">
            {data.unlinked.map((m) => (
              <li key={`${m.source_kind}:${m.source_id}`} className="flex flex-col gap-1">
                <div className="flex items-center gap-2">
                  <span className="min-w-0 flex-1">
                    <SourceLine m={m} />
                  </span>
                  {(m.source_kind === "artifact" || m.source_kind === "ticket") && id && (
                    <Button
                      variant="ghost"
                      icon="link"
                      className="h-6 px-1.5 text-xs"
                      disabled={link.isPending}
                      onClick={() => link.mutate({ target_kind: kind, target_id: id, source_kind: m.source_kind as "artifact" | "ticket", source_id: m.source_id })}
                    >
                      Link it
                    </Button>
                  )}
                </div>
                {m.snippets.map((s, i) => (
                  <Snippet key={i} text={s} />
                ))}
              </li>
            ))}
          </ul>
          {link.isError && <p className="text-xs text-pill-blocked-fg">{link.error.message}</p>}
        </section>
      )}
    </div>
  );
}

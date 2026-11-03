"use client";

import Link from "next/link";
import { useMemo } from "react";
import type { Schemas } from "@/lib/api/client";
import { citationSources } from "@/lib/chat-context";
import { TargetMark } from "@/components/domain/wikilink";

/** What the assistant read for a reply, as links. Hidden when it read nothing. */
export function Sources({ citations }: { citations?: Schemas["ChatCitation"][] }) {
  const sources = useMemo(() => citationSources(citations), [citations]);
  if (sources.length === 0) return null;
  return (
    <section aria-label="Sources" className="flex max-w-[min(720px,100%)] flex-col gap-1 text-xs text-fg-secondary" data-testid="sources">
      <h3 className="text-[11.5px] font-medium uppercase tracking-wide text-fg-tertiary">Sources</h3>
      <ul className="flex flex-wrap gap-x-3 gap-y-1">
        {sources.map((s) => {
          const body = (
            <>
              <span className="mr-1 inline-block align-[-1px] [&>*]:inline-grid">
                <TargetMark t={s.target as Schemas["LinkTarget"]} size={12} />
              </span>
              {s.label}
              {s.version && <span className="ml-1 font-mono text-[11px] text-fg-tertiary">{s.version}</span>}
              {s.section && <span className="ml-1 text-fg-tertiary">› {s.section}</span>}
            </>
          );
          return (
            <li key={s.key} data-testid="source">
              {s.href ? (
                <Link href={s.href} className="text-accent hover:underline">{body}</Link>
              ) : (
                <span>{body}</span>
              )}
            </li>
          );
        })}
      </ul>
    </section>
  );
}

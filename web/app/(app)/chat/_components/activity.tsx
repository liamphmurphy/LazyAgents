"use client";

import { useMemo } from "react";
import type { LinkTarget } from "@/lib/api/client";
import { useChatRunEvents, useResolvedLinks } from "@/lib/api/hooks";
import { activityOf, toolCalls, type Activity } from "@/lib/chat-ui";
import { LinkTargets, WikiLink } from "@/components/domain/wikilink";

const str = (v: unknown) => (v == null ? "" : typeof v === "string" ? v : JSON.stringify(v));

/** The page or ticket the newest call reads, linked once it resolves. Unknown refs stay plain text (no ghost "create" link). */
function ActivityRef({ refText, label }: { refText: string; label: string }) {
  const resolved = useResolvedLinks([refText]);
  const targets = useMemo(() => {
    const m = new Map<string, LinkTarget>();
    for (const l of resolved.data ?? []) if (l.target.kind !== "unresolved" && l.target.href) m.set(l.ref, l.target);
    return m;
  }, [resolved.data]);
  if (!targets.has(refText)) return <span className="text-fg">{label}</span>;
  return (
    <LinkTargets.Provider value={targets}>
      <WikiLink refText={refText}>{label}</WikiLink>
    </LinkTargets.Provider>
  );
}

function LiveLine({ a }: { a: Activity }) {
  return (
    <p data-testid="live-activity" className="flex min-w-0 items-center gap-2 text-[12.5px] text-fg-secondary">
      <span aria-hidden className="size-3 flex-none animate-spin rounded-full border-2 border-border-strong border-t-accent" />
      <span className="min-w-0 truncate">
        {a.verb}
        {a.ref && a.label && (
          <>
            {" "}
            <ActivityRef refText={a.ref} label={a.label} />
          </>
        )}
        {a.text && ` ${a.text}`}…
      </span>
    </p>
  );
}

/**
 * While a turn runs: the newest tool call as a live line ("Reading [[ADR-012]]…",
 * ADR Decision 5), with every call of the turn under Tool calls.
 */
export function RunActivity({ runId, since }: { runId?: string; since?: string }) {
  const q = useChatRunEvents(runId);
  const calls = useMemo(() => toolCalls(q.data, since), [q.data, since]);
  if (calls.length === 0) return null;
  const latest = calls[calls.length - 1];
  return (
    <>
      <LiveLine a={activityOf(latest)} />
      <details className="rounded-lg border border-border bg-surface px-3 py-2 text-[12.5px]">
        <summary className="cursor-pointer text-fg-secondary">Tool calls ({calls.length})</summary>
        <ul className="mt-2 flex flex-col gap-1 font-mono text-xs">
          {calls.map((c) => (
            <li key={c.key} className="flex gap-2">
              <span className="text-fg">{c.tool}</span>
              <span className="min-w-0 flex-1 truncate text-fg-tertiary">{str(c.input)}</span>
              <span className="text-fg-tertiary">{c.done ? "done" : "running"}</span>
            </li>
          ))}
        </ul>
      </details>
    </>
  );
}

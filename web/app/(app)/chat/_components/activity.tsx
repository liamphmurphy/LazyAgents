"use client";

import { useMemo } from "react";
import type { LinkTarget } from "@/lib/api/client";
import { useChatRunEvents, useResolvedLinks } from "@/lib/api/hooks";
import { activityOf, chatStatus, interimLine, toolCalls, type Activity, type ToolCall } from "@/lib/chat-ui";
import { LinkTargets, WikiLink } from "@/components/domain/wikilink";
import { ActivityScene, sceneLabel, useHeldScene } from "./activity-scene";
import { PersonaAvatar, PersonaName, type PersonaLook } from "./persona";

const str = (v: unknown) => (v == null ? "" : typeof v === "string" ? v : JSON.stringify(v));

/** The page or ticket the newest call reads, linked once it resolves. Unknown refs stay plain text (no ghost "create" link). */
export function ActivityRef({ refText, label }: { refText: string; label: string }) {
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

function CallList({ calls }: { calls: ToolCall[] }) {
  return (
    <details className="w-full rounded-lg border border-border bg-surface px-3 py-2 text-[12.5px]">
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
  );
}

/**
 * While a turn runs: the newest tool call as a live line ("Reading [[ADR-012]]…",
 * ADR Decision 5), with every call of the turn under Tool calls. Shown in the Activity tab only.
 */
export function RunActivity({ runId, since }: { runId?: string; since?: string }) {
  const q = useChatRunEvents(runId);
  const calls = useMemo(() => toolCalls(q.data, since), [q.data, since]);
  if (calls.length === 0) return null;
  const latest = calls[calls.length - 1];
  return (
    <>
      <LiveLine a={activityOf(latest)} />
      <CallList calls={calls} />
    </>
  );
}

/** A finished turn's tool calls, in the Activity tab. */
export function TurnToolCalls({ runId, since, until }: { runId?: string; since?: string; until?: string }) {
  const q = useChatRunEvents(runId, true, false);
  const calls = useMemo(() => toolCalls(q.data, since, until), [q.data, since, until]);
  if (calls.length === 0) return null;
  return <CallList calls={calls} />;
}

/**
 * The chat tab's card while a turn runs: "<Persona> is reading the code…" beside a small
 * scene for that kind of work, from the newest tool call at a high level only. The exact
 * calls stay in the Activity tab. Its height is fixed so scene changes never move the chat.
 */
export function ChatStatus({ runId, since, persona, stopping }: { runId?: string; since?: string; persona: PersonaLook; stopping?: boolean }) {
  const q = useChatRunEvents(runId);
  const status = useMemo(() => chatStatus(toolCalls(q.data, since)), [q.data, since]);
  const scene = useHeldScene(status.kind);
  return (
    <div data-testid="chat-status-card" className="@container flex h-[72px] w-full max-w-[min(720px,100%)] items-center gap-3 rounded-xl border border-border bg-surface px-3.5">
      <span className="flex-none motion-safe:animate-pulse">
        <PersonaAvatar persona={persona} size={28} />
      </span>
      <div className="min-w-0 flex-1">
        <p role="status" aria-live="polite" className="truncate text-[13.5px] text-fg" data-testid="chat-status">
          {stopping ? "Stopping…" : `${persona.name} is ${status.phrase}…`}
        </p>
        <p aria-hidden className="truncate text-xs text-fg-tertiary @max-[300px]:hidden">
          {sceneLabel(scene.current)}
        </p>
      </div>
      <ActivityScene current={scene.current} previous={scene.previous} stopping={stopping} />
    </div>
  );
}

/**
 * The running turn's opening line ("Good question, let me check the runs."), shown as a
 * reply bubble while the assistant works. The prompt asks for one only on turns that
 * need real work; when the turn ends the server saves it as its own message ahead of the reply.
 */
export function InterimReply({ runId, since, persona }: { runId?: string; since?: string; persona: PersonaLook }) {
  const q = useChatRunEvents(runId);
  const line = useMemo(() => interimLine(q.data, since), [q.data, since]);
  if (!line) return null;
  return (
    <div className="flex flex-col items-start gap-1" data-testid="interim-reply">
      <span className="flex items-center gap-1.5 text-xs text-fg-tertiary">
        <PersonaAvatar persona={persona} size={22} />
        <PersonaName persona={persona} className="text-[13px]" />
      </span>
      <p className="max-w-[min(720px,100%)] whitespace-pre-wrap rounded-xl border border-border bg-surface px-3.5 py-2.5 text-[13.5px]">{line}</p>
    </div>
  );
}

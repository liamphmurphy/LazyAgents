"use client";

import { useEffect, useState, type ReactNode } from "react";
import { useQueryClient } from "@tanstack/react-query";
import type { Schemas } from "@/lib/api/client";
import { chatKeys, useCancelChat, useChatMessages } from "@/lib/api/hooks";
import { asideCount } from "@/lib/send-mode";
import { cx } from "@/lib/cx";
import { Icon } from "@/components/ui/icon";
import { ChatMarkdown } from "./chat-markdown";
import { PersonaAvatar, personaLook } from "./persona";

type Aside = Schemas["ChatSession"];

/** What an aside's card can do with its question once answered. */
export type AsideActions = { onQueue: (question: string) => void; onSteer: (question: string) => void; disabled?: boolean };

/**
 * The asides asked during the running reply, under its status card and joined
 * to it by a rail: each answered by a side instance that saw the reply so far.
 */
export function LiveAsides({ asides, actions }: { asides: Aside[]; actions: AsideActions }) {
  const [hidden, setHidden] = useState<Set<string>>(() => new Set());
  const shown = asides.filter((a) => !hidden.has(a.id));
  if (shown.length === 0) return null;
  return (
    <AsideRail testId="live-asides">
      {shown.map((a) => (
        <AsideCard key={a.id} aside={a} live actions={actions} onHide={() => setHidden((h) => new Set(h).add(a.id))} />
      ))}
    </AsideRail>
  );
}

/** On a finished reply: its asides, folded into one chip, since they never became part of the conversation. */
export function ReplyAsides({ asides, name }: { asides: Aside[]; name: string }) {
  const [open, setOpen] = useState(false);
  if (asides.length === 0) return null;
  return (
    <div className="flex flex-col items-start gap-2" data-testid="reply-asides">
      <button
        type="button"
        aria-expanded={open}
        onClick={() => setOpen((o) => !o)}
        className="inline-flex h-[26px] items-center gap-1.5 rounded-full border border-border px-2.5 text-xs text-fg-secondary hover:bg-hover hover:text-fg"
      >
        <Icon name="branch" size={12} />
        {asideCount(asides.length)} · not in {name}&rsquo;s context
        <Icon name="chev" size={11} className={cx("transition-transform", open && "rotate-180")} />
      </button>
      {open && (
        <AsideRail>
          {asides.map((a) => (
            <AsideCard key={a.id} aside={a} />
          ))}
        </AsideRail>
      )}
    </div>
  );
}

function AsideRail({ children, testId }: { children: ReactNode; testId?: string }) {
  return (
    <div data-testid={testId} className="relative ml-[22px] flex w-[calc(100%-22px)] max-w-[698px] flex-col gap-2.5 border-l-[1.5px] border-state-review/35 pl-4">
      {children}
    </div>
  );
}

function AsideCard({ aside, live, actions, onHide }: { aside: Aside; live?: boolean; actions?: AsideActions; onHide?: () => void }) {
  // The persona that was answering the chat when the aside was asked.
  const persona = personaLook(aside.persona);
  const qc = useQueryClient();
  const messages = useChatMessages(aside.id);
  const stop = useCancelChat(aside.id);
  // Refetch as the aside moves on, also without the realtime connection.
  const stamp = `${aside.state}:${aside.message_count}`;
  useEffect(() => {
    void qc.invalidateQueries({ queryKey: chatKeys.messages(aside.id) });
  }, [stamp, aside.id, qc]);
  const list = messages.data?.messages ?? [];
  const question = list.find((m) => m.role === "user")?.body_md ?? aside.title.replace(/^Aside: /, "");
  const answer = list.findLast((m) => m.role === "assistant");
  const failure = aside.state === "failed" ? (list.findLast((m) => m.role === "system")?.body_md ?? aside.error ?? "The aside couldn’t answer.") : null;
  const answering = aside.state === "running";
  return (
    <article
      aria-label={`Aside: ${question}`}
      data-testid="aside"
      className="relative flex flex-col gap-2 rounded-xl border border-state-review/25 bg-surface px-3 py-2.5 before:absolute before:-left-[17px] before:top-[18px] before:w-4 before:border-t-[1.5px] before:border-state-review/35"
    >
      <header className="flex items-center gap-2 text-xs text-fg-tertiary">
        <PersonaAvatar persona={persona} size={20} />
        <span className="text-[12.5px] font-medium text-pill-review-fg">{persona.name}</span>
        <span className="rounded-xs bg-pill-review-bg px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-[0.06em] text-pill-review-fg">Aside</span>
        <span className="min-w-0 truncate">{live ? "sees the live reply · read-only" : "asked during this reply"}</span>
        {live && answering && (
          <button type="button" disabled={stop.isPending} onClick={() => stop.mutate()} className="ml-auto rounded-md px-1.5 py-0.5 hover:bg-hover hover:text-fg disabled:opacity-50">
            Stop
          </button>
        )}
        {live && !answering && onHide && (
          <button type="button" aria-label="Hide aside" onClick={onHide} className="ml-auto grid size-5 place-items-center rounded-md hover:bg-hover hover:text-fg">
            <Icon name="x" size={11} />
          </button>
        )}
      </header>
      <p className="max-w-[90%] self-end whitespace-pre-wrap rounded-xl bg-selected px-3 py-1.5 text-[13.5px] text-fg">{question}</p>
      {answer ? (
        <div className="type-chat text-[13.5px] leading-[1.55] text-fg-2">
          <ChatMarkdown>{answer.body_md}</ChatMarkdown>
        </div>
      ) : failure ? (
        <p role="alert" className="text-[13px] text-pill-blocked-fg">{failure}</p>
      ) : answering ? (
        <p role="status" className="text-shimmer text-[13px]">Reading the live reply: its steps, tool output and what it has written…</p>
      ) : (
        <p className="text-[13px] text-fg-tertiary">Stopped before answering.</p>
      )}
      {live && answer && actions && (
        <div className="flex flex-wrap gap-1.5">
          <button type="button" disabled={actions.disabled} onClick={() => actions.onQueue(question)} className={ACTION}>
            <Icon name="clock" size={12} />
            Queue as follow-up
          </button>
          <button type="button" disabled={actions.disabled} onClick={() => actions.onSteer(question)} className={ACTION}>
            <Icon name="bolt" size={12} />
            Steer with this
          </button>
        </div>
      )}
    </article>
  );
}

const ACTION =
  "inline-flex h-[26px] items-center gap-1.5 rounded-md border border-border-strong px-2 text-xs text-fg-secondary hover:bg-hover hover:text-fg disabled:cursor-not-allowed disabled:opacity-50";

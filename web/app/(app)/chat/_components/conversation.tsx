"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { ApiError, type Schemas } from "@/lib/api/client";
import {
  chatKeys,
  useCancelChat,
  useChat,
  useChatMessages,
  useChatModels,
  useChatRunEvents,
  useSendChatMessage,
} from "@/lib/api/hooks";
import { cx } from "@/lib/cx";
import { Button } from "@/components/ui/button";
import { Banner, EmptyState, Skeleton, TextArea } from "@/components/ui/primitives";
import { ChatMarkdown } from "./chat-markdown";
import { ModelPicker, modelLabel, sameSelection, type Selection } from "./model-picker";

type Message = Schemas["ChatMessage"];
type Groups = Schemas["ChatHarnessModels"][] | undefined;

const errorText = (e: unknown) => (e instanceof Error ? e.message : "Something went wrong. Please try again.");

function Tag({ groups, m }: { groups: Groups; m: Message }) {
  if (!m.harness || !m.model) return null;
  const l = modelLabel(groups, { harness: m.harness, model: m.model });
  return (
    <span className="font-mono text-[11.5px] text-fg-tertiary" data-testid="model-tag">
      {l.harness} · {l.model}
    </span>
  );
}

function Divider({ children, testId }: { children: string; testId?: string }) {
  return (
    <li className="flex items-center gap-3 py-1 text-xs text-fg-tertiary" data-testid={testId}>
      <span aria-hidden className="h-px flex-1 bg-border-subtle" />
      <span>{children}</span>
      <span aria-hidden className="h-px flex-1 bg-border-subtle" />
    </li>
  );
}

function Bubble({ m, groups }: { m: Message; groups: Groups }) {
  if (m.role === "system") return <Divider testId="system-message">{m.body_md}</Divider>;
  const mine = m.role === "user";
  return (
    <li className={cx("flex flex-col gap-1", mine ? "items-end" : "items-start")} data-testid={`${m.role}-message`}>
      <span className="text-xs text-fg-tertiary">
        {mine ? (m.author?.name ?? "You") : "Assistant"}
        {!mine && " · "}
        {!mine && <Tag groups={groups} m={m} />}
      </span>
      <div
        className={cx(
          "max-w-[min(720px,100%)] rounded-xl border px-3.5 py-2.5 text-[13.5px]",
          mine ? "border-border-strong bg-selected" : "border-border bg-surface",
        )}
      >
        <ChatMarkdown>{m.body_md}</ChatMarkdown>
      </div>
    </li>
  );
}

const str = (v: unknown) => (v == null ? "" : typeof v === "string" ? v : JSON.stringify(v));

function ToolCalls({ runId, since }: { runId?: string; since?: string }) {
  const q = useChatRunEvents(runId);
  const calls = useMemo(() => {
    const t = since ? Date.parse(since) : 0;
    const out: Array<{ key: number; tool: string; input: string; done: boolean }> = [];
    const open = new Map<string, number>();
    for (const e of q.data ?? []) {
      if (Date.parse(e.at) < t) continue;
      const p = e.payload;
      if (e.type === "tool_call") {
        if (p.id) open.set(str(p.id), out.length);
        out.push({ key: e.seq, tool: str(p.tool), input: str(p.input), done: false });
      } else if (e.type === "tool_result" && p.id && open.has(str(p.id))) {
        out[open.get(str(p.id))!].done = true;
      }
    }
    return out;
  }, [q.data, since]);
  if (calls.length === 0) return null;
  return (
    <details className="rounded-lg border border-border bg-surface px-3 py-2 text-[12.5px]">
      <summary className="cursor-pointer text-fg-secondary">
        Tool calls ({calls.length})
      </summary>
      <ul className="mt-2 flex flex-col gap-1 font-mono text-xs">
        {calls.map((c) => (
          <li key={c.key} className="flex gap-2">
            <span className="text-fg">{c.tool}</span>
            <span className="min-w-0 flex-1 truncate text-fg-tertiary">{c.input}</span>
            <span className="text-fg-tertiary">{c.done ? "done" : "running"}</span>
          </li>
        ))}
      </ul>
    </details>
  );
}

export function Conversation({ id }: { id: string }) {
  const chat = useChat(id);
  const messages = useChatMessages(id);
  const models = useChatModels();
  const send = useSendChatMessage(id);
  const cancel = useCancelChat(id);
  const qc = useQueryClient();
  const [text, setText] = useState("");
  const [picked, setPicked] = useState<Selection | null>(null);
  const area = useRef<HTMLTextAreaElement>(null);
  const end = useRef<HTMLLIElement>(null);

  const c = chat.data;
  const loaded = !!c;
  const running = c?.state === "running";
  const locked = running || send.isPending;
  const groups: Groups = models.data;
  const current: Selection | undefined = c && { harness: c.harness, model: c.model };
  const selection = picked ?? current;
  const pending = !!(picked && current && !sameSelection(picked, current));
  const list = messages.data?.messages;
  const lastUser = list?.findLast((m) => m.role === "user");

  // Refetch the transcript when the chat changes (also covers polling without SSE).
  const stamp = c && `${c.state}:${c.message_count}:${c.revision}`;
  useEffect(() => {
    if (stamp) void qc.invalidateQueries({ queryKey: chatKeys.messages(id) });
  }, [stamp, id, qc]);
  // Focus returns to the composer on load and whenever a turn ends.
  useEffect(() => {
    if (loaded && !locked) area.current?.focus();
  }, [locked, loaded]);
  useEffect(() => {
    end.current?.scrollIntoView?.({ block: "end" });
  }, [list?.length, running]);

  function submit(body: string) {
    if (!body.trim() || locked) return;
    cancel.reset();
    send.mutate(
      pending && picked ? { body, harness: picked.harness, model: picked.model } : { body },
      {
        onSuccess: () => {
          setText("");
          setPicked(null);
        },
      },
    );
  }

  if (chat.isError) {
    const gone = chat.error instanceof ApiError && chat.error.status === 404;
    return (
      <div className="p-6">
        <Banner tone="blocked" title={gone ? "This chat doesn’t exist" : "Couldn’t load the chat"}>
          {errorText(chat.error)}
        </Banner>
      </div>
    );
  }
  if (!c) {
    return (
      <div aria-busy="true" className="flex flex-1 flex-col gap-4 p-6">
        <Skeleton className="h-6 w-1/3" />
        <Skeleton className="h-16 w-2/3" />
        <Skeleton className="ml-auto h-10 w-1/2" />
      </div>
    );
  }
  const actionError = send.error ?? cancel.error;
  return (
    <section aria-label={c.title} className="flex min-h-0 flex-1 flex-col">
      <div className="flex items-center gap-2 border-b border-border-subtle px-6 py-2.5">
        <h1 className="type-heading truncate">{c.title}</h1>
        {c.archived && <span className="text-xs text-fg-tertiary">Archived</span>}
      </div>
      <div className="flex-1 overflow-y-auto px-6 py-5">
        {messages.isError ? (
          <Banner tone="blocked" title="Couldn’t load messages">{errorText(messages.error)}</Banner>
        ) : !list ? (
          <div aria-busy="true" className="flex flex-col gap-3">
            <Skeleton className="h-12 w-1/2" />
            <Skeleton className="h-16 w-2/3" />
          </div>
        ) : list.length === 0 && !running ? (
          <EmptyState icon="msg" title="Ask anything about this project">
            The assistant can search the wiki, artifacts and tickets. Write a message below to begin.
          </EmptyState>
        ) : (
          <ol aria-label="Conversation" role="log" aria-live="polite" className="mx-auto flex max-w-[860px] flex-col gap-4">
            {messages.data?.has_more && <Divider>Earlier messages are not shown</Divider>}
            {list.map((m) => (
              <Bubble key={m.id} m={m} groups={groups} />
            ))}
            {pending && selection && (
              <Divider testId="pending-switch">
                {`Next message will use ${modelLabel(groups, selection).harness} · ${modelLabel(groups, selection).model}`}
              </Divider>
            )}
            {running && (
              <li className="flex flex-col gap-2" data-testid="thinking">
                <span role="status" className="text-[13px] text-fg-secondary">Thinking…</span>
                <ToolCalls runId={c.current_run_id} since={lastUser?.created_at} />
              </li>
            )}
            <li ref={end} aria-hidden />
          </ol>
        )}
      </div>
      <div className="mx-auto flex w-full max-w-[860px] flex-col gap-2 px-6 pb-5">
        {c.state === "failed" && (
          <Banner
            tone="blocked"
            title="The last turn failed"
            actions={
              lastUser && (
                <Button variant="outline" icon="refresh" disabled={locked} onClick={() => submit(lastUser.body_md)}>
                  Retry
                </Button>
              )
            }
          >
            {c.error ?? "No error details were recorded."}
          </Banner>
        )}
        {actionError && (
          <p role="alert" className="text-[12.5px] text-pill-blocked-fg">
            {actionError instanceof ApiError && actionError.status === 409
              ? `Can’t do that right now: ${actionError.message}`
              : errorText(actionError)}
          </p>
        )}
        {c.archived ? (
          <p className="text-[13px] text-fg-secondary">This chat is archived. Restore it to continue.</p>
        ) : (
          <form
            onSubmit={(e) => {
              e.preventDefault();
              submit(text);
            }}
            className="flex flex-col gap-2"
          >
            <TextArea
              ref={area}
              aria-label="Message"
              rows={3}
              value={text}
              disabled={locked}
              placeholder={locked ? "Waiting for the reply…" : "Message the assistant. Enter sends, Shift+Enter adds a line."}
              onChange={(e) => setText(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter" && !e.shiftKey && !e.nativeEvent.isComposing) {
                  e.preventDefault();
                  submit(text);
                }
              }}
            />
            <div className="flex flex-wrap items-center gap-2">
              {selection && <ModelPicker groups={groups} value={selection} onChange={setPicked} disabled={locked} />}
              <span className="flex-1" />
              {running && (
                <Button variant="outline" icon="pause" disabled={cancel.isPending} onClick={() => cancel.mutate()}>
                  Stop
                </Button>
              )}
              <Button type="submit" variant="primary" disabled={locked || !text.trim()}>
                Send
              </Button>
            </div>
          </form>
        )}
      </div>
    </section>
  );
}

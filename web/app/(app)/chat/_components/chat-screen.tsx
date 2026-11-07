"use client";

import { useRouter, useSearchParams } from "next/navigation";
import { useEffect, useRef, useState } from "react";
import { useBootstrap, useChat, useChats, useCreateChat, useStartChat } from "@/lib/api/hooks";
import { parseAttach, removeChip, toContextRefs, type Chip } from "@/lib/chat-context";
import { Button } from "@/components/ui/button";
import { Banner, Skeleton, TextArea } from "@/components/ui/primitives";
import { TopBar } from "@/components/shell/topbar";
import { ProjectScopeState, projectCrumbs, useProjectScope } from "@/components/shell/project-scope";
import { NoHarnessNote, useHarnessHealth } from "@/components/domain/setup";
import { Conversation } from "./conversation";
import { ChipList } from "./context-chips";
import { MemoryButton, MemoryPanel, useMemoryPanel } from "./memory-panel";

const errorText = (e: unknown) => (e instanceof Error ? e.message : "Something went wrong. Please try again.");

const SUGGESTIONS = [
  "What did my agents finish this week?",
  "What’s blocking my open epics?",
  "Draft tickets from a spec",
  "Search the web for the latest on a topic",
];

/** A first-time conversation (mockup frame 2): greeting, suggestions, and a composer that starts the chat. */
function Welcome({ project, disabled }: { project: string; disabled: boolean }) {
  const router = useRouter();
  const search = useSearchParams();
  const boot = useBootstrap();
  const start = useStartChat();
  const [text, setText] = useState("");
  // A "Start chat" link with no chat yet (?attach=…) lands here; its chips go with the first message.
  const [chips, setChips] = useState<Chip[]>(() => parseAttach(search.getAll("attach"), search.get("attach_title")));
  const area = useRef<HTMLTextAreaElement>(null);
  const name = boot.data?.me.name?.split(" ")[0];
  const locked = disabled || start.isPending;

  function submit() {
    const body = text.trim();
    if (!body || locked) return;
    const context_refs = toContextRefs(chips);
    start.mutate(
      { project, message: { body, ...(context_refs ? { context_refs } : {}) } },
      { onSuccess: (c) => router.push(`/chat/${c.id}?project=${encodeURIComponent(project)}`) },
    );
  }
  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="flex flex-1 flex-col items-center justify-center gap-3 overflow-y-auto px-6 py-8 text-center">
        <span aria-hidden className="grid size-12 place-items-center rounded-full bg-accent/15 text-xl font-semibold text-accent">
          A
        </span>
        <h1 className="text-xl font-semibold text-fg">{name ? `Hi ${name}, what can I help with?` : "What can I help with?"}</h1>
        <p className="max-w-[520px] text-[13px] text-fg-secondary">
          I can read your runs, tickets, epics, wiki and files, and create tickets for you. I remember what matters and tell you when I do.
        </p>
        <ul aria-label="Suggestions" className="mt-2 grid w-full max-w-[520px] grid-cols-2 gap-2 max-sm:grid-cols-1">
          {SUGGESTIONS.map((s) => (
            <li key={s}>
              <button
                type="button"
                disabled={locked}
                onClick={() => {
                  setText(s);
                  area.current?.focus();
                }}
                className="h-full w-full rounded-xl border border-border bg-surface px-3.5 py-3 text-left text-[13px] text-fg hover:bg-hover disabled:opacity-50"
              >
                {s}
              </button>
            </li>
          ))}
        </ul>
        <p className="mt-2 text-[11.5px] text-fg-tertiary">Your memory is private to you and separate from the team wiki.</p>
      </div>
      <form
        className="mx-auto flex w-full max-w-[860px] flex-col gap-2 px-6 pb-5"
        onSubmit={(e) => {
          e.preventDefault();
          submit();
        }}
      >
        {start.isError && (
          <p role="alert" className="text-[12.5px] text-pill-blocked-fg">{errorText(start.error)}</p>
        )}
        <ChipList chips={chips} onRemove={(k) => setChips((cs) => removeChip(cs, k))} disabled={locked} />
        <TextArea
          ref={area}
          aria-label="Message"
          rows={3}
          value={text}
          disabled={locked}
          placeholder="Message your assistant… Enter sends, Shift+Enter adds a line."
          onChange={(e) => setText(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter" && !e.shiftKey && !e.nativeEvent.isComposing) {
              e.preventDefault();
              submit();
            }
          }}
        />
        <div className="flex items-center justify-end">
          <Button type="submit" variant="primary" disabled={locked || !text.trim()}>
            {start.isPending ? "Starting…" : "Send"}
          </Button>
        </div>
      </form>
    </div>
  );
}

/**
 * The Personal Assistant (replaces Chat): one conversation surface, no agent
 * picker and no chat list. /chat opens the newest conversation, or the first-time
 * welcome; "New conversation" starts afresh. Memory opens as a right-hand panel.
 */
export function ChatScreen({ id }: { id?: string }) {
  const router = useRouter();
  const search = useSearchParams();
  const boot = useBootstrap();
  const current = useChat(id);
  // An open chat carries its project; the index resolves ?project=, then the remembered one.
  const scope = useProjectScope("required", { skip: !!id });
  const project = id
    ? (current.data?.project ?? search.get("project") ?? "")
    : scope.status === "ready" ? (scope.project?.key ?? "") : "";
  const scoped = boot.data?.projects.find((p) => p.key === project);
  const memory = useMemoryPanel();
  const create = useCreateChat();
  const health = useHarnessHealth();
  const noHarness = !!health && !health.ready;
  // /chat resumes the newest conversation, unless a "Start chat" link brought context to attach.
  const attaching = search.has("attach");
  const chats = useChats(!id && !attaching ? project : "");
  const newest = chats.data?.chats.reduce<(typeof chats.data.chats)[number] | undefined>((a, c) => {
    const t = (x: typeof c) => Date.parse(x.last_message_at ?? x.updated_at);
    return !a || t(c) > t(a) ? c : a;
  }, undefined);
  useEffect(() => {
    if (id || attaching || !newest) return;
    const q = new URLSearchParams(search.toString());
    q.set("project", project);
    router.replace(`/chat/${newest.id}?${q.toString()}`);
  }, [id, attaching, newest, project, router, search]);

  function newConversation() {
    // An empty current conversation is already new.
    if (id && current.data?.message_count === 0) return;
    create.mutate({ project }, { onSuccess: (c) => router.push(`/chat/${c.id}?project=${encodeURIComponent(project)}`) });
  }
  const actions = (
    <>
      <MemoryButton />
      <Button variant="secondary" icon="plus" disabled={!project || noHarness || create.isPending} onClick={newConversation}>
        New conversation
      </Button>
    </>
  );
  const resolving = !id && !attaching && (!chats.data || !!newest);
  return (
    <div className="flex h-dvh flex-col">
      <TopBar crumbs={projectCrumbs(scoped, "Assistant")} />
      {!id && scope.status !== "ready" ? (
        <ProjectScopeState base="/chat" scope={scope} />
      ) : (
        <div className="relative flex min-h-0 flex-1">
          <div className="flex min-w-0 flex-1 flex-col">
            {(noHarness || create.isError) && (
              <div className="flex flex-col gap-1 px-6 pt-3">
                {noHarness && <NoHarnessNote health={health} />}
                {create.isError && <p role="alert" className="text-xs text-pill-blocked-fg">{errorText(create.error)}</p>}
              </div>
            )}
            {id ? (
              <Conversation key={id} id={id} actions={actions} />
            ) : (
              <>
                <div className="flex items-center justify-end gap-2 border-b border-border-subtle px-6 py-2.5">{actions}</div>
                {chats.isError ? (
                  <div className="p-6">
                    <Banner tone="blocked" title="Couldn’t load your conversations">{errorText(chats.error)}</Banner>
                  </div>
                ) : resolving ? (
                  <div aria-busy="true" className="flex flex-1 flex-col gap-4 p-6">
                    <Skeleton className="h-6 w-1/3" />
                    <Skeleton className="h-16 w-2/3" />
                  </div>
                ) : (
                  <Welcome project={project} disabled={!project || noHarness} />
                )}
              </>
            )}
          </div>
          {memory.open && <MemoryPanel />}
        </div>
      )}
    </div>
  );
}

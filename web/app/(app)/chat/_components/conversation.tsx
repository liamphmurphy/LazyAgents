"use client";

import { useRouter, useSearchParams } from "next/navigation";
import { useEffect, useRef, useState, type ReactNode } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { Tooltip } from "radix-ui";
import { ApiError, type Schemas } from "@/lib/api/client";
import {
  chatKeys,
  useBootstrap,
  useCancelChat,
  useChat,
  useChatMessages,
  useChatModels,
  useCreateChat,
  useDeleteChat,
  usePersonas,
  useSendChatMessage,
} from "@/lib/api/hooks";
import { addChip, chipFromRef, chipFromTarget, parseAttach, removeChip, toContextRefs, type Chip } from "@/lib/chat-context";
import { chatTitle, needsSwitchConfirm } from "@/lib/chat-ui";
import { cx } from "@/lib/cx";
import { Button } from "@/components/ui/button";
import { Banner, EmptyState, Skeleton } from "@/components/ui/primitives";
import { LinkEditor } from "@/components/domain/link-editor";
import { ChatMarkdown } from "./chat-markdown";
import { acceptTarget, AddContext, ChipList, SentChips } from "./context-chips";
import { SaveActions } from "./save-actions";
import { Sources } from "./sources";
import { ModelPicker, modelLabel, sameSelection, type Selection } from "./model-picker";
import { RunActivity } from "./activity";
import { MemoryNotices } from "./memory-notices";
import { AttachButton, AttachmentChips, DropOverlay, SentAttachments, useAttachments, useFileDrop } from "./attachments";
import { WebSearchPill } from "./web-search";
import { RestoreChat } from "./restore";
import { HarnessSwitchDialog } from "./switch-dialog";
import { UsageTag } from "./usage-tag";
import { PersonaAvatar, PersonaGreeting, PersonaName, PersonaSwitcher, personaLook, type PersonaLook } from "./persona";
import { ChatCostChip } from "@/components/domain/cost-chip";
import { unavailableReason } from "@/lib/setup";
import Link from "next/link";

type Message = Schemas["ChatMessage"];
type Groups = Schemas["ChatHarnessModels"][] | undefined;

const errorText = (e: unknown) => (e instanceof Error ? e.message : "Something went wrong. Please try again.");

function Tag({ groups, m }: { groups: Groups; m: Message }) {
  if (!m.harness || !m.model) return null;
  const l = modelLabel(groups, { harness: m.harness, model: m.model });
  return (
    <UsageTag m={m}>
      {l.harness} · {l.model}
    </UsageTag>
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

type ChatInfo = { id: string; title: string; project: string; agent: { name: string } };

/** The window of run events that belongs to an assistant reply: from its turn's message to the next one. */
type Turn = { since?: string; until?: string };

function Bubble({ m, groups, chat, persona, saveable, names, turn }: { m: Message; groups: Groups; chat: ChatInfo; persona: PersonaLook; saveable: boolean; names: Map<string, string>; turn?: Turn }) {
  if (m.role === "system") return <Divider testId="system-message">{m.body_md}</Divider>;
  const mine = m.role === "user";
  return (
    <li className={cx("flex flex-col gap-1", mine ? "items-end" : "items-start")} data-testid={`${m.role}-message`}>
      {mine ? (
        <span className="text-xs text-fg-tertiary">{m.author?.name ?? "You"}</span>
      ) : (
        // Assistant replies carry the persona's avatar and its name in the persona's color.
        <span className="flex items-center gap-1.5 text-xs text-fg-tertiary" data-testid="assistant-identity">
          <PersonaAvatar persona={persona} size={22} />
          <PersonaName persona={persona} className="text-[13px]" />
          <span aria-hidden>·</span>
          <Tag groups={groups} m={m} />
        </span>
      )}
      <div
        className={cx(
          "max-w-[min(720px,100%)] rounded-xl border px-3.5 py-2.5 text-[13.5px]",
          mine ? "border-border-strong bg-selected" : "border-border bg-surface",
        )}
      >
        <ChatMarkdown>{m.body_md}</ChatMarkdown>
      </div>
      {mine && <SentAttachments attachments={m.attachments} />}
      {mine && <SentChips chips={(m.context_refs ?? []).map((r) => chipFromRef(r, names))} />}
      {!mine && <Sources citations={m.citations} />}
      {turn && <WebSearchPill runId={m.run_id} since={turn.since} until={turn.until} />}
      {turn && <MemoryNotices runId={m.run_id} since={turn.since} until={turn.until} />}
      {!mine && saveable && m.body_md.trim() && <SaveActions chat={chat} body={m.body_md} />}
    </li>
  );
}

/**
 * Each turn's last assistant reply gets the window of events between its user
 * message and the next one, so a turn's memory notices show once, under its answer.
 */
function turnWindows(list: Message[] | undefined): Map<string, Turn> {
  const out = new Map<string, Turn>();
  let since: string | undefined;
  let reply: Message | undefined;
  const close = (until?: string) => reply && out.set(reply.id, { since, until });
  for (const m of list ?? []) {
    if (m.role === "user") {
      close(m.created_at);
      since = m.created_at;
      reply = undefined;
    } else if (m.role === "assistant") reply = m;
  }
  close(undefined);
  return out;
}

export function Conversation({ id, actions }: { id: string; actions?: ReactNode }) {
  const chat = useChat(id);
  const messages = useChatMessages(id);
  const models = useChatModels();
  const send = useSendChatMessage(id);
  const cancel = useCancelChat(id);
  const boot = useBootstrap();
  const personas = usePersonas();
  const create = useCreateChat();
  const discard = useDeleteChat();
  const qc = useQueryClient();
  const [text, setText] = useState("");
  const [picked, setPicked] = useState<Selection | null>(null);
  // A pick on another harness waits here until the person confirms what the switch loses.
  const [confirming, setConfirming] = useState<Selection | null>(null);
  const router = useRouter();
  const search = useSearchParams();
  // "Start chat" links arrive with ?attach=kind:id; seed once, then drop it from the URL.
  const [chips, setChips] = useState<Chip[]>(() => parseAttach(search.getAll("attach"), search.get("attach_title")));
  const files = useAttachments();
  const area = useRef<HTMLTextAreaElement>(null);
  const end = useRef<HTMLLIElement>(null);

  const c = chat.data;
  const loaded = !!c;
  const running = c?.state === "running";
  const stopping = running && (cancel.isPending || cancel.isSuccess);
  const locked = running || send.isPending;
  const groups: Groups = models.data;
  const current: Selection | undefined = c && { harness: c.harness, model: c.model };
  const selection = picked ?? current;
  const pending = !!(picked && current && !sameSelection(picked, current));
  // The next message would start a run on a harness that can't run: say why before sending.
  const unavailable = selection && groups?.find((g) => g.harness === selection.harness && !g.available);
  const list = messages.data?.messages;
  const lastUser = list?.findLast((m) => m.role === "user");
  const turns = turnWindows(list);
  const drop = useFileDrop(files.add, !!c && !c.archived && !locked);

  useEffect(() => {
    if (!search.has("attach")) return;
    const q = new URLSearchParams(search.toString());
    q.delete("attach");
    q.delete("attach_title");
    router.replace(`/chat/${id}?${q.toString()}`);
  }, [search, router, id]);
  // Refs only carry ids; remember what the chips were called so sent messages can show it.
  const [names, setNames] = useState(() => new Map<string, string>());
  const attach = (t: Parameters<typeof chipFromTarget>[0]) => {
    const chip = chipFromTarget(t);
    if (chip) setChips((cs) => addChip(cs, chip));
  };

  // Refetch the transcript when the chat changes (also covers polling without SSE).
  const stamp = c && `${c.state}:${c.message_count}:${c.revision}`;
  useEffect(() => {
    if (!stamp) return;
    void qc.invalidateQueries({ queryKey: chatKeys.messages(id) });
    // A finished turn's last tool results (memory notices) may have landed after the final poll.
    void qc.invalidateQueries({ queryKey: ["runs", "chat"] });
  }, [stamp, id, qc]);
  // Focus returns to the composer on load and whenever a turn ends.
  useEffect(() => {
    if (loaded && !locked) area.current?.focus();
  }, [locked, loaded]);
  useEffect(() => {
    end.current?.scrollIntoView?.({ block: "end" });
  }, [list?.length, running]);

  function submit(body: string) {
    if (!body.trim() || locked || unavailable || files.pending) return;
    cancel.reset();
    const context_refs = toContextRefs(chips);
    setNames((n) => new Map([...n, ...chips.map((c): [string, string] => [c.key, c.label])]));
    send.mutate(
      { body, ...(pending && picked ? { harness: picked.harness, model: picked.model } : {}), ...(context_refs ? { context_refs } : {}), ...(files.ids.length ? { attachment_ids: files.ids } : {}) },
      {
        onSuccess: () => {
          files.clear();
          setText("");
          setPicked(null);
          setChips([]);
        },
      },
    );
  }

  // Before the first message the persona can still change: a chat keeps its persona for life,
  // so switching starts a fresh chat with the other persona and drops this empty one.
  const fresh = !!c && !c.archived && c.message_count === 0 && !running && !list?.length;
  function switchPersona(personaId: string) {
    if (!c || personaId === c.persona?.id || create.isPending) return;
    create.mutate(
      { project: c.project, persona_id: personaId, harness: c.harness, model: c.model },
      {
        onSuccess: (next) => {
          router.replace(`/chat/${next.id}?project=${encodeURIComponent(c.project)}`);
          discard.mutate(c.id);
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
  const actionError = send.error ?? cancel.error ?? create.error;
  // A chat without a persona, or whose persona was deleted, is the built-in Assistant.
  const persona = personaLook(c.persona);
  const full = personas.data?.personas.find((p) => p.id === c.persona?.id);
  // Restoring a chat opened from the Archived list shows it under Active again.
  const restored = () => {
    if (!search.has("view")) return;
    const q = new URLSearchParams(search.toString());
    q.delete("view");
    router.replace(`/chat/${id}?${q.toString()}`);
  };
  const pick = (s: Selection) => {
    if (current && needsSwitchConfirm(current, s, picked, c.message_count)) setConfirming(s);
    else setPicked(s);
  };
  return (
    <Tooltip.Provider delayDuration={200}>
      <section aria-label={chatTitle(c)} className="relative flex min-h-0 flex-1 flex-col" {...drop.props}>
        <DropOverlay over={drop.over} />
        <div className="flex items-center gap-2 border-b border-border-subtle px-6 py-2.5">
          <h1 className="type-heading truncate">{chatTitle(c)}</h1>
          <span
            className="flex flex-none items-center gap-1.5 rounded-full border border-border bg-surface py-0.5 pl-1 pr-2.5 text-xs max-sm:hidden"
            aria-label={`Persona: ${persona.name}`}
          >
            <PersonaAvatar persona={persona} size={16} />
            <PersonaName persona={persona} />
          </span>
          {c.archived && <span className="flex-none rounded-full bg-selected px-2 py-0.5 text-[11.5px] text-fg-secondary">Archived</span>}
          <ChatCostChip chatId={c.id} project={c.project} />
          <span className="flex-1" />
          {c.archived ? (
            <RestoreChat chat={c} onRestored={restored} />
          ) : (
            current && (
              <span className="truncate font-mono text-[11.5px] text-fg-tertiary max-sm:hidden">
                {modelLabel(groups, current).harness} · {modelLabel(groups, current).model}
              </span>
            )
          )}
          {actions}
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
            c.archived ? (
              <EmptyState icon="msg" title="A new conversation">
                Ask about your runs, tickets, epics and wiki, or have me draft tickets. I save useful things to your private memory and tell you when I do.
              </EmptyState>
            ) : (
              <PersonaGreeting
                persona={persona}
                tone={full?.tone}
                isDefault={!!c.persona && c.persona.id === personas.data?.default_persona_id}
                firstName={boot.data?.me.name?.split(" ")[0]}
                canSwitch={!!personas.data?.personas.length}
              />
            )
          ) : (
            <ol aria-label="Conversation" role="log" aria-live="polite" className="mx-auto flex max-w-[860px] flex-col gap-4">
              {messages.data?.has_more && <Divider>Earlier messages are not shown</Divider>}
              {list.map((m) => (
                <Bubble key={m.id} m={m} groups={groups} chat={c} persona={persona} saveable={!running} names={names} turn={turns.get(m.id)} />
              ))}
              {pending && selection && (
                <Divider testId="pending-switch">
                  {`Next message will use ${modelLabel(groups, selection).harness} · ${modelLabel(groups, selection).model}`}
                </Divider>
              )}
              {running && (
                <li className="flex flex-col gap-2" data-testid="thinking">
                  <span role="status" className="text-[13px] text-fg-secondary">{stopping ? "Stopping…" : "Thinking…"}</span>
                  <RunActivity runId={c.current_run_id} since={lastUser?.created_at} />
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
                  <Button variant="outline" icon="refresh" disabled={locked || !!unavailable} onClick={() => submit(lastUser.body_md)}>
                    Retry
                  </Button>
                )
              }
            >
              {c.error ?? "No error details were recorded."}
            </Banner>
          )}
          {unavailable && !c.archived && (
            <Banner
              tone="human"
              title={`${unavailable.label} is unavailable`}
              actions={
                <Link href="/settings#secrets" className="text-[12.5px] text-accent hover:underline">
                  Add the key →
                </Link>
              }
            >
              {unavailableReason(unavailable)}. Pick another model or add the key.
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
            <div className="flex items-center gap-3 rounded-lg border border-border bg-surface px-4 py-2.5">
              <p className="flex-1 text-[13px] text-fg-secondary">This chat is archived. Restore it to continue.</p>
              <RestoreChat chat={c} variant="primary" onRestored={restored} />
            </div>
          ) : (
            <form
              onSubmit={(e) => {
                e.preventDefault();
                submit(text);
              }}
              className="flex flex-col gap-2"
            >
              <AttachmentChips attachments={files} />
              <ChipList chips={chips} onRemove={(k) => setChips((cs) => removeChip(cs, k))} disabled={locked} />
              <LinkEditor
                inputRef={(el) => {
                  area.current = el;
                }}
                mono={false}
                aria-label="Message"
                rows={3}
                value={text}
                onChange={setText}
                onPick={attach}
                accept={acceptTarget}
                disabled={locked}
                placeholder={locked ? "Waiting for the reply…" : `Message ${c.persona ? persona.name : "your assistant"}… Type [[ to attach context. Enter sends, Shift+Enter adds a line.`}
                onKeyDown={(e) => {
                  if (e.key === "Enter" && !e.shiftKey && !e.nativeEvent.isComposing) {
                    e.preventDefault();
                    submit(text);
                  }
                }}
              />
              <div className="flex flex-wrap items-center gap-2">
                {fresh && <PersonaSwitcher value={c.persona?.id} onChange={switchPersona} disabled={locked || create.isPending} />}
                <AttachButton onFiles={files.add} disabled={locked} />
                <AddContext onPick={attach} onClosed={() => area.current?.focus()} disabled={locked} />
                {selection && <ModelPicker groups={groups} value={selection} onChange={pick} disabled={locked} />}
                <span className="flex-1" />
                {running && (
                  <Button variant="outline" icon="pause" disabled={stopping} onClick={() => cancel.mutate()}>
                    {stopping ? "Stopping…" : "Stop"}
                  </Button>
                )}
                <Button type="submit" variant="primary" disabled={locked || !!unavailable || files.pending || !text.trim()}>
                  Send
                </Button>
              </div>
            </form>
          )}
        </div>
        {current && (
          <HarnessSwitchDialog
            from={modelLabel(groups, current)}
            to={confirming && modelLabel(groups, confirming)}
            agent={c.agent.name || "The agent"}
            onCancel={() => setConfirming(null)}
            onClosed={() => area.current?.focus()}
            onConfirm={() => {
              setPicked(confirming);
              setConfirming(null);
            }}
          />
        )}
      </section>
    </Tooltip.Provider>
  );
}

"use client";

import { useRouter, useSearchParams } from "next/navigation";
import { useEffect, useRef, useState, type ReactNode } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { Tooltip } from "radix-ui";
import { api, ApiError, unwrap, type Schemas } from "@/lib/api/client";
import {
  chatKeys,
  useBootstrap,
  useCancelChat,
  useChat,
  useChatMessages,
  useChatModels,
  useCreateChat,
  useDeleteChat,
  useSendChatMessage,
} from "@/lib/api/hooks";
import { usePersonas } from "@/lib/api/persona-hooks";
import { Tabs } from "@/components/ui/controls";
import { addChip, chipFromRef, chipFromTarget, parseAttach, removeChip, toContextRefs, type Chip } from "@/lib/chat-context";
import { chatTitle, needsSwitchConfirm, runningSince, turnWindows, type TurnWindow } from "@/lib/chat-ui";
import { codesInRepos, insertMention, mentionQuery, mentionSuggestions, repoShortName } from "@/lib/persona-ui";
import { cx } from "@/lib/cx";
import { Button } from "@/components/ui/button";
import { Icon } from "@/components/ui/icon";
import { Banner, EmptyState, Skeleton } from "@/components/ui/primitives";
import { LinkEditor } from "@/components/domain/link-editor";
import { ChatMarkdown } from "./chat-markdown";
import { CodeChanges } from "./code-changes";
import { acceptTarget, AddContext, ChipList, SentChips } from "./context-chips";
import { SaveActions } from "./save-actions";
import { Sources } from "./sources";
import { EffortPicker, ModelPicker, effortLabel, modelEfforts, modelLabel, sameSelection, type Effort, type Selection } from "./model-picker";
import { ChatStatus, InterimReply, RunActivity, TurnToolCalls } from "./activity";
import { MemoryNotices } from "./memory-notices";
import { AttachButton, AttachmentChips, DropOverlay, ReplyAttachments, SentAttachments, useAttachments, useFileDrop } from "./attachments";
import { WebSearchPill } from "./web-search";
import { RestoreChat } from "./restore";
import { HarnessSwitchDialog } from "./switch-dialog";
import { UsageTag } from "./usage-tag";
import { PersonaAvatar, PersonaGreeting, PersonaName, PersonaStack, PersonaSwitcher, RepoSelect, personaLook, type PersonaLook } from "./persona";
import { PersonaProposals } from "./persona-proposals";
import { HumanAvatar } from "@/components/domain/avatars";
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

function Bubble({ m, chat, persona, saveable, names }: { m: Message; chat: ChatInfo; persona: PersonaLook; saveable: boolean; names: Map<string, string> }) {
  if (m.role === "system") return <Divider testId="system-message">{m.body_md}</Divider>;
  const mine = m.role === "user";
  const author = m.persona ? personaLook(m.persona) : persona;
  return (
    <li className={cx("flex flex-col gap-1", mine ? "items-end" : "items-start")} data-testid={`${m.role}-message`}>
      {mine ? (
        <span className="flex items-center gap-1.5 text-xs text-fg-tertiary">
          {m.author?.avatar_url && <HumanAvatar name={m.author.name} initials={m.author.initials} color={m.author.avatar_color} src={m.author.avatar_url} size={18} label={false} />}
          {m.author?.name ?? "You"}
        </span>
      ) : (
        // Assistant replies carry the avatar of the persona that wrote them (the chat's host
        // when unset) and its name in its color, so a group chat reads like one.
        // The harness · model tag and usage live in the Activity tab, so the chat stays immersive.
        <span className="flex items-center gap-1.5 text-xs text-fg-tertiary" data-testid="assistant-identity">
          <PersonaAvatar persona={author} size={22} />
          <PersonaName persona={author} className="text-[13px]" />
        </span>
      )}
      <div
        className={cx(
          "max-w-[min(720px,100%)] rounded-xl border px-3.5 py-2.5 text-[13.5px]",
          mine ? "border-border-strong bg-selected" : "border-border bg-surface",
        )}
        style={mine || !m.persona ? undefined : { borderLeftColor: author.color, borderLeftWidth: 3 }}
      >
        <ChatMarkdown>{m.body_md}</ChatMarkdown>
      </div>
      {mine ? <SentAttachments attachments={m.attachments} /> : <ReplyAttachments attachments={m.attachments} />}
      {!mine && <CodeChanges changes={m.code_changes} />}
      {mine && <SentChips chips={(m.context_refs ?? []).map((r) => chipFromRef(r, names))} />}
      {!mine && <Sources citations={m.citations} />}
      {!mine && saveable && m.body_md.trim() && <SaveActions chat={chat} body={m.body_md} />}
    </li>
  );
}

const snippet = (s: string) => (s.length > 120 ? `${s.slice(0, 120).trimEnd()}…` : s);

/**
 * Everything behind the replies: per turn, the model and usage, tool calls, web searches
 * and memory changes (with Undo), plus the running turn's live activity. Kept out of the
 * chat tab so the conversation reads as if no tools ran.
 */
function ActivityLog({ list, turns, groups, running, runId, since }: { list: Message[] | undefined; turns: Map<string, TurnWindow>; groups: Groups; running: boolean; runId?: string; since?: string }) {
  const replies = (list ?? []).filter((m) => turns.has(m.id));
  const prompt = new Map<string, string>();
  let last: Message | undefined;
  for (const m of list ?? []) {
    if (m.role === "user") last = m;
    else if (turns.get(m.id)?.followup) prompt.set(m.id, "Follow-up: reacting to the generated images");
    else if (turns.has(m.id) && last) prompt.set(m.id, last.body_md);
  }
  if (!replies.length && !running) {
    return <EmptyState icon="msg" title="No activity yet">Tool calls, web searches and memory changes show here once the assistant replies.</EmptyState>;
  }
  return (
    <ol aria-label="Activity" className="mx-auto flex max-w-[860px] flex-col gap-4">
      {replies.map((m) => {
        const t = turns.get(m.id)!;
        return (
          <li key={m.id} data-testid="activity-turn" className="flex flex-col items-start gap-1.5 border-b border-border-subtle pb-4">
            <span className="flex w-full items-center gap-2 text-xs text-fg-tertiary">
              <span className="min-w-0 flex-1 truncate text-fg-secondary">{snippet(prompt.get(m.id) ?? "")}</span>
              <Tag groups={groups} m={m} />
            </span>
            <TurnToolCalls runId={m.run_id} since={t.since} until={t.until} />
            <WebSearchPill runId={m.run_id} since={t.since} until={t.until} />
            <MemoryNotices runId={m.run_id} since={t.since} until={t.until} />
          </li>
        );
      })}
      {running && (
        <li className="flex flex-col gap-2" data-testid="activity-running">
          <span className="text-xs text-fg-secondary">Current turn</span>
          <RunActivity runId={runId} since={since} />
        </li>
      )}
    </ol>
  );
}

const DRAFT_KEY = (id: string) => `lazyagents.chat-draft:${id}`;
function putDraft(id: string, text: string) {
  try {
    sessionStorage.setItem(DRAFT_KEY(id), text);
  } catch {
    /* No storage: the draft is lost, as before. */
  }
}
function readDraft(id: string): string {
  try {
    return sessionStorage.getItem(DRAFT_KEY(id)) ?? "";
  } catch {
    return "";
  }
}
function dropDraft(id: string) {
  try {
    sessionStorage.removeItem(DRAFT_KEY(id));
  } catch {
    /* Nothing stored. */
  }
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
  // A persona switch hands the unsent draft over to the new chat.
  const [text, setText] = useState(() => readDraft(id));
  // Read once into state, then forget it (an effect, so a StrictMode re-render still sees it).
  useEffect(() => dropDraft(id), [id]);
  const [picked, setPicked] = useState<Selection | null>(null);
  // null keeps the chat's effort; "default" clears it.
  const [pickedEffort, setPickedEffort] = useState<Effort | "default" | null>(null);
  const [tab, setTab] = useState<"chat" | "activity">("chat");
  // A pick on another harness waits here until the person confirms what the switch loses.
  const [confirming, setConfirming] = useState<Selection | null>(null);
  const router = useRouter();
  const search = useSearchParams();
  // "Start chat" links arrive with ?attach=kind:id; seed once, then drop it from the URL.
  const [chips, setChips] = useState<Chip[]>(() => parseAttach(search.getAll("attach"), search.get("attach_title")));
  const files = useAttachments();
  const area = useRef<HTMLTextAreaElement>(null);
  const end = useRef<HTMLLIElement>(null);
  // The @mention being typed in the composer, and which suggestion is highlighted.
  const [mention, setMention] = useState<{ start: number; query: string } | null>(null);
  const [mentionActive, setMentionActive] = useState(0);

  const c = chat.data;
  const loaded = !!c;
  const running = c?.state === "running";
  const stopping = running && (cancel.isPending || cancel.isSuccess);
  // Also locked while a persona switch creates the replacement chat, so nothing is sent to the one it drops.
  // A running reply doesn't lock the composer: a message sent then interrupts it,
  // and the next reply answers both, picking up from what the stopped one had done.
  const locked = send.isPending || create.isPending;
  const groups: Groups = models.data;
  const current: Selection | undefined = c && { harness: c.harness, model: c.model };
  const selection = picked ?? current;
  const pending = !!(picked && current && !sameSelection(picked, current));
  const offered = selection ? modelEfforts(groups, selection) : { efforts: [] };
  // The effort the next message runs at; one the selected model lacks falls back to the default, as on the server.
  const wanted = pickedEffort === null ? c?.effort : pickedEffort === "default" ? undefined : pickedEffort;
  const effort = wanted && offered.efforts.includes(wanted) ? wanted : undefined;
  const effortPending = !!c && effort !== c.effort;
  // The next message would start a run on a harness that can't run: say why before sending.
  const unavailable = selection && groups?.find((g) => g.harness === selection.harness && !g.available);
  const list = messages.data?.messages;
  const lastUser = list?.findLast((m) => m.role === "user");
  // The running turn starts after the newest message, which may be a reply, not
  // the person's: a persona handing on, or the image follow-up.
  const since = runningSince(list) ?? lastUser?.created_at;
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
      {
        body,
        ...(pending && picked ? { harness: picked.harness, model: picked.model } : {}),
        ...(effortPending ? { effort: effort ?? "default" } : {}),
        ...(context_refs ? { context_refs } : {}), ...(files.ids.length ? { attachment_ids: files.ids } : {}) },
      {
        onSuccess: () => {
          files.clear();
          setText("");
          setPicked(null);
          setPickedEffort(null);
          setChips([]);
        },
      },
    );
  }

  // Before the first message the persona can still change: a chat keeps its persona for life,
  // so switching starts a fresh chat with the other persona and drops this empty one.
  const fresh = !!c && !c.archived && c.message_count === 0 && !running && !list?.length;
  function switchPersona(personaId: string, repo?: string) {
    if (!c || (personaId === c.persona?.id && !repo) || create.isPending || send.isPending || send.isSuccess) return;
    const old = c;
    create.mutate(
      { project: old.project, persona_id: personaId, harness: old.harness, model: old.model, ...(old.effort ? { effort: old.effort } : {}), ...(repo ? { repo_url: repo } : {}) },
      {
        onSuccess: async (next) => {
          if (text.trim()) putDraft(next.id, text);
          router.replace(`/chat/${next.id}?project=${encodeURIComponent(old.project)}`);
          // Only drop the old chat if it is still empty (a message may have come from another tab).
          // If the check or the delete fails, the empty chat just stays in the list.
          const latest = await unwrap(api.GET("/chats/{id}", { params: { path: { id: old.id } } })).catch(() => undefined);
          if (latest && latest.message_count === 0 && latest.state !== "running") discard.mutate(old.id);
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
  const participants = (c.participants ?? (c.persona ? [c.persona] : [])).map(personaLook);
  const group = participants.length > 1;
  // Who answers next (or is answering now): the last speaker, else the host.
  const speaker = personaLook(c.speaker ?? c.persona);
  const suggestions = mention ? mentionSuggestions(personas.data?.personas ?? [], mention.query) : [];
  const syncMention = (value: string) => {
    const el = area.current;
    const next = el ? mentionQuery(value, el.selectionStart) : null;
    if (next?.query !== mention?.query) setMentionActive(0);
    setMention(next);
  };
  const pickMention = (name: string) => {
    const el = area.current;
    if (!el || !mention) return;
    const next = insertMention(text, mention.start, el.selectionStart, name);
    setText(next.text);
    setMention(null);
    requestAnimationFrame(() => {
      el.focus();
      el.setSelectionRange(next.caret, next.caret);
    });
  };
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
          {group ? (
            <span
              className="flex min-w-0 flex-none items-center gap-1.5 rounded-full border border-border bg-surface py-0.5 pl-1 pr-2.5 text-xs max-sm:hidden"
              data-testid="chat-participants"
              title={participants.map((p) => p.name).join(", ")}
            >
              <span className="sr-only">Personas: </span>
              <PersonaStack personas={participants} />
              <span className="truncate">
                {participants.map((p, i) => (
                  <span key={p.id ?? p.name}>
                    {i > 0 && <span className="text-fg-tertiary">, </span>}
                    <PersonaName persona={p} />
                  </span>
                ))}
              </span>
            </span>
          ) : (
            <span className="flex flex-none items-center gap-1.5 rounded-full border border-border bg-surface py-0.5 pl-1 pr-2.5 text-xs max-sm:hidden">
              <span className="sr-only">Persona: </span>
              <PersonaAvatar persona={persona} size={16} />
              <PersonaName persona={persona} />
            </span>
          )}
          {c.repo_url && (
            <span
              data-testid="repo-chip"
              title={c.repo_url}
              className="flex min-w-0 flex-none items-center gap-1.5 rounded-full border border-border bg-surface px-2.5 py-0.5 text-xs text-fg-secondary max-md:hidden"
            >
              <Icon name="branch" size={13} className="flex-none" />
              <span className="truncate text-fg">{repoShortName(c.repo_url)}</span>
              {c.work_branch && <span className="truncate font-mono text-[11.5px]">{c.work_branch}</span>}
            </span>
          )}
          {c.archived && <span className="flex-none rounded-full bg-selected px-2 py-0.5 text-[11.5px] text-fg-secondary">Archived</span>}
          <span className="flex-1" />
          <Tabs
            label="Chat view"
            value={tab}
            onChange={(k) => setTab(k === "activity" ? "activity" : "chat")}
            items={[
              { key: "chat", label: "Chat" },
              { key: "activity", label: "Activity" },
            ]}
          />
          {c.archived && <RestoreChat chat={c} onRestored={restored} />}
          {actions}
        </div>
        {/* Mounted while hidden so memory notices still refresh the header count. */}
        <div role="tabpanel" aria-label="Activity" hidden={tab !== "activity"} className="flex-1 overflow-y-auto px-6 py-5">
          <div className="mx-auto mb-4 flex max-w-[860px] items-center gap-2">
            <ChatCostChip chatId={c.id} project={c.project} />
            {current && (
              <span className="truncate font-mono text-[11.5px] text-fg-tertiary">
                {modelLabel(groups, current).harness} · {modelLabel(groups, current).model}
                {c.effort && ` · ${effortLabel(c.effort)} effort`}
              </span>
            )}
          </div>
          <ActivityLog list={list} turns={turns} groups={groups} running={running} runId={c.current_run_id} since={since} />
        </div>
        <div role="tabpanel" aria-label="Chat" hidden={tab !== "chat"} className="flex-1 overflow-y-auto px-6 py-5">
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
                <Bubble key={m.id} m={m} chat={c} persona={persona} saveable={!running} names={names} />
              ))}
              <PersonaProposals chatId={c.id} disabled={c.archived} />
              {pending && selection && (
                <Divider testId="pending-switch">
                  {`Next message will use ${modelLabel(groups, selection).harness} · ${modelLabel(groups, selection).model}`}
                </Divider>
              )}
              {effortPending && (
                <Divider testId="pending-effort">{`Next message will use ${effortLabel(effort).toLowerCase()} effort`}</Divider>
              )}
              {running && (
                <li className="flex flex-col gap-2" data-testid="thinking">
                  <InterimReply runId={c.current_run_id} since={since} persona={speaker} />
                  <ChatStatus runId={c.current_run_id} since={since} persona={speaker} stopping={stopping} />
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
              data-dock-avoid=""
              onSubmit={(e) => {
                e.preventDefault();
                submit(text);
              }}
              className="flex flex-col gap-2"
            >
              <AttachmentChips attachments={files} />
              <ChipList chips={chips} onRemove={(k) => setChips((cs) => removeChip(cs, k))} disabled={locked} />
              <div className="relative">
                {suggestions.length > 0 && (
                  <ul
                    role="listbox"
                    aria-label="Mention a persona"
                    data-testid="mention-suggestions"
                    className="absolute bottom-full left-2 z-10 mb-1 flex min-w-[200px] flex-col rounded-lg border border-border bg-surface-raised p-1 shadow-lg"
                  >
                    {suggestions.map((p, i) => (
                      <li key={p.id} role="option" aria-selected={i === mentionActive}>
                        <button
                          type="button"
                          onMouseDown={(e) => e.preventDefault()}
                          onMouseEnter={() => setMentionActive(i)}
                          onClick={() => pickMention(p.name)}
                          className={cx("flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-[13px]", i === mentionActive && "bg-selected")}
                        >
                          <PersonaAvatar persona={personaLook(p)} size={18} />
                          <PersonaName persona={personaLook(p)} />
                          {participants.some((x) => x.id === p.id) && <span className="ml-auto text-[11px] text-fg-tertiary">in chat</span>}
                        </button>
                      </li>
                    ))}
                  </ul>
                )}
                <LinkEditor
                  inputRef={(el) => {
                    area.current = el;
                  }}
                  mono={false}
                  aria-label="Message"
                  rows={3}
                  value={text}
                  onChange={(v) => {
                    setText(v);
                    syncMention(v);
                  }}
                  onBlur={() => setTimeout(() => setMention(null), 120)}
                  onPick={attach}
                  accept={acceptTarget}
                  disabled={locked}
                  placeholder={
                    locked
                      ? "Sending…"
                      : running
                        ? `${c.speaker || c.persona ? speaker.name : "Your assistant"} is replying. Send a message to interrupt: the reply picks up from where it was, with your new message.`
                        : `Message ${c.speaker || c.persona ? speaker.name : "your assistant"}… Type @ to bring in another persona, [[ to attach context. Enter sends, Shift+Enter adds a line.`
                  }
                  onKeyDown={(e) => {
                    if (suggestions.length > 0) {
                      if (e.key === "ArrowDown" || e.key === "ArrowUp") {
                        e.preventDefault();
                        setMentionActive((a) => (a + (e.key === "ArrowDown" ? 1 : suggestions.length - 1)) % suggestions.length);
                        return;
                      }
                      if ((e.key === "Enter" || e.key === "Tab") && !e.nativeEvent.isComposing) {
                        e.preventDefault();
                        pickMention(suggestions[Math.min(mentionActive, suggestions.length - 1)].name);
                        return;
                      }
                      if (e.key === "Escape") {
                        e.preventDefault();
                        setMention(null);
                        return;
                      }
                    }
                    if (e.key === "Enter" && !e.shiftKey && !e.nativeEvent.isComposing) {
                      e.preventDefault();
                      submit(text);
                    }
                  }}
                />
              </div>
              <div className="flex flex-wrap items-center gap-2">
                {fresh && <PersonaSwitcher value={c.persona?.id} onChange={switchPersona} disabled={locked || create.isPending} />}
                {fresh && c.persona && c.repo_url && (full?.repos.length ?? 0) > 1 && codesInRepos(full) && (
                  <RepoSelect repos={full!.repos} value={c.repo_url} onChange={(r) => switchPersona(c.persona!.id, r)} disabled={locked || create.isPending} />
                )}
                <AttachButton onFiles={files.add} disabled={locked} />
                <AddContext onPick={attach} onClosed={() => area.current?.focus()} disabled={locked} />
                {selection && <ModelPicker groups={groups} value={selection} onChange={pick} disabled={locked || running} />}
                <EffortPicker
                  efforts={offered.efforts}
                  defaultEffort={offered.defaultEffort}
                  value={effort}
                  onChange={(e) => setPickedEffort(e ?? "default")}
                  disabled={locked || running}
                />
                <span className="flex-1" />
                {running && (
                  <Button variant="outline" icon="pause" disabled={stopping} onClick={() => cancel.mutate()}>
                    {stopping ? "Stopping…" : "Stop"}
                  </Button>
                )}
                <Button
                  type="submit"
                  variant="primary"
                  disabled={locked || !!unavailable || files.pending || !text.trim()}
                  title={running ? "Stop the reply and answer this together with your last message" : undefined}
                >
                  {running ? "Interrupt" : "Send"}
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

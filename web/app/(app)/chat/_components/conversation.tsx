"use client";

import { canvasFenceIds } from "@/lib/chat-canvas";
import { DOC_DRAFT, designRequest, type CreateKind } from "@/lib/chat-commands";
import { useRouter, useSearchParams } from "next/navigation";
import { memo, useCallback, useEffect, useMemo, useRef, useState, type ReactNode, type RefObject } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { Tooltip } from "radix-ui";
import { useEntrances } from "@/components/ui/motion";
import { api, ApiError, unwrap, type Schemas } from "@/lib/api/client";
import {
  chatKeys,
  useBootstrap,
  useCancelChat,
  useChat,
  useChatAsides,
  useChatMessages,
  useEditLastChatMessage,
  useChatModels,
  useCreateChat,
  useDeleteChat,
  useQueueFollowup,
  useSendChatMessage,
  useSetChatCommitEmail,
  useStartAside,
} from "@/lib/api/hooks";
import { asidesInWindow, commandSendMode, cycleSendMode, SEND_MODE_ENTER, SEND_MODE_HINT, SEND_MODE_LABEL, sendModeAction, sendModePlaceholder, type SendMode } from "@/lib/send-mode";
import { SEND_TONE, SendModeSwitch } from "./send-modes";
import { FollowupTray } from "./followups";
import { LiveAsides, ReplyAsides } from "./asides";
import { RepositoryPicker } from "./repository-picker";
import { useRepositories, useSetChatRepository } from "@/lib/api/repository-hooks";
import { usePersonas } from "@/lib/api/persona-hooks";
import { useWorkflows, type Workflow } from "@/lib/api/workflow-hooks";
import { ChatWorkflowTray, WorkflowStartCard } from "@/components/domain/workflow-cards";
import { ChatRunsPill, ChatTicketRunTray } from "@/components/domain/dispatch-cards";
import { ActivityScene } from "./activity-scene";
import { focusActivityStep, useActivityFocus } from "./activity-focus";
import { setDock } from "@/lib/chat-panels";
import { addChip, chipFromRef, chipFromTarget, parseAttach, toContextRefs, type Chip } from "@/lib/chat-context";
import { chatTitle, needsSwitchConfirm, runningSince, turnWindows, type TurnWindow } from "@/lib/chat-ui";
import { cx } from "@/lib/cx";
import { Button } from "@/components/ui/button";
import { Icon } from "@/components/ui/icon";
import { Banner, EmptyState, Skeleton } from "@/components/ui/primitives";
import { ChatMarkdown } from "./chat-markdown";
import { CodeChanges } from "./code-changes";
import { SentChips } from "./context-chips";
import { SaveActions } from "./save-actions";
import { Sources } from "./sources";
import { splitSourceList } from "@/lib/chat-sources";
import { AskAboutSelection, CopyButton, quoteForComposer, ResearchToggle, TemporaryBadge, TemporaryToggle } from "./reply-tools";
import { ChatSuggestions } from "./suggestions";
import { Composer, type CommandRun } from "./composer";
import { RecommendActions, SaveSummary, VideoPlayers } from "@/components/domain/recommend-fence";
import { VideoCards, VideoRendererNotes } from "@/components/domain/video-card";
import { useChatVideos } from "@/lib/api/video-hooks";
import type { ChatVideo } from "@/lib/chat-video";
import { CiteContext } from "@/components/domain/cite-marker";
import { summaryRequestUrl } from "@/lib/chat-recommend";
import { hostOf } from "@/lib/web-search";
import { timelineTime } from "@/lib/format";
import { ModelEffortPicker, effortLabel, modelEffortSummary, modelEfforts, modelLabel, sameSelection, type Effort, type Selection } from "./model-picker";
import { ChatStatus, InterimReply, RunActivity, TurnToolCalls } from "./activity";
import { MemoryNotices } from "./memory-notices";
import { DropOverlay, ReplyAttachments, SentAttachments, useAttachments, useFileDrop } from "./attachments";
import { WebSearchPill } from "./web-search";
import { RestoreChat } from "./restore";
import { HarnessSwitchDialog } from "./switch-dialog";
import { UsageTag } from "./usage-tag";
import { PersonaAvatar, PersonaGreeting, PersonaName, PersonaStack, PersonaSwitcher, personaLook, type PersonaLook } from "./persona";
import { PersonaProposals } from "./persona-proposals";
import { ConnectorRequests } from "./connector-requests";
import { SecretRequests } from "./secret-requests";
import { SecretsChip } from "./secrets-chip";
import { ChatCostChip } from "@/components/domain/cost-chip";
import { unavailableReason } from "@/lib/setup";
import { ChatProjectPicker } from "./chat-project";
import { CompactChatHeader } from "./compact-chat-header";
import { useCompact } from "@/lib/viewport";
import { composerOptions } from "@/lib/composer-options";
import { CommitEmailChip, CommitEmailPicker } from "./commit-email";
import { needsCommitEmailPick } from "@/lib/commit-email";
import { onComposerDrop } from "@/lib/playground-report";
import { WorkspaceFolderChip } from "@/components/domain/workspace-folder";
import { SpaceBreadcrumb, SpaceContextChips } from "@/app/(app)/spaces/_components/space-chat";
import Link from "next/link";
import { hoverReveal } from "@/lib/reveal";

type Message = Schemas["ChatMessage"];
type Groups = Schemas["ChatHarnessModels"][] | undefined;

const errorText = (e: unknown) => (e instanceof Error ? e.message : "Something went wrong. Please try again.");

function Tag({ groups, m }: { groups: Groups; m: Message }) {
  if (!m.harness || !m.model) return null;
  const l = modelLabel(groups, { harness: m.harness, model: m.model });
  return (
    <UsageTag m={m}>
      {l.harness} · {l.model}
      {m.effort && ` · ${effortLabel(m.effort)}${m.effort_level ? " (auto)" : ""}`}
    </UsageTag>
  );
}

// Actions that stay out of the way until the message is hovered or focused.
const REVEAL = cx(hoverReveal("msg"), "transition-opacity focus-within:opacity-100");
const LONG_REPLY = 1500;

function Divider({ children, testId }: { children: string; testId?: string }) {
  return (
    <li className="flex items-center gap-3 py-1 text-xs text-fg-tertiary" data-testid={testId}>
      <span aria-hidden className="h-px flex-1 bg-border-subtle" />
      <span>{children}</span>
      <span aria-hidden className="h-px flex-1 bg-border-subtle" />
    </li>
  );
}

type ChatInfo = { id: string; title: string; project?: string; agent: { name: string } };

const Bubble = memo(function Bubble({ m, chat, persona, saveable, names, turn, summaryUrl, editable, onEdit, onRetry, highlighted, videos, asides }: { m: Message; chat: ChatInfo; persona: PersonaLook; saveable: boolean; names: Map<string, string>; turn?: TurnWindow; summaryUrl?: string | null; editable?: boolean; onEdit?: (body: string) => void; onRetry?: () => void; highlighted?: boolean; videos?: ChatVideo[]; asides?: ReactNode }) {
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState(m.body_md);
  const self = useRef<HTMLLIElement>(null);
  // The closing URL list shows as Sources cards, not as text. Saved replies keep it.
  const parts = useMemo(() => (m.role === "assistant" ? splitSourceList(m.body_md) : { body: m.body_md, listed: [] }), [m.role, m.body_md]);
  const [citeFocus, setCiteFocus] = useState<{ n: number; tick: number }>();
  const cites = useMemo(() => {
    const titles = new Map<number, string>();
    for (const l of parts.listed) if (l.n !== undefined) titles.set(l.n, l.title || l.ref || hostOf(l.url));
    return { numbers: [...titles.keys()], value: { titles, focus: (n: number) => setCiteFocus((f) => ({ n, tick: (f?.tick ?? 0) + 1 })) } };
  }, [parts.listed]);
  if (m.role === "system") {
    // A turn that failed keeps the videos it started, so what went wrong stays visible.
    return videos?.length ? (
      <>
        <li className="flex flex-col items-start"><VideoCards chatId={chat.id} videos={videos} messageId={m.id} /></li>
        <Divider testId="system-message">{m.body_md}</Divider>
      </>
    ) : <Divider testId="system-message">{m.body_md}</Divider>;
  }
  const mine = m.role === "user";
  const author = m.persona ? personaLook(m.persona) : persona;
  return (
    <li ref={self} id={`msg-${m.id}`} data-author={m.persona?.name} data-author-id={m.persona?.id} className={cx("group/msg flex scroll-mt-4 flex-col gap-1 rounded-xl transition-shadow", mine ? "items-end" : "items-start", highlighted && "ring-2 ring-accent ring-offset-4 ring-offset-bg")} data-enter-key={m.id} data-enter={m.role} data-testid={`${m.role}-message`}>
      {/* Your messages need no name: the bubble on the right is you. */}
      {mine ? null : (
        // Assistant replies carry the avatar of the persona that wrote them (the chat's host
        // when unset) and its name in its color, so a group chat reads like one.
        // The harness · model tag and usage live in the Activity tab, so the chat stays immersive.
        <span className="flex items-center gap-2 text-xs text-fg-tertiary" data-testid="assistant-identity">
          <PersonaAvatar persona={author} size={22} />
          <PersonaName persona={author} className="text-[13px]" />
          <time dateTime={m.created_at}>{timelineTime(m.created_at)}</time>
        </span>
      )}
      {!mine && videos?.length ? <VideoCards chatId={chat.id} videos={videos} messageId={m.id} /> : null}
      {/* Replies read as prose on the page; only real objects (canvases, runs, sources) get a card. */}
      <div
        className={cx(
          "type-chat max-w-[min(720px,100%)]",
          mine ? "rounded-2xl bg-selected px-4 py-2.5" : "w-full text-fg-2",
        )}
        data-testid={mine ? undefined : "reply-body"}
      >
        {editing ? (
          <form
            className="flex w-[min(560px,70vw)] flex-col gap-2"
            onSubmit={(e) => {
              e.preventDefault();
              if (draft.trim() && onEdit) {
                onEdit(draft);
                setEditing(false);
              }
            }}
          >
            <textarea aria-label="Edit message" autoFocus rows={Math.min(10, draft.split("\n").length + 1)} value={draft} onChange={(e) => setDraft(e.target.value)} onKeyDown={(e) => e.key === "Escape" && setEditing(false)} className="w-full resize-y rounded-md border border-border-strong bg-surface p-2 text-[13.5px] text-fg focus:border-accent focus:outline-none" />
            <div className="flex justify-end gap-1.5">
              <Button variant="ghost" onClick={() => setEditing(false)}>Cancel</Button>
              <Button type="submit" variant="primary" disabled={!draft.trim()}>Save and resend</Button>
            </div>
          </form>
        ) : (
          <CiteContext.Provider value={cites.value}>
            <ChatMarkdown replyId={mine ? undefined : m.id} cites={cites.numbers}>{parts.body}</ChatMarkdown>
          </CiteContext.Provider>
        )}
      </div>
      {!mine && videos?.length ? <VideoRendererNotes videos={videos} /> : null}
      {mine ? <SentAttachments attachments={m.attachments} /> : <ReplyAttachments attachments={m.attachments} />}
      {!mine && <CodeChanges changes={m.code_changes} />}
      {mine && <SentChips chips={(m.context_refs ?? []).map((r) => {
        const chip = chipFromRef(r, names);
        // A design's selection rides along with every message sent from the editor.
        return r.kind === "canvas" && r.selection ? { ...chip, label: r.selection.nodes.length ? `Design · ${r.selection.nodes.length} selected` : "Design" } : chip;
      })} />}
      {mine && editable && !editing && (
        <div className={REVEAL}>
          <Button variant="ghost" icon="edit" onClick={() => { setDraft(m.body_md); setEditing(true); }}>Edit</Button>
        </div>
      )}
      {!mine && <Sources citations={m.citations} body={parts.body} listed={parts.listed} runId={m.run_id} since={turn?.since} until={turn?.until} webEvents={!!turn} focus={citeFocus} />}
      {!mine && asides}
      {!mine && saveable && summaryUrl && <SaveSummary messageId={m.id} url={summaryUrl} />}
      {!mine && saveable && m.body_md.trim() && (
        <div className={cx("flex flex-wrap items-center gap-1.5", REVEAL)} data-testid="reply-actions">
          <CopyButton text={m.body_md} />
          {m.body_md.length > LONG_REPLY && <Button variant="ghost" onClick={() => self.current?.scrollIntoView({ block: "start", behavior: "smooth" })}>Top of reply</Button>}
          {onRetry && <Button variant="ghost" onClick={() => { if (window.confirm("Replace this reply with a new one?")) onRetry(); }}>Retry</Button>}
          <SaveActions chat={chat} body={m.body_md} />
        </div>
      )}
    </li>
  );
});

type ChatAsides = NonNullable<ReturnType<typeof useChatAsides>["data"]>;

/**
 * The transcript's messages. Typing in the composer re-renders the whole chat screen, so
 * everything handed to a Bubble here is kept referentially stable (memoized, or read through
 * a ref when it is a callback) and a keystroke only re-checks props instead of re-rendering
 * every message's markdown.
 */
function BubbleList({ list, chat, persona, speakerName, names, highlight, running, since, locked, archived, editPending, lastUser, videosByMessage, asideList, pinnedRef, editLast }: {
  list: Message[];
  chat: ChatInfo;
  persona: PersonaLook;
  speakerName: string;
  names: Map<string, string>;
  highlight: string | null;
  running: boolean;
  since?: string;
  locked: boolean;
  archived: boolean;
  editPending: boolean;
  lastUser?: Message;
  videosByMessage: Map<string, ChatVideo[]>;
  asideList?: ChatAsides;
  pinnedRef: RefObject<boolean>;
  editLast: (body: string) => void;
}) {
  const turns = useMemo(() => turnWindows(list), [list]);
  const latest = useRef({ editLast, retryBody: lastUser?.body_md });
  useEffect(() => {
    latest.current = { editLast, retryBody: lastUser?.body_md };
  });
  const onEdit = useCallback((body: string) => {
    pinnedRef.current = true;
    latest.current.editLast(body);
  }, [pinnedRef]);
  const onRetry = useCallback(() => {
    pinnedRef.current = true;
    if (latest.current.retryBody !== undefined) latest.current.editLast(latest.current.retryBody);
  }, [pinnedRef]);
  // The last reply's window stays open while the next turn runs: that turn's asides are live, below.
  const asideNodes = useMemo(() => {
    const out = new Map<string, ReactNode>();
    for (const [id, t] of turns) {
      const asked = asidesInWindow(asideList, t.since, t.until ?? (running ? since : undefined));
      if (asked.length) out.set(id, <ReplyAsides asides={asked} name={speakerName} />);
    }
    return out;
  }, [turns, asideList, running, since, speakerName]);
  const canEdit = !running && !locked && !editPending && !archived;
  const canRetry = !running && !editPending && !archived && !!lastUser;
  const lastId = list.at(-1)?.id;
  return list.map((m, i) => (
    <Bubble key={m.id} m={m} chat={chat} persona={persona} saveable={!running} names={names} turn={turns.get(m.id)} highlighted={highlight === m.id}
      editable={canEdit && m.id === lastUser?.id}
      onEdit={onEdit}
      onRetry={canRetry && m.id === lastId ? onRetry : undefined}
      summaryUrl={m.role === "assistant" && turns.has(m.id) ? summaryRequestUrl(list.slice(0, i).findLast((x) => x.role === "user")?.body_md) : null}
      videos={videosByMessage.get(m.id)}
      asides={asideNodes.get(m.id) ?? null} />
  ));
}

/**
 * What a card in a reply can do. The value only changes with the chat's data, not with the
 * draft, so the cards in every reply don't re-render per keystroke; the callbacks are read
 * through a ref for the same reason.
 */
function ReplyActions({ list, chat, running, disabled, agent, ask, researchMore, children }: {
  list: Message[];
  chat: { id: string; project?: string; archived: boolean };
  running: boolean;
  disabled: boolean;
  agent: string;
  ask: (messageId: string, body: string) => void;
  researchMore: (artifactId: string, title: string) => void;
  children: ReactNode;
}) {
  const callbacks = useRef({ ask, researchMore });
  useEffect(() => {
    callbacks.current = { ask, researchMore };
  });
  const canvas = useMemo(() => {
    const last = new Map<string, string>();
    let newest: string | undefined;
    for (const m of list) {
      if (m.role !== "assistant") continue;
      for (const canvasId of canvasFenceIds(m.body_md)) {
        last.set(canvasId, m.id);
        newest = canvasId;
      }
    }
    return { newest, lastMessage: (canvasId: string) => last.get(canvasId), live: running, readOnly: chat.archived, agent };
  }, [list, running, chat.archived, agent]);
  const value = useMemo(() => ({
    ask: (messageId: string, body: string) => callbacks.current.ask(messageId, body),
    researchMore: (artifactId: string, title: string) => callbacks.current.researchMore(artifactId, title),
    disabled,
    chatId: chat.id,
    project: chat.project,
    canvas,
    nextMessage: (id: string) => list.slice(list.findIndex((m) => m.id === id) + 1).find((m) => m.role === "user")?.body_md,
  }), [disabled, chat.id, chat.project, canvas, list]);
  return <RecommendActions.Provider value={value}>{children}</RecommendActions.Provider>;
}

const snippet = (s: string) => (s.length > 120 ? `${s.slice(0, 120).trimEnd()}…` : s);

/**
 * Everything behind the replies: per turn, the model and usage, tool calls, web searches
 * and memory changes (with Undo), plus the running turn's live activity. Kept out of the
 * chat tab so the conversation reads as if no tools ran.
 */
function ActivityLog({ list, turns, groups, running, runId, since, focus }: { list: Message[] | undefined; turns: Map<string, TurnWindow>; groups: Groups; running: boolean; runId?: string; since?: string; focus?: { key: string; at: number } }) {
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
          <RunActivity runId={runId} since={since} focus={focus} />
        </li>
      )}
    </ol>
  );
}

/** Activity's cost, model line and log, for the compact tab and the desktop dock's tab. */
function ActivityBody({ chat: c, list, groups, focus }: { chat: NonNullable<ReturnType<typeof useChat>["data"]>; list: Message[] | undefined; groups: Groups; focus?: { key: string; at: number } }) {
  const current: Selection = { harness: c.harness, model: c.model, auto: !!c.auto_model };
  const running = c.state === "running";
  const lastUser = list?.findLast((m) => m.role === "user");
  const since = runningSince(list) ?? lastUser?.created_at;
  return (
    <>
      <div className="mx-auto mb-4 flex max-w-[860px] flex-wrap items-center gap-2">
        <ChatCostChip chatId={c.id} project={c.project} />
        <span className="truncate font-mono text-xs text-fg-tertiary">
          {modelLabel(groups, current).harness} · {modelLabel(groups, current).model}
          {current.auto && " (auto)"}
          {c.effort && ` · ${effortLabel(c.effort)} effort`}
        </span>
      </div>
      <ActivityLog list={list} turns={turnWindows(list)} groups={groups} running={running} runId={c.current_run_id} since={since} focus={focus} />
    </>
  );
}

/** The desktop dock's Activity tab: beside the conversation, so a reply and its trace read side by side. */
export function ChatActivity({ chatId }: { chatId: string }) {
  const chat = useChat(chatId).data;
  const target = useSearchParams().get("message");
  // The same query the conversation reads, so it's shared rather than fetched twice.
  const list = useChatMessages(chatId, target ? 1000 : 200).data?.messages;
  const groups: Groups = useChatModels().data;
  const focus = useActivityFocus();
  if (!chat) return null;
  return (
    // Its own tooltip provider: the dock sits outside the conversation's.
    <Tooltip.Provider delayDuration={200}>
      <div data-testid="chat-activity" className="flex-1 overflow-y-auto px-4 py-4">
        <ActivityBody chat={chat} list={list} groups={groups} focus={focus} />
      </div>
    </Tooltip.Provider>
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

export function Conversation({ id, onNew, actions }: { id: string; onNew: () => void; actions?: ReactNode }) {
  const chat = useChat(id);
  const urlSearch = useSearchParams();
  const target = urlSearch.get("message");
  const messages = useChatMessages(id, target ? 1000 : 200);
  const edit = useEditLastChatMessage(id);
  const models = useChatModels();
  const send = useSendChatMessage(id);
  const queue = useQueueFollowup(id);
  const aside = useStartAside(id);
  const asideList = useChatAsides(id).data;
  // While a reply runs, how Send sends: queue a follow-up (the default), ask an aside, or interrupt.
  const [sendMode, setSendMode] = useState<SendMode>("followup");
  const [modeError, setModeError] = useState<string | null>(null);
  const [players] = useState(() => new Map<string, (seconds: number) => void>());
  const cancel = useCancelChat(id);
  const boot = useBootstrap();
  const personas = usePersonas();
  const create = useCreateChat();
  const selectRepo = useSetChatRepository(id);
  const [repoSignal, setRepoSignal] = useState(0);
  // The compact composer keeps the repository picker in its options sheet: the chat menu opens that.
  const [sheetSignal, setSheetSignal] = useState(0);
  const savedRepos = useRepositories();
  const discard = useDeleteChat();
  const qc = useQueryClient();
  // A persona switch hands the unsent draft over to the new chat.
  const [text, setText] = useState(() => readDraft(id));
  // Read once into state, then forget it (an effect, so a StrictMode re-render still sees it).
  useEffect(() => dropDraft(id), [id]);
  const [picked, setPicked] = useState<Selection | null>(null);
  // null keeps the chat's effort; "default" clears it.
  const [pickedEffort, setPickedEffort] = useState<Effort | "default" | null>(null);
  // Only the compact header switches the conversation for Activity; on desktop Activity is a dock tab beside it.
  const [tab, setTab] = useState<"chat" | "activity">("chat");
  const compact = useCompact();
  const view = compact ? tab : "chat";
  // "View reasoning" on a chat step: Activity opens on that step (the compact tab, or the dock's tab).
  const [traceFocus, setTraceFocus] = useState<{ key: string; at: number }>();
  const openStep = (key: string) => {
    if (!compact) {
      focusActivityStep(key);
      const q = setDock(new URLSearchParams(window.location.search), { tab: "activity", open: true });
      router.replace(`${window.location.pathname}?${q.toString()}`, { scroll: false });
      return;
    }
    setTab("activity");
    setTraceFocus({ key, at: Date.now() });
  };
  // A pick on another harness waits here until the person confirms what the switch loses.
  const [confirming, setConfirming] = useState<Selection | null>(null);
  const router = useRouter();
  const search = useSearchParams();
  // "Start chat" links arrive with ?attach=kind:id; seed once, then drop it from the URL.
  const [chips, setChips] = useState<Chip[]>(() => parseAttach(search.getAll("attach"), search.get("attach_title")));
  const files = useAttachments();
  const area = useRef<HTMLTextAreaElement>(null);
  const end = useRef<HTMLLIElement>(null);
  const scroller = useRef<HTMLDivElement>(null);
  const log = useRef<HTMLOListElement>(null);
  // Following the newest message; off once the person scrolls up, or arrives at a search hit.
  const pinned = useRef(!target);
  // Dragging the scrollbar or touch-scrolling: only then does a scroll away from the end mean the person moved.
  const dragging = useRef(false);
  const [newReply, setNewReply] = useState(false);
  // On for one message: asks for a research plan to approve.
  const [research, setResearch] = useState(false);
  const [highlight, setHighlight] = useState<string | null>(null);
  const workflows = useWorkflows();
  const [startingWorkflow, setStartingWorkflow] = useState<Workflow | null>(null);
  const setCommitEmail = useSetChatCommitEmail();
  const [commitPickerOpen, setCommitPickerOpen] = useState(false);

  const c = chat.data;
  const loaded = !!c;
  const running = c?.state === "running";
  // Videos sit on the reply their turn stored; the running turn's show live under it.
  const videos = useChatVideos(c?.id, running);
  const videosByMessage = useMemo(() => {
    const out = new Map<string, ChatVideo[]>();
    for (const v of videos.data?.videos ?? []) if (v.message_id) out.set(v.message_id, [...(out.get(v.message_id) ?? []), v]);
    return out;
  }, [videos.data]);
  const liveVideos = useMemo(() => (videos.data?.videos ?? []).filter((v) => !v.message_id && running && v.run_id === c?.current_run_id), [videos.data, running, c?.current_run_id]);
  const stopping = running && (cancel.isPending || cancel.isSuccess);
  // Also locked while a persona switch creates the replacement chat, so nothing is sent to the one it drops.
  // A running reply doesn't lock the composer: a message sent then interrupts it,
  // and the next reply answers both, picking up from what the stopped one had done.
  const locked = send.isPending || create.isPending;
  const groups: Groups = models.data;
  const current: Selection | undefined = c && { harness: c.harness, model: c.model, auto: !!c.auto_model };
  const selection = picked ?? current;
  const pending = !!(picked && current && !sameSelection(picked, current));
  const offered = selection ? modelEfforts(groups, selection) : { efforts: [] };
  // The effort the next message runs at; one the selected model lacks falls back to automatic, as on the server.
  const wanted = pickedEffort === null ? c?.effort : pickedEffort === "default" ? undefined : pickedEffort;
  const effort = wanted && offered.efforts.includes(wanted) ? wanted : undefined;
  const modelSummary = selection ? modelEffortSummary(groups, selection, offered.efforts.length > 0, effort).text : undefined;
  const effortPending = !!c && effort !== c.effort;
  // The next message would start a run on a harness that can't run: say why before sending.
  const unavailable = selection && groups?.find((g) => g.harness === selection.harness && !g.available);
  const nextRunSettings = {
    ...(pending && picked ? (picked.auto ? { model: "auto" } : { harness: picked.harness, model: picked.model }) : {}),
    ...(effortPending ? { effort: effort ?? ("default" as const) } : {}),
  };
  const list = messages.data?.messages;
  useEntrances(log);
  const lastUser = list?.findLast((m) => m.role === "user");
  // The running turn starts after the newest message, which may be a reply, not
  // the person's: a persona handing on, or the image follow-up.
  const since = runningSince(list) ?? lastUser?.created_at;
  const drop = useFileDrop(files.add, !!c && !c.archived && !locked);
  // From the playground: Send to chat's report and screenshot, or an ask to map its API.
  const addFiles = useRef(files.add);
  useEffect(() => {
    addFiles.current = files.add;
  });
  useEffect(() => {
    if (!c?.id || c.archived) return;
    return onComposerDrop(c.id, (drop) => {
      if (drop.files?.length) addFiles.current(drop.files);
      if (drop.text) setText((t) => (t.trim() ? `${t}\n\n${drop.text}` : drop.text!));
      area.current?.focus();
    });
  }, [c?.id, c?.archived]);

  useEffect(() => {
    if (!search.has("attach")) return;
    const q = new URLSearchParams(search.toString());
    q.delete("attach");
    q.delete("attach_title");
    router.replace(`/chat/${id}?${q.toString()}`);
  }, [search, router, id]);
  // Refs only carry ids; remember what the chips were called so sent messages can show it.
  const [names, setNames] = useState(() => new Map<string, string>());

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
    if (pinned.current) {
      end.current?.scrollIntoView?.({ block: "end" });
      return;
    }
    if (list?.at(-1)?.role !== "assistant") return;
    const show = setTimeout(() => setNewReply(true), 0);
    return () => clearTimeout(show);
  }, [list?.length, running]); // eslint-disable-line react-hooks/exhaustive-deps -- only a new message or turn moves the view
  // Replies keep growing after they land (a running turn's narration and steps, diagrams, apps,
  // images) and trays above the composer shrink the view; stay at the end while pinned there.
  const hasLog = !!list && (list.length > 0 || running);
  useEffect(() => {
    const view = scroller.current;
    const content = log.current;
    if (!view || !content) return;
    const follow = new ResizeObserver(() => {
      if (pinned.current) view.scrollTop = view.scrollHeight;
    });
    follow.observe(view);
    follow.observe(content);
    return () => follow.disconnect();
  }, [hasLog]);
  const targetFound = !!target && !!list?.some((m) => m.id === target);
  useEffect(() => {
    if (!target || !targetFound) return;
    pinned.current = false;
    document.getElementById(`msg-${target}`)?.scrollIntoView({ block: "center" });
    const on = setTimeout(() => setHighlight(target), 0);
    const off = setTimeout(() => setHighlight(null), 2400);
    return () => {
      clearTimeout(on);
      clearTimeout(off);
    };
  }, [target, targetFound]);
  // The composer starts at one line and grows with what's typed, up to a limit.
  useEffect(() => {
    const el = area.current;
    if (!el) return;
    el.style.height = "auto";
    el.style.height = `${Math.min(el.scrollHeight + 2, 220)}px`;
  }, [text, loaded]);
  const followLatest = () => {
    pinned.current = true;
    setNewReply(false);
    end.current?.scrollIntoView?.({ block: "end", behavior: "smooth" });
  };
  const researchMore = (artifactId: string, title: string) => {
    const chip = chipFromTarget({ kind: "artifact", id: artifactId, title, artifact_type: "report" });
    if (chip) setChips((cs) => addChip(cs, chip));
    setResearch(true);
    setText("Go deeper on this report: ");
    area.current?.focus();
  };
  // Sent from its own box, so the draft in the composer (and its chips and files) stays put.
  // A failed send moves the question to the composer rather than losing it.
  const askAboutSelection = (selected: string, question: string, authorId?: string, author?: string) => {
    if (locked || unavailable) return false;
    const body = quoteForComposer(selected, authorId && authorId !== (c?.speaker ?? c?.persona)?.id ? author : undefined) + question;
    cancel.reset();
    pinned.current = true;
    send.mutate({ body, ...nextRunSettings }, { onError: () => setText((t) => (t ? `${body}\n\n${t}` : body)) });
    return true;
  };

  function submit(body: string) {
    if (!body.trim() || locked || unavailable || files.pending || commitPickRequired) return;
    cancel.reset();
    pinned.current = true;
    const context_refs = toContextRefs(chips);
    setNames((n) => new Map([...n, ...chips.map((c): [string, string] => [c.key, c.label])]));
    send.mutate(
      {
        body,
        ...(research ? { research: true } : {}),
        ...nextRunSettings,
        ...(context_refs ? { context_refs } : {}), ...(files.ids.length ? { attachment_ids: files.ids } : {}) },
      {
        onSuccess: () => {
          files.clear();
          setText("");
          setPicked(null);
          setPickedEffort(null);
          setChips([]);
          setResearch(false);
        },
      },
    );
  }

  // Queues the message, or asks it as an aside, without stopping the reply; Interrupt
  // sends it as usual. Neither of the first two carries context, files or research.
  function sendWhileRunning(mode: SendMode, body: string): string | void {
    if (mode === "interrupt") return submit(body);
    if (!body.trim() || locked || queue.isPending || aside.isPending) return;
    if (chips.length || files.ids.length || files.pending || research) {
      const what = mode === "aside" ? "An aside" : "A follow-up";
      return `${what} is text only: remove the context and files, or pick Interrupt to send them now.`;
    }
    setModeError(null);
    pinned.current = true;
    if (mode === "aside") aside.mutate(body, { onSuccess: () => setText("") });
    else queue.mutate(body, { onSuccess: () => setText("") });
  }

  // A card's button or a choice: goes to the persona that wrote the reply. A mention
  // is only needed when someone else would answer next.
  function askAuthor(messageId: string, body: string) {
    pinned.current = true;
    const by = list?.find((m) => m.id === messageId)?.persona;
    send.mutate({ body: by && by.id !== (c?.speaker ?? c?.persona)?.id ? `@${by.name} ${body}` : body });
  }

  // Before the first message the persona can still change: a chat keeps its persona for life,
  // so switching starts a fresh chat with the other persona and drops this empty one.
  const fresh = !!c && !c.archived && c.message_count === 0 && !running && !list?.length;
  // With several addresses, a coding chat asks which one its commits use; the
  // first message waits for the answer, later ones don't.
  const myEmails = boot.data?.me.emails ?? [];
  const askCommitEmail = !!c && !c.archived && needsCommitEmailPick(myEmails, c.repo_url, c.commit_email);
  const commitPickRequired = fresh && askCommitEmail;
  function switchPersona(personaId: string) {
    if (!c || personaId === c.persona?.id || create.isPending || send.isPending || send.isSuccess) return;
    replaceChat(personaId, undefined, text, !!c.temporary);
  }
  // A chat is temporary for life, so before the first message switching it is a fresh chat too.
  function switchTemporary(temporary: boolean) {
    if (!c || temporary === !!c.temporary || create.isPending || send.isPending || send.isSuccess) return;
    replaceChat(c.persona?.id, c.repo_url, text, temporary);
  }
  // Moves to a new chat, carrying the draft over. The old one is only dropped if it is still empty.
  function replaceChat(personaId: string | undefined, repo: string | undefined, draft: string, temporary: boolean) {
    if (!c) return;
    const old = c;
    create.mutate(
      { ...(old.project ? { project: old.project } : {}), ...(personaId ? { persona_id: personaId } : {}), ...(temporary ? { temporary } : {}), harness: old.harness, model: old.auto_model ? "auto" : old.model, ...(old.effort ? { effort: old.effort } : {}), ...(repo ? { repo_url: repo } : {}) },
      {
        onSuccess: async (next) => {
          if (draft.trim()) putDraft(next.id, draft);
          router[old.message_count === 0 ? "replace" : "push"](`/chat/${next.id}`);
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
  const actionError = send.error ?? queue.error ?? aside.error ?? edit.error ?? cancel.error ?? create.error ?? setCommitEmail.error;
  // A chat without a persona, or whose persona was deleted, is the built-in Assistant.
  const persona = personaLook(c.persona);
  const full = personas.data?.personas.find((p) => p.id === c.persona?.id);
  const participants = (c.participants ?? (c.persona ? [c.persona] : [])).map(personaLook);
  const group = participants.length > 1;
  // Who answers next (or is answering now): the last speaker, else the host.
  const speaker = personaLook(c.speaker ?? c.persona);
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

  // ---- / commands: what each does; the Composer parses and runs them ----
  const startInstead = (personaId: string, repo?: string): string | void => {
    if (create.isPending || send.isPending) return "Wait for the current message to finish sending.";
    replaceChat(personaId, repo, "", false);
  };
  const whileIdle = (what: string, run: () => void): string | void => (running ? `Wait for the reply to finish before changing the ${what}.` : run());
  const plainCommands: Record<string, CommandRun> = {
    new: onNew,
    memory: () => router.push("/customization"),
    stop: () => {
      if (!running) return "There’s no reply to stop.";
      if (!stopping) cancel.mutate();
    },
  };
  const published = (workflows.data?.workflows ?? []).filter((w) => w.published_version > 0);
  // /workflow on the welcome makes the chat first, then opens it with ?workflow= to show the start card.
  const workflowCard = startingWorkflow ?? published.find((w) => w.id === search.get("workflow")) ?? null;
  const closeWorkflow = () => {
    setStartingWorkflow(null);
    if (!search.has("workflow")) return;
    const q = new URLSearchParams(search.toString());
    q.delete("workflow");
    const qs = q.toString();
    router.replace(qs ? `/chat/${id}?${qs}` : `/chat/${id}`);
  };
  // The + menu's "Create with {persona}": Doc and Design start a draft for the person to finish; decks start in Slides.
  const createWith = (kind: CreateKind) => {
    if (kind === "slides") return router.push("/apps/slides");
    setText((t) => (kind === "design" ? `/design ${t.trim()}` : `${DOC_DRAFT}${t.trim()}`));
    requestAnimationFrame(() => {
      const el = area.current;
      el?.focus();
      el?.setSelectionRange(el.value.length, el.value.length);
    });
  };
  const commandOptions = (name: string): { name: string; run: CommandRun }[] => {
    switch (name) {
      case "workflow":
        return published.map((w) => ({ name: w.name, run: () => setStartingWorkflow(w) }));
      case "model":
        return [
          ...(current ? [{ name: "Auto", run: () => whileIdle("model", () => pick({ ...current, auto: true })) }] : []),
          ...(groups ?? [])
            .filter((g) => g.available)
            .flatMap((g) => g.models.map((m) => ({ name: `${g.label} ${m.label}`, run: () => whileIdle("model", () => pick({ harness: g.harness, model: m.id })) }))),
        ];
      case "effort":
        return [undefined, ...offered.efforts].map((e) => ({ name: effortLabel(e).toLowerCase(), run: () => whileIdle("effort", () => setPickedEffort(e ?? "default")) }));
      case "persona":
        return (personas.data?.personas ?? []).map((p) => ({ name: p.name, run: () => (p.id === c.persona?.id ? `You’re already talking to ${p.name}.` : startInstead(p.id)) }));
      case "repo":
        return (c.speaker ?? c.persona)?.capabilities?.code
          ? (savedRepos.data ?? []).map((r) => ({ name: r.name, run: () => selectRepo.mutate(r.url) }))
          : [];
      default:
        return [];
    }
  };
  const composerList = composerOptions({
    code: !!(c.speaker ?? c.persona)?.capabilities?.code,
    repo: c.repo_url,
    pendingRepo: c.pending_repo_url,
    persona: fresh ? (c.persona?.name ?? speaker.name) : undefined,
    sendMode: running ? SEND_MODE_LABEL[sendMode] : undefined,
    research,
    temporary: fresh ? !!c.temporary : undefined,
    model: modelSummary,
    project: c.space ? undefined : { name: boot.data?.projects.find((p) => p.key === c.project)?.name },
  });
  return (
    <Tooltip.Provider delayDuration={200}>
      <section aria-label={chatTitle(c)} className="relative flex min-h-0 flex-1 flex-col" {...drop.props}>
        <DropOverlay over={drop.over} />
        {/* Sized by its own width: a panel beside the chat can leave it narrow on a wide screen. */}
        {/* The page's only header: who you're talking to, the title, then the chat's context. */}
        {compact ? (
          <CompactChatHeader
            chat={c}
            personas={group ? participants : [persona]}
            tab={tab}
            onTab={setTab}
            running={running}
            canCode={!!(c.speaker ?? c.persona)?.capabilities?.code}
            onRepository={() => { setTab("chat"); setSheetSignal((n) => n + 1); }}
            onRestored={restored}
          />
        ) : (
        <div className="@container flex min-h-[52px] items-center gap-2.5 border-b border-border-subtle px-6 py-2">
          {group ? <PersonaStack personas={participants} size={22} /> : <span title={persona.name} className="flex-none"><PersonaAvatar persona={persona} size={26} /><span className="sr-only">Persona: {persona.name}</span></span>}
          {c.space && <SpaceBreadcrumb space={c.space} />}
          <h1 className="min-w-0 truncate text-[14.5px] font-semibold">{chatTitle(c)}</h1>
          {group && (
            <span
              className="flex min-w-0 flex-none items-center gap-1.5 rounded-full border border-border bg-surface py-0.5 pl-1 pr-2.5 text-xs max-sm:hidden @max-[460px]:hidden"
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
          )}
          {c.workspace_path && <WorkspaceFolderChip path={c.workspace_path} className="max-md:hidden @max-[720px]:hidden" />}
          <SecretsChip chatId={c.id} />
          {c.repo_url && c.commit_email && myEmails.length > 1 && (
            <CommitEmailChip email={c.commit_email} open={commitPickerOpen} onClick={() => setCommitPickerOpen((o) => !o)} />
          )}
          {/* In a space the project sits with the space's context chips below. */}
          {!c.space && <ChatProjectPicker chat={c} disabled={running} />}
          {c.archived && <span className="flex-none rounded-full bg-selected px-2 py-0.5 text-xs text-fg-secondary">Archived</span>}
          {c.temporary && <TemporaryBadge />}
          <span className="flex-1" />
          <ChatRunsPill chatId={c.id} />
          {c.archived && <RestoreChat chat={c} onRestored={restored} />}
          {actions}
        </div>
        )}
        {c.space && view === "chat" && <SpaceContextChips space={c.space} chat={c} disabled={running} />}
        {/* Mounted while hidden so memory notices still refresh. On desktop Activity is the dock's tab (ChatActivity). */}
        {compact && (
          <div role="tabpanel" aria-label="Activity" hidden={tab !== "activity"} className="flex-1 overflow-y-auto px-6 py-5">
            <ActivityBody chat={c} list={list} groups={groups} focus={traceFocus} />
          </div>
        )}
        <div
          ref={scroller}
          role="tabpanel"
          aria-label="Chat"
          hidden={view !== "chat"}
          className="relative flex-1 overflow-y-auto px-6 py-5"
          // Only the person's input lets go of the end. A scroll event alone can't tell: the one for our
          // own scroll, or for the browser clamping it as content shrinks, may land after the content grew
          // again, far from the end without anyone having moved.
          onWheel={(e) => {
            if (e.deltaY < 0) pinned.current = false;
          }}
          onKeyDown={(e) => {
            if (e.key === "ArrowUp" || e.key === "PageUp" || e.key === "Home") pinned.current = false;
          }}
          onPointerDown={(e) => {
            // On the panel itself rather than a message: that's the scrollbar.
            if (e.target === e.currentTarget) dragging.current = true;
          }}
          onPointerUp={() => (dragging.current = false)}
          onTouchStart={() => (dragging.current = true)}
          onTouchEnd={() => (dragging.current = false)}
          onScroll={(e) => {
            const el = e.currentTarget;
            const atEnd = el.scrollHeight - el.scrollTop - el.clientHeight < 120;
            if (atEnd) {
              pinned.current = true;
              setNewReply(false);
            } else if (dragging.current) pinned.current = false;
          }}
        >
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
              <>
              <PersonaGreeting
                persona={persona}
                tone={full?.tone}
                isDefault={!!c.persona && c.persona.id === personas.data?.default_persona_id}
                firstName={boot.data?.me.name?.split(" ")[0]}
                canSwitch={!!personas.data?.personas.length}
              />
              {/* Picking one sends it, which would drop a half-written message. */}
              {!text.trim() && <ChatSuggestions onPick={submit} disabled={locked || !!unavailable} />}
              </>
            )
          ) : (
            <>
            <VideoPlayers.Provider value={players}>
            <ReplyActions list={list} chat={c} running={running} disabled={running || locked || !!unavailable || c.archived} agent={(c.speaker ?? c.persona)?.name ?? "Assistant"} ask={askAuthor} researchMore={researchMore}>
            <ol ref={log} aria-label="Conversation" role="log" aria-live="polite" className="mx-auto flex max-w-[768px] flex-col gap-6">
              {messages.data?.has_more && <Divider>Earlier messages are not shown</Divider>}
              <BubbleList list={list} chat={c} persona={persona} speakerName={speaker.name} names={names} highlight={highlight} running={running} since={since}
                locked={locked} archived={c.archived} editPending={edit.isPending} lastUser={lastUser} videosByMessage={videosByMessage} asideList={asideList}
                pinnedRef={pinned} editLast={edit.mutate} />
              <PersonaProposals chatId={c.id} disabled={c.archived} />
              <ConnectorRequests chatId={c.id} disabled={c.archived} />
              <SecretRequests chatId={c.id} disabled={c.archived} />
              {pending && selection && (
                <Divider testId="pending-switch">
                  {selection.auto
                    ? "Next message will pick its model automatically"
                    : `Next message will use ${modelLabel(groups, selection).harness} · ${modelLabel(groups, selection).model}`}
                </Divider>
              )}
              {effortPending && (
                <Divider testId="pending-effort">{`Next message will use ${effortLabel(effort).toLowerCase()} effort`}</Divider>
              )}
              {running && (
                <li className="flex flex-col gap-2" data-testid="thinking">
                  <InterimReply runId={c.current_run_id} since={since} persona={speaker} />
                  <VideoCards chatId={c.id} videos={liveVideos} onStop={stopping ? undefined : () => cancel.mutate()} />
                  <ChatStatus chatId={c.id} disabled={c.archived} runId={c.current_run_id} since={since} persona={speaker} stopping={stopping} onOpenStep={openStep} />
                  <LiveAsides
                    asides={asidesInWindow(asideList, since)}
                    actions={{
                      onQueue: (question) => queue.mutate(question),
                      onSteer: (question) => {
                        pinned.current = true;
                        send.mutate({ body: question });
                      },
                      disabled: locked || queue.isPending,
                    }}
                  />
                </li>
              )}
              <li ref={end} aria-hidden />
            </ol>
            </ReplyActions>
            </VideoPlayers.Provider>
            {!c.archived && <AskAboutSelection root={log} onAsk={askAboutSelection} />}
            {newReply && (
              <div className="sticky bottom-1 z-20 flex justify-center">
                <button type="button" onClick={followLatest} className="flex items-center gap-1.5 rounded-full border border-border-float bg-surface-raised px-3 py-1 text-xs font-medium text-fg shadow-[var(--shadow-float)] hover:bg-hover">
                  <Icon name="chev" size={12} />
                  New reply
                </button>
              </div>
            )}
            </>
          )}
        </div>
        <div className="mx-auto flex w-full max-w-[816px] flex-col gap-2 px-6 pb-3">
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
              title={unavailable.checking ? `Checking ${unavailable.label} models…` : `${unavailable.label} is unavailable`}
              actions={
                !unavailable.checking && <Link href="/settings#secrets" className="text-[12.5px] text-accent hover:underline">
                  Add the key →
                </Link>
              }
            >
              {unavailable.checking ? "This updates automatically when the model list is ready." : `${unavailableReason(unavailable)}. Pick another model or add the key.`}
            </Banner>
          )}
          {modeError && (
            <p role="alert" data-testid="send-mode-error" className="text-[12.5px] text-pill-blocked-fg">
              {modeError}
            </p>
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
            <>
            {c.repo_url && (askCommitEmail || commitPickerOpen) && (
              <div className="mb-2">
                <CommitEmailPicker
                  emails={myEmails}
                  name={boot.data?.me.name ?? ""}
                  repo={c.repo_url}
                  value={askCommitEmail ? undefined : c.commit_email}
                  required={commitPickRequired}
                  disabled={setCommitEmail.isPending}
                  onPick={(email) => {
                    setCommitEmail.reset();
                    setCommitEmail.mutate({ id: c.id, email }, { onSuccess: () => setCommitPickerOpen(false) });
                  }}
                  onClose={() => setCommitPickerOpen(false)}
                />
              </div>
            )}
            <ChatWorkflowTray chatId={c.id} />
            <FollowupTray
              chatId={c.id}
              name={speaker.name}
              running={running}
              held={!running && list?.at(-1)?.role !== "assistant"}
              disabled={locked}
              onSendNow={(body, done) => {
                pinned.current = true;
                send.mutate({ body }, { onSuccess: done });
              }}
              onEdit={(body) => {
                setText((t) => (t.trim() ? `${body}\n\n${t}` : body));
                setSendMode("followup");
                area.current?.focus();
              }}
            />
            <ChatTicketRunTray chatId={c.id} scene={(kind) => <ActivityScene current={kind} />} persona={<PersonaAvatar persona={persona} size={26} />} />
            {workflowCard && (
              <div className="mb-2">
                <WorkflowStartCard workflow={workflowCard} chatId={c.id} onDone={closeWorkflow} onDismiss={closeWorkflow} />
              </div>
            )}
            <Composer
              sheetSignal={sheetSignal}
              text={text}
              onText={setText}
              inputRef={area}
              files={files}
              chips={chips}
              onChips={setChips}
              onSubmit={(body) => {
                const problem = running ? sendWhileRunning(sendMode, body) : submit(body);
                setModeError(problem || null);
              }}
              onCycle={running ? (back) => setSendMode((m) => cycleSendMode(m, back)) : undefined}
              hints={
                running
                  ? `Enter ${SEND_MODE_ENTER[sendMode]}  ·  Tab switches mode  ·  /aside  ·  /queue  ·  Shift+Enter for a new line`
                  : undefined
              }
              commands={{ plain: plainCommands, options: commandOptions,
              free: {
                ...Object.fromEntries(
                  (["aside", "queue"] as const).map((name) => [
                    name,
                    (body: string) => {
                      const mode = commandSendMode(name)!;
                      if (running) return sendWhileRunning(mode, body);
                      if (mode === "aside") return `Nothing is running, so there's nothing to ask about on the side: send it as a message.`;
                      submit(body);
                    },
                  ]),
                ),
                // The persona starts it with design_create, which opens it beside the chat.
                design: (brief: string) => (running ? sendWhileRunning("followup", designRequest(brief)) : submit(designRequest(brief))),
              },
              repository: (arg) => { if (!(c.speaker ?? c.persona)?.capabilities?.code) return "This persona cannot write code."; if (arg) selectRepo.mutate(arg); else setRepoSignal((n) => n + 1); }, hasWorkflows: published.length > 0 }}
              personas={personas.data?.personas ?? []}
              inChat={(personaId) => participants.some((x) => x.id === personaId)}
              create={c.archived ? undefined : { persona: speaker.name, onCreate: createWith }}
              disabled={locked}
              placeholder={
                locked
                  ? "Sending…"
                  : running
                    ? sendModePlaceholder(sendMode, c.speaker || c.persona ? speaker.name : "your assistant")
                    : `Message ${c.speaker || c.persona ? speaker.name : "your assistant"}${c.space ? ` in ${c.space.name}` : ""}…`
              }
              options={composerList}
              controls={{
                repository: (
                  <RepositoryPicker
                    value={c.repo_url}
                    pending={c.pending_repo_url}
                    onPick={(r) => selectRepo.mutate(r)}
                    disabled={c.archived || selectRepo.isPending}
                    error={selectRepo.error}
                    signal={repoSignal}
                    personaName={speaker.name}
                    personaRepos={personas.data?.personas.find((p) => p.id === (c.speaker ?? c.persona)?.id)?.repos}
                  />
                ),
                persona: <PersonaSwitcher value={c.persona?.id} onChange={switchPersona} disabled={locked || create.isPending} />,
                "send-mode": (
                  <SendModeSwitch
                    value={sendMode}
                    onChange={(m) => {
                      setSendMode(m);
                      setModeError(null);
                      area.current?.focus();
                    }}
                    disabled={locked}
                  />
                ),
                research: <ResearchToggle on={research} onChange={setResearch} disabled={locked} />,
                temporary: <TemporaryToggle on={!!c.temporary} onChange={switchTemporary} disabled={locked || create.isPending} />,
                model: selection && (
                  <ModelEffortPicker
                    groups={groups}
                    value={selection}
                    onChange={pick}
                    efforts={offered.efforts}
                    effort={effort}
                    onEffort={(e) => setPickedEffort(e ?? "default")}
                    disabled={locked || running}
                  />
                ),
                // The desktop composer leaves this to the header; the compact + sheet lists it too.
                project: !c.space && <ChatProjectPicker chat={c} disabled={running} />,
              }}
              actions={
                running && (
                  <Button variant="outline" icon="pause" disabled={stopping} onClick={() => cancel.mutate()}>
                    {stopping ? "Stopping…" : "Stop"}
                  </Button>
                )
              }
              send={{
                label: running ? sendModeAction(sendMode) : research ? "Plan it" : "Send",
                disabled: locked || !!unavailable || files.pending || !text.trim() || commitPickRequired || queue.isPending || aside.isPending,
                title: commitPickRequired ? "Pick which email commits use first" : running ? SEND_MODE_HINT[sendMode] : undefined,
                tone: running ? SEND_TONE[sendMode] : "accent",
              }}
            />
            </>
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

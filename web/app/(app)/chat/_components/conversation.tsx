"use client";

import { canvasFenceIds } from "@/lib/chat-canvas";
import { useRouter, useSearchParams } from "next/navigation";
import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { Tooltip } from "radix-ui";
import { useEntrances } from "@/components/ui/motion";
import { api, ApiError, unwrap, type Schemas } from "@/lib/api/client";
import {
  chatKeys,
  useBootstrap,
  useCancelChat,
  useChat,
  useChatMessages,
  useEditLastChatMessage,
  useChatModels,
  useCreateChat,
  useDeleteChat,
  useSendChatMessage,
  useSetChatCommitEmail,
} from "@/lib/api/hooks";
import { usePersonas } from "@/lib/api/persona-hooks";
import { useWorkflows, type Workflow } from "@/lib/api/workflow-hooks";
import { ChatWorkflowTray, WorkflowStartCard } from "@/components/domain/workflow-cards";
import { ChatTicketRunTray } from "@/components/domain/dispatch-cards";
import { ActivityScene } from "./activity-scene";
import { Tabs } from "@/components/ui/controls";
import { addChip, chipFromRef, chipFromTarget, parseAttach, removeChip, toContextRefs, type Chip } from "@/lib/chat-context";
import { chatTitle, needsSwitchConfirm, runningSince, turnWindows, type TurnWindow } from "@/lib/chat-ui";
import { CHAT_COMMANDS, commandMenu, literalBody, parseCommand } from "@/lib/chat-commands";
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
import { splitSourceList } from "@/lib/chat-sources";
import { AskAboutSelection, CopyButton, quoteForComposer, ResearchToggle } from "./reply-tools";
import { ChatSuggestions } from "./suggestions";
import { RecommendActions, SaveSummary, VideoPlayers } from "@/components/domain/recommend-fence";
import { VideoCards, VideoRendererNotes } from "@/components/domain/video-card";
import { useChatVideos } from "@/lib/api/video-hooks";
import type { ChatVideo } from "@/lib/chat-video";
import { CiteContext } from "@/components/domain/cite-marker";
import { summaryRequestUrl } from "@/lib/chat-recommend";
import { hostOf } from "@/lib/web-search";
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
import { ConnectorRequests } from "./connector-requests";
import { SecretRequests } from "./secret-requests";
import { SecretsChip } from "./secrets-chip";
import { HumanAvatar } from "@/components/domain/avatars";
import { ChatCostChip } from "@/components/domain/cost-chip";
import { unavailableReason } from "@/lib/setup";
import { ChatProjectPicker } from "./chat-project";
import { CommitEmailChip, CommitEmailPicker } from "./commit-email";
import { needsCommitEmailPick } from "@/lib/commit-email";
import { WorkspaceFolderChip } from "@/components/domain/workspace-folder";
import { SpaceBreadcrumb, SpaceContextChips } from "@/app/(app)/spaces/_components/space-chat";
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

// Actions that stay out of the way until the message is hovered or focused.
const REVEAL = "opacity-0 transition-opacity group-hover/msg:opacity-100 focus-within:opacity-100 [@media(hover:none)]:opacity-100";
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

function Bubble({ m, chat, persona, saveable, names, turn, summaryUrl, editable, onEdit, onRetry, highlighted, videos }: { m: Message; chat: ChatInfo; persona: PersonaLook; saveable: boolean; names: Map<string, string>; turn?: TurnWindow; summaryUrl?: string | null; editable?: boolean; onEdit?: (body: string) => void; onRetry?: () => void; highlighted?: boolean; videos?: ChatVideo[] }) {
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
      {!mine && videos?.length ? <VideoCards chatId={chat.id} videos={videos} messageId={m.id} /> : null}
      <div
        className={cx(
          "max-w-[min(720px,100%)] rounded-xl border px-3.5 py-2.5 text-[13.5px]",
          // Replies share one column edge with what sits under them.
          mine ? "border-border-strong bg-selected" : "w-full border-border bg-surface",
        )}
        style={mine || !m.persona ? undefined : { borderLeftColor: author.color, borderLeftWidth: 3 }}
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
      {mine && <SentChips chips={(m.context_refs ?? []).map((r) => chipFromRef(r, names))} />}
      {mine && editable && !editing && (
        <div className={REVEAL}>
          <Button variant="ghost" icon="edit" onClick={() => { setDraft(m.body_md); setEditing(true); }}>Edit</Button>
        </div>
      )}
      {!mine && <Sources citations={m.citations} body={parts.body} listed={parts.listed} runId={m.run_id} since={turn?.since} until={turn?.until} webEvents={!!turn} focus={citeFocus} />}
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
  const [players] = useState(() => new Map<string, (seconds: number) => void>());
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
  // "View reasoning" on a chat step: the Activity tab opens on that step.
  const [traceFocus, setTraceFocus] = useState<{ key: string; at: number }>();
  const openStep = (key: string) => {
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
  const [newReply, setNewReply] = useState(false);
  // On for one message: asks for a research plan to approve.
  const [research, setResearch] = useState(false);
  const [highlight, setHighlight] = useState<string | null>(null);
  // The @mention being typed in the composer, and which suggestion is highlighted.
  const [mention, setMention] = useState<{ start: number; query: string } | null>(null);
  const [mentionActive, setMentionActive] = useState(0);
  const workflows = useWorkflows();
  const [startingWorkflow, setStartingWorkflow] = useState<Workflow | null>(null);
  // The / command menu: which row is highlighted, whether Esc closed it, and why the last command didn't run.
  const [commandActive, setCommandActive] = useState(0);
  const [commandDismissed, setCommandDismissed] = useState(false);
  const [commandError, setCommandError] = useState<string | null>(null);
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
  const current: Selection | undefined = c && { harness: c.harness, model: c.model };
  const selection = picked ?? current;
  const pending = !!(picked && current && !sameSelection(picked, current));
  const offered = selection ? modelEfforts(groups, selection) : { efforts: [] };
  // The effort the next message runs at; one the selected model lacks falls back to automatic, as on the server.
  const wanted = pickedEffort === null ? c?.effort : pickedEffort === "default" ? undefined : pickedEffort;
  const effort = wanted && offered.efforts.includes(wanted) ? wanted : undefined;
  const effortPending = !!c && effort !== c.effort;
  // The next message would start a run on a harness that can't run: say why before sending.
  const unavailable = selection && groups?.find((g) => g.harness === selection.harness && !g.available);
  const nextRunSettings = {
    ...(pending && picked ? { harness: picked.harness, model: picked.model } : {}),
    ...(effortPending ? { effort: effort ?? ("default" as const) } : {}),
  };
  const list = messages.data?.messages;
  useEntrances(log);
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
    if (pinned.current) {
      end.current?.scrollIntoView?.({ block: "end" });
      return;
    }
    if (list?.at(-1)?.role !== "assistant") return;
    const show = setTimeout(() => setNewReply(true), 0);
    return () => clearTimeout(show);
  }, [list?.length, running]); // eslint-disable-line react-hooks/exhaustive-deps -- only a new message or turn moves the view
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
  function switchPersona(personaId: string, repo?: string) {
    if (!c || (personaId === c.persona?.id && !repo) || create.isPending || send.isPending || send.isSuccess) return;
    replaceChat(personaId, repo, text);
  }
  // Moves to a new chat, carrying the draft over. The old one is only dropped if it is still empty.
  function replaceChat(personaId: string, repo: string | undefined, draft: string) {
    if (!c) return;
    const old = c;
    create.mutate(
      { ...(old.project ? { project: old.project } : {}), persona_id: personaId, harness: old.harness, model: old.model, ...(old.effort ? { effort: old.effort } : {}), ...(repo ? { repo_url: repo } : {}) },
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
  const actionError = send.error ?? edit.error ?? cancel.error ?? create.error ?? setCommitEmail.error;
  // A chat without a persona, or whose persona was deleted, is the built-in Assistant.
  const persona = personaLook(c.persona);
  const full = personas.data?.personas.find((p) => p.id === c.persona?.id);
  const participants = (c.participants ?? (c.persona ? [c.persona] : [])).map(personaLook);
  const group = participants.length > 1;
  // Who answers next (or is answering now): the last speaker, else the host.
  const speaker = personaLook(c.speaker ?? c.persona);
  const canvasCards = (() => {
    const last = new Map<string, string>();
    let newest: string | undefined;
    for (const m of list ?? []) {
      if (m.role !== "assistant") continue;
      for (const canvasId of canvasFenceIds(m.body_md)) {
        last.set(canvasId, m.id);
        newest = canvasId;
      }
    }
    return { newest, lastMessage: (canvasId: string) => last.get(canvasId), live: running, readOnly: c.archived, agent: (c.speaker ?? c.persona)?.name ?? "Assistant" };
  })();
  const menu = commandDismissed ? null : commandMenu(text);
  const suggestions = mention && !menu ? mentionSuggestions(personas.data?.personas ?? [], mention.query) : [];
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

  // ---- / commands: what each does, and the composer's handling of them ----
  type CommandRun = () => string | void; // a returned string is why it couldn't run
  const startInstead = (personaId: string, repo?: string): string | void => {
    if (create.isPending || send.isPending) return "Wait for the current message to finish sending.";
    replaceChat(personaId, repo, "");
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
  const commandOptions = (name: string): { name: string; run: CommandRun }[] => {
    switch (name) {
      case "workflow":
        return published.map((w) => ({ name: w.name, run: () => setStartingWorkflow(w) }));
      case "model":
        return (groups ?? [])
          .filter((g) => g.available)
          .flatMap((g) => g.models.map((m) => ({ name: `${g.label} ${m.label}`, run: () => whileIdle("model", () => pick({ harness: g.harness, model: m.id })) })));
      case "effort":
        return [undefined, ...offered.efforts].map((e) => ({ name: effortLabel(e).toLowerCase(), run: () => whileIdle("effort", () => setPickedEffort(e ?? "default")) }));
      case "persona":
        return (personas.data?.personas ?? []).map((p) => ({ name: p.name, run: () => (p.id === c.persona?.id ? `You’re already talking to ${p.name}.` : startInstead(p.id)) }));
      case "repo":
        return codesInRepos(full) && c.persona
          ? full!.repos.map((r) => ({ name: repoShortName(r), run: () => (r === c.repo_url ? "This chat is already in that repository." : startInstead(c.persona!.id, r)) }))
          : [];
      default:
        return [];
    }
  };
  const runCommand = (run: CommandRun) => {
    const problem = run();
    setCommandError(problem || null);
    if (!problem) setText("");
  };
  const commandItems: { name: string; label: string; hint?: string; choose: () => void }[] = !menu
    ? []
    : menu.stage === "arg"
      ? mentionSuggestions(commandOptions(menu.name), menu.query, 8).map((o) => ({ name: o.name, label: o.name, choose: () => runCommand(o.run) }))
      : mentionSuggestions(CHAT_COMMANDS, menu.query, CHAT_COMMANDS.length).map((cmd) => ({
          name: cmd.name,
          label: `/${cmd.name}`,
          hint: cmd.hint,
          choose: () => {
            setCommandActive(0);
            if (cmd.args) setText(`/${cmd.name} `);
            else runCommand(plainCommands[cmd.name]);
          },
        }));
  // A finished command, typed in full or with the menu closed.
  const runTyped = ({ name, arg }: { name: string; arg: string }) => {
    const command = CHAT_COMMANDS.find((x) => x.name === name);
    if (!command) return setCommandError(`Unknown command /${name}. Start with // to send it as a message.`);
    if (!command.args) return arg ? setCommandError(`/${name} doesn’t take anything after it.`) : runCommand(plainCommands[name]);
    if (name === "workflow" && published.length === 0) return setCommandError("You have no published workflows yet. Build one under Workflows.");
    if (!arg) return setCommandError(`Say which ${command.args}: /${name} <${command.args}>`);
    const match = mentionSuggestions(commandOptions(name), arg)[0];
    if (!match) return setCommandError(`No ${command.args} matches “${arg}”.`);
    runCommand(match.run);
  };
  const submitComposer = () => {
    const command = parseCommand(text);
    if (command) runTyped(command);
    else submit(literalBody(text));
  };
  return (
    <Tooltip.Provider delayDuration={200}>
      <section aria-label={chatTitle(c)} className="relative flex min-h-0 flex-1 flex-col" {...drop.props}>
        <DropOverlay over={drop.over} />
        {/* Sized by its own width: a panel beside the chat can leave it narrow on a wide screen. */}
        <div className="@container flex items-center gap-2 border-b border-border-subtle px-6 py-2.5">
          {c.space && <SpaceBreadcrumb space={c.space} />}
          <h1 className="type-heading min-w-0 truncate">{chatTitle(c)}</h1>
          {group ? (
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
          ) : (
            <span className="flex flex-none items-center gap-1.5 rounded-full border border-border bg-surface py-0.5 pl-1 pr-2.5 text-xs max-sm:hidden @max-[460px]:hidden">
              <span className="sr-only">Persona: </span>
              <PersonaAvatar persona={persona} size={16} />
              <PersonaName persona={persona} />
            </span>
          )}
          {c.repo_url && (
            <span
              data-testid="repo-chip"
              title={[c.repo_url, ...(c.work_branches ?? [])].join("\n")}
              className="flex min-w-0 flex-none items-center gap-1.5 rounded-full border border-border bg-surface px-2.5 py-0.5 text-xs text-fg-secondary max-md:hidden @max-[720px]:hidden"
            >
              <Icon name="branch" size={13} className="flex-none" />
              <span className="truncate text-fg">{repoShortName(c.repo_url)}</span>
              {!!c.work_branches?.length && (
                <span className="truncate font-mono text-[11.5px]">
                  {c.work_branches[c.work_branches.length - 1]}
                  {c.work_branches.length > 1 && ` +${c.work_branches.length - 1}`}
                </span>
              )}
            </span>
          )}
          {c.workspace_path && <WorkspaceFolderChip path={c.workspace_path} className="max-md:hidden @max-[720px]:hidden" />}
          <SecretsChip chatId={c.id} />
          {c.repo_url && c.commit_email && myEmails.length > 1 && (
            <CommitEmailChip email={c.commit_email} open={commitPickerOpen} onClick={() => setCommitPickerOpen((o) => !o)} />
          )}
          {/* In a space the project sits with the space's context chips below. */}
          {!c.space && <ChatProjectPicker chat={c} disabled={running} />}
          {c.archived && <span className="flex-none rounded-full bg-selected px-2 py-0.5 text-[11.5px] text-fg-secondary">Archived</span>}
          <span className="flex-1" />
          <span className="flex-none">
          <Tabs
            label="Chat view"
            value={tab}
            onChange={(k) => setTab(k === "activity" ? "activity" : "chat")}
            items={[
              { key: "chat", label: "Chat" },
              { key: "activity", label: "Activity" },
            ]}
          />
          </span>
          {c.archived && <RestoreChat chat={c} onRestored={restored} />}
          {actions}
        </div>
        {c.space && tab === "chat" && <SpaceContextChips space={c.space} chat={c} disabled={running} />}
        {/* Mounted while hidden so memory notices still refresh. */}
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
          <ActivityLog list={list} turns={turns} groups={groups} running={running} runId={c.current_run_id} since={since} focus={traceFocus} />
        </div>
        <div
          ref={scroller}
          role="tabpanel"
          aria-label="Chat"
          hidden={tab !== "chat"}
          className="relative flex-1 overflow-y-auto px-6 py-5"
          onScroll={(e) => {
            const el = e.currentTarget;
            const atEnd = el.scrollHeight - el.scrollTop - el.clientHeight < 120;
            pinned.current = atEnd;
            if (atEnd) setNewReply(false);
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
            <RecommendActions.Provider value={{ ask: askAuthor, disabled: running || locked || !!unavailable || c.archived, chatId: c.id, project: c.project, researchMore, canvas: canvasCards, nextMessage: (id) => list.slice(list.findIndex((m) => m.id === id) + 1).find((m) => m.role === "user")?.body_md }}>
            <ol ref={log} aria-label="Conversation" role="log" aria-live="polite" className="mx-auto flex max-w-[860px] flex-col gap-4">
              {messages.data?.has_more && <Divider>Earlier messages are not shown</Divider>}
              {list.map((m, i) => (
                <Bubble key={m.id} m={m} chat={c} persona={persona} saveable={!running} names={names} turn={turns.get(m.id)} highlighted={highlight === m.id}
                  editable={!running && !locked && !edit.isPending && !c.archived && m.id === lastUser?.id}
                  onEdit={(body) => { pinned.current = true; edit.mutate(body); }}
                  onRetry={!running && !edit.isPending && !c.archived && lastUser && m.id === list.at(-1)?.id ? () => { pinned.current = true; edit.mutate(lastUser.body_md); } : undefined}
                  summaryUrl={m.role === "assistant" && turns.has(m.id) ? summaryRequestUrl(list.slice(0, i).findLast((x) => x.role === "user")?.body_md) : null}
                  videos={videosByMessage.get(m.id)} />
              ))}
              <PersonaProposals chatId={c.id} disabled={c.archived} />
              <ConnectorRequests chatId={c.id} disabled={c.archived} />
              <SecretRequests chatId={c.id} disabled={c.archived} />
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
                  <VideoCards chatId={c.id} videos={liveVideos} onStop={stopping ? undefined : () => cancel.mutate()} />
                  <ChatStatus runId={c.current_run_id} since={since} persona={speaker} stopping={stopping} onOpenStep={openStep} />
                </li>
              )}
              <li ref={end} aria-hidden />
            </ol>
            </RecommendActions.Provider>
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
          {commandError && (
            <p role="alert" data-testid="command-error" className="text-[12.5px] text-pill-blocked-fg">
              {commandError}
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
            <ChatTicketRunTray chatId={c.id} scene={(kind) => <ActivityScene current={kind} />} />
            {startingWorkflow && (
              <div className="mb-2">
                <WorkflowStartCard workflow={startingWorkflow} chatId={c.id} onDone={() => setStartingWorkflow(null)} onDismiss={() => setStartingWorkflow(null)} />
              </div>
            )}
            <form
              data-dock-avoid=""
              onPaste={files.onPaste}
              onSubmit={(e) => {
                e.preventDefault();
                submitComposer();
              }}
              className="flex flex-col gap-2"
            >
              <AttachmentChips attachments={files} />
              <ChipList chips={chips} onRemove={(k) => setChips((cs) => removeChip(cs, k))} disabled={locked} />
              <div className="relative">
                {commandItems.length > 0 && (
                  <ul
                    role="listbox"
                    aria-label="Commands"
                    data-testid="command-suggestions"
                    className="absolute bottom-full left-2 z-10 mb-1 flex min-w-[280px] flex-col rounded-lg border border-border bg-surface-raised p-1 shadow-lg"
                  >
                    {commandItems.map((item, i) => (
                      <li key={item.name} role="option" aria-selected={i === commandActive}>
                        <button
                          type="button"
                          onMouseDown={(e) => e.preventDefault()}
                          onMouseEnter={() => setCommandActive(i)}
                          onClick={item.choose}
                          className={cx("flex w-full items-center gap-3 rounded-md px-2 py-1.5 text-left text-[13px]", i === commandActive && "bg-selected")}
                        >
                          <span className="flex-none">{item.label}</span>
                          {item.hint && <span className="ml-auto truncate text-xs text-fg-tertiary">{item.hint}</span>}
                        </button>
                      </li>
                    ))}
                  </ul>
                )}
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
                  rows={1}
                  value={text}
                  onChange={(v) => {
                    setText(v);
                    syncMention(v);
                    setCommandActive(0);
                    setCommandDismissed(false);
                    setCommandError(null);
                  }}
                  onBlur={() =>
                    setTimeout(() => {
                      if (document.activeElement === area.current) return;
                      setMention(null);
                      setCommandDismissed(true);
                    }, 120)
                  }
                  onPick={attach}
                  accept={acceptTarget}
                  disabled={locked}
                  placeholder={
                    locked
                      ? "Sending…"
                      : running
                        ? `${c.speaker || c.persona ? speaker.name : "Your assistant"} is replying. Send a message to steer it.`
                        : `Message ${c.speaker || c.persona ? speaker.name : "your assistant"}${c.space ? ` in ${c.space.name}` : ""}…  / for commands, @ for a persona, [[ for context`
                  }
                  onKeyDown={(e) => {
                    if (commandItems.length > 0) {
                      if (e.key === "ArrowDown" || e.key === "ArrowUp") {
                        e.preventDefault();
                        setCommandActive((a) => (a + (e.key === "ArrowDown" ? 1 : commandItems.length - 1)) % commandItems.length);
                        return;
                      }
                      if ((e.key === "Enter" || e.key === "Tab") && !e.shiftKey && !e.nativeEvent.isComposing) {
                        e.preventDefault();
                        commandItems[Math.min(commandActive, commandItems.length - 1)].choose();
                        return;
                      }
                      if (e.key === "Escape") {
                        e.preventDefault();
                        setCommandDismissed(true);
                        return;
                      }
                    }
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
                      submitComposer();
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
                <ResearchToggle on={research} onChange={setResearch} disabled={locked} />
                {selection && <ModelPicker groups={groups} value={selection} onChange={pick} disabled={locked || running} />}
                <EffortPicker
                  efforts={offered.efforts}
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
                  disabled={locked || !!unavailable || files.pending || !text.trim() || commitPickRequired}
                  title={
                    commitPickRequired
                      ? "Pick which email commits use first"
                      : running
                        ? "Stop the reply and answer this together with your last message"
                        : undefined
                  }
                >
                  {running ? "Interrupt" : research ? "Plan it" : "Send"}
                </Button>
              </div>
            </form>
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

"use client";

import { useCanvasPanel } from "@/lib/use-canvas-panel";
import { ResearchToggle } from "./reply-tools";
import Link from "next/link";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { useDeferredValue, useEffect, useRef, useState, type FormEvent, type ReactNode } from "react";
import type { Schemas } from "@/lib/api/client";
import { useBootstrap, useChat, useChats, useChatSearch, useCreateChat, useDeleteChat, useStartChat, useUpdateChat } from "@/lib/api/hooks";
import { usePersonas } from "@/lib/api/persona-hooks";
import { chatTitle, newestChat } from "@/lib/chat-ui";
import { codesInRepos } from "@/lib/persona-ui";
import { shortAgo } from "@/lib/format";
import { cx } from "@/lib/cx";
import { parseAttach, removeChip, toContextRefs, type Chip } from "@/lib/chat-context";
import { Button, IconButton } from "@/components/ui/button";
import { Segmented } from "@/components/ui/controls";
import { Banner, Skeleton, TextArea, TextInput } from "@/components/ui/primitives";
import { TopBar } from "@/components/shell/topbar";
import { ProjectScopeState, useProjectScope } from "@/components/shell/project-scope";
import { NoHarnessNote, useHarnessHealth } from "@/components/domain/setup";
import { Conversation } from "./conversation";
import { AttachButton, AttachmentChips, DropOverlay, useAttachments, useFileDrop } from "./attachments";
import { ChipList } from "./context-chips";
import { RestoreChat } from "./restore";
import { CanvasButton, CanvasPanel } from "./canvas-panel";
import { MemoryPanel, useMemoryPanel } from "./memory-panel";
import { PersonaAvatar, PersonaFilter, PersonaName, PersonaSwitcher, RepoSelect, personaLook, useOpenPersonaManager, usePersonaFilter } from "./persona";
import { PersonaPanel, usePersonaPanel } from "./persona-panel";

type Chat = Schemas["ChatSession"];
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
  const personas = usePersonas();
  // Undefined until picked: the chat gets the default persona.
  const [picked, setPicked] = useState<string>();
  const personaId = picked ?? personas.data?.default_persona_id;
  const fullPersona = personas.data?.personas.find((p) => p.id === personaId);
  const persona = personaLook(fullPersona);
  // A coding persona works in one of its repos: the first unless another is picked.
  const [repoPicked, setRepoPicked] = useState<string>();
  const repos = fullPersona?.repos ?? [];
  const repo = codesInRepos(fullPersona) ? (repoPicked && repos.includes(repoPicked) ? repoPicked : repos[0]) : undefined;
  const managePersonas = useOpenPersonaManager();
  const [text, setText] = useState("");
  const [research, setResearch] = useState(false);
  // A "Start chat" link with no chat yet (?attach=…) lands here; its chips go with the first message.
  const [chips, setChips] = useState<Chip[]>(() => parseAttach(search.getAll("attach"), search.get("attach_title")));
  const files = useAttachments();
  const area = useRef<HTMLTextAreaElement>(null);
  const name = boot.data?.me.name?.split(" ")[0];
  const locked = disabled || start.isPending;
  const drop = useFileDrop(files.add, !locked);

  function submit() {
    const body = text.trim();
    if (!body || locked || files.pending) return;
    const context_refs = toContextRefs(chips);
    start.mutate(
      { project, persona_id: personaId, ...(repo ? { repo_url: repo } : {}), message: { body, ...(research ? { research: true } : {}), ...(context_refs ? { context_refs } : {}), ...(files.ids.length ? { attachment_ids: files.ids } : {}) } },
      { onSuccess: (c) => router.push(`/chat/${c.id}?project=${encodeURIComponent(project)}`) },
    );
  }
  return (
    <div className="relative flex min-h-0 flex-1 flex-col" {...drop.props}>
      <DropOverlay over={drop.over} />
      <div className="flex flex-1 flex-col items-center justify-center gap-3 overflow-y-auto px-6 py-8 text-center">
        <PersonaAvatar persona={persona} size={48} />
        <h1 className="text-xl font-semibold text-fg">{name ? `Hi ${name}, what can I help with?` : "What can I help with?"}</h1>
        <p className="max-w-[520px] text-[13px] text-fg-secondary">
          I can read your runs, tickets, epics, wiki and files, search the web, and create tickets for you. I remember what matters and tell you when I do.
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
        {fullPersona?.capabilities.code && !repos.length && (
          <p className="text-[12.5px] text-fg-secondary" data-testid="no-repos-hint">
            {fullPersona.name} can write code but has no repository yet.{" "}
            <button type="button" className="text-accent hover:underline" onClick={managePersonas}>
              Edit the persona to add one
            </button>
          </p>
        )}
        <AttachmentChips attachments={files} />
        <ChipList chips={chips} onRemove={(k) => setChips((cs) => removeChip(cs, k))} disabled={locked} />
        <TextArea
          ref={area}
          aria-label="Message"
          rows={3}
          value={text}
          disabled={locked}
          placeholder={`Message ${persona.id ? persona.name : "your assistant"}… Enter sends, Shift+Enter adds a line.`}
          onChange={(e) => setText(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter" && !e.shiftKey && !e.nativeEvent.isComposing) {
              e.preventDefault();
              submit();
            }
          }}
        />
        <div className="flex items-center gap-2">
          <PersonaSwitcher value={personaId} onChange={setPicked} disabled={locked} />
          {repo && <RepoSelect repos={repos} value={repo} onChange={setRepoPicked} disabled={locked} />}
          <AttachButton onFiles={files.add} disabled={locked} />
          <ResearchToggle on={research} onChange={setResearch} disabled={locked} />
          <span className="flex-1" />
          <Button type="submit" variant="primary" disabled={locked || files.pending || !text.trim()}>
            {start.isPending ? "Starting…" : research ? "Plan it" : "Send"}
          </Button>
        </div>
      </form>
    </div>
  );
}

/** The row's persona avatar and, under the title, when it was last active (with the persona's name when listing all). */
function RowMeta({ chat, showPersona }: { chat: Chat; showPersona: boolean }) {
  const look = personaLook(chat.persona);
  return (
    <span className="flex items-center gap-1 text-[11px] text-fg-tertiary">
      {showPersona && (
        <>
          <PersonaName persona={look} className="!font-normal" />
          <span aria-hidden>·</span>
        </>
      )}
      <time dateTime={chat.last_message_at ?? chat.created_at}>{shortAgo(chat.last_message_at ?? chat.created_at)}</time>
    </span>
  );
}

/** Trash button that asks first, inline in the row; deleting can't be undone. */
function DeleteChat({ chat, onDeleted }: { chat: Chat; onDeleted: () => void }) {
  const del = useDeleteChat();
  const [asking, setAsking] = useState(false);
  const label = chatTitle(chat);
  if (!asking) return <IconButton icon="trash" label={`Delete ${label}`} onClick={() => setAsking(true)} />;
  return (
    <div role="group" aria-label={`Confirm delete ${label}`} className="flex basis-full flex-wrap items-center gap-1.5 px-2 pb-1.5">
      <span className="flex-1 text-xs text-fg">Delete this conversation? This can’t be undone.</span>
      <Button
        variant="primary"
        className="!h-6 bg-state-blocked !px-2 !text-xs"
        disabled={del.isPending}
        // mutateAsync: the row unmounts when the list refetches.
        onClick={() => del.mutateAsync(chat.id).then(onDeleted, () => undefined)}
      >
        Delete
      </Button>
      <Button variant="outline" className="!h-6 !px-2 !text-xs" onClick={() => setAsking(false)}>
        Cancel
      </Button>
      {del.isError && <p role="alert" className="basis-full text-xs text-pill-blocked-fg">{errorText(del.error)}</p>}
    </div>
  );
}

/** An archived conversation: open it read-only, or restore it to the active list. */
function ArchivedRow({ chat, active, project, onDeleted, all }: { chat: Chat; active: boolean; project: string; onDeleted: () => void; all: boolean }) {
  return (
    <li className={cx("flex flex-wrap items-center gap-1 rounded-md pr-1", active ? "bg-selected" : "hover:bg-hover")}>
      <Link
        href={`/chat/${chat.id}?project=${encodeURIComponent(project)}&view=archived`}
        aria-current={active ? "page" : undefined}
        className="flex min-w-0 flex-1 items-center gap-2 px-2 py-1.5 text-[13px]"
      >
        {all && <PersonaAvatar persona={personaLook(chat.persona)} />}
        <span className="flex min-w-0 flex-col">
          <span className="truncate">{chatTitle(chat)}</span>
          <RowMeta chat={chat} showPersona={all} />
        </span>
      </Link>
      <RestoreChat chat={chat} compact />
      <DeleteChat chat={chat} onDeleted={onDeleted} />
    </li>
  );
}

function ChatRow({ chat, active, project, onLeave, all }: { chat: Chat; active: boolean; project: string; onLeave: () => void; all: boolean }) {
  const update = useUpdateChat();
  const [editing, setEditing] = useState(false);
  const [title, setTitle] = useState(chat.title);
  const label = chatTitle(chat);

  function save(e: FormEvent) {
    e.preventDefault();
    const t = title.trim();
    if (!t || t === chat.title) return setEditing(false);
    // mutateAsync: the row may unmount on refetch before mutate-level callbacks run.
    update.mutateAsync({ id: chat.id, patch: { revision: chat.revision, title: t } }).then(() => setEditing(false), () => undefined);
  }
  return (
    <li className="flex flex-col">
      {editing ? (
        <form onSubmit={save} className="flex items-center gap-1 px-1 py-1">
          <TextInput
            aria-label="Conversation title"
            value={title}
            placeholder="New conversation"
            autoFocus
            onChange={(e) => setTitle(e.target.value)}
            onKeyDown={(e) => e.key === "Escape" && setEditing(false)}
          />
          <IconButton icon="check" label="Save title" type="submit" />
          <IconButton icon="x" label="Cancel rename" onClick={() => setEditing(false)} />
        </form>
      ) : (
        <div className={cx("group relative flex flex-wrap items-center gap-1 rounded-md pr-1", active ? "bg-selected" : "hover:bg-hover")}>
          <Link
            href={`/chat/${chat.id}?project=${encodeURIComponent(project)}`}
            aria-current={active ? "page" : undefined}
            className="flex min-w-0 flex-1 items-center gap-2 px-2 py-1.5 text-[13px]"
          >
            {all && <PersonaAvatar persona={personaLook(chat.persona)} />}
            <span className="flex min-w-0 flex-1 flex-col">
              <span className="flex min-w-0 items-center gap-2">
                {chat.state === "running" && <span aria-hidden className="size-1.5 flex-none rounded-full bg-state-done" />}
                <span className="truncate">{label}</span>
                {chat.state === "running" && <span className="sr-only"> (running)</span>}
                {chat.state === "failed" && <span className="text-xs text-pill-blocked-fg">Failed</span>}
              </span>
              <RowMeta chat={chat} showPersona={all} />
            </span>
          </Link>
          {/* Over the row until hovered, so titles get the full width. A delete confirmation takes its own line. */}
          <div className={cx("absolute right-1 top-1/2 flex -translate-y-1/2 items-center gap-1 rounded-md opacity-0 transition-opacity group-hover:opacity-100 focus-within:opacity-100 [@media(hover:none)]:opacity-100 has-[[role=group]]:static has-[[role=group]]:basis-full has-[[role=group]]:translate-y-0 has-[[role=group]]:flex-wrap has-[[role=group]]:opacity-100", active ? "bg-selected" : "bg-hover")}>
          <IconButton icon="edit" label={`Rename ${label}`} onClick={() => { setTitle(chat.title); setEditing(true); }} />
          <IconButton
            icon="folder"
            label={`Archive ${label}`}
            disabled={update.isPending}
            onClick={() =>
              update.mutateAsync({ id: chat.id, patch: { revision: chat.revision, archived: true } }).then(onLeave, () => undefined)
            }
          />
          <DeleteChat chat={chat} onDeleted={onLeave} />
          </div>
        </div>
      )}
      {update.isError && (
        <p role="alert" className="px-2 pb-1 text-xs text-pill-blocked-fg">
          {errorText(update.error)}
        </p>
      )}
    </li>
  );
}

const MATCH = /(\uE000[^\uE001]*\uE001)/;

/** A search hit's excerpt with its matches marked; the excerpt is only ever text. */
function Snippet({ text }: { text: string }) {
  return (
    <span className="line-clamp-2 text-xs text-fg-secondary">
      {text.split(MATCH).map((part, i) => (i % 2 ? <mark key={i} className="rounded-sm bg-accent-soft px-px text-fg">{part.slice(1, -1)}</mark> : part))}
    </span>
  );
}

/** Message search across the person's chats: results stand in for the list while there's a query. */
function ChatSearchResults({ query, onPick }: { query: string; onPick: () => void }) {
  const results = useChatSearch(query);
  const hits = results.data?.hits ?? [];
  if (query.trim().length < 2 || results.isPending) return <p className="px-2 py-1.5 text-xs text-fg-tertiary">{query.trim().length < 2 ? "Keep typing…" : "Searching…"}</p>;
  if (results.isError) return <p role="alert" className="px-2 py-1.5 text-xs text-pill-blocked-fg">{errorText(results.error)}</p>;
  if (hits.length === 0) return <p className="px-2 py-1.5 text-xs text-fg-tertiary">No messages match.</p>;
  return (
    <ul aria-label="Search results" className="flex flex-col gap-0.5">
      {hits.map((h) => (
        <li key={h.message_id} data-testid="chat-search-hit">
          <Link href={`/chat/${h.chat_id}?project=${encodeURIComponent(h.project_key)}&message=${h.message_id}`} onClick={onPick} className="flex flex-col gap-0.5 rounded-md px-2 py-1.5 hover:bg-hover">
            <span className="truncate text-[13px] text-fg">{h.chat_title || "New conversation"}</span>
            <Snippet text={h.snippet} />
            <span className="truncate text-xs text-fg-tertiary">{h.role === "user" ? "You" : (h.persona_name ?? "Assistant")}{h.archived ? " · archived" : ""}</span>
          </Link>
        </li>
      ))}
    </ul>
  );
}

/** The conversation history: Active/Archived (kept in ?view= so it survives opening one), with rename, archive and restore. */
function HistorySidebar({ project, id, newChat }: { project: string; id?: string; newChat: ReactNode }) {
  const router = useRouter();
  const pathname = usePathname();
  const search = useSearchParams();
  const view = search.get("view") === "archived" ? "archived" : "active";
  const setView = (v: "active" | "archived") => {
    const q = new URLSearchParams(search.toString());
    if (v === "archived") q.set("view", "archived");
    else q.delete("view");
    router.replace(`${pathname}?${q.toString()}`);
  };
  // The persona dropdown starts on the default persona; GET /chats does the filtering.
  const filter = usePersonaFilter();
  const all = filter.query === "all";
  const active = useChats(project, false, filter.query);
  const archived = useChats(view === "archived" ? project : "", true, filter.query);
  const chats = view === "archived" ? archived : active;
  const list = chats.data?.chats;
  // Picking a persona opens its newest conversation, unless the open one is already with it.
  // Set by a pick; the effect below runs once that persona's list arrives (a new query, so new data).
  const following = useRef(false);
  const pickPersona = (v: typeof filter.value) => {
    if (v === filter.value) return;
    following.current = v !== "all";
    filter.set(v);
  };
  useEffect(() => {
    if (!following.current || !active.data) return;
    following.current = false;
    const open = active.data.chats.some((c) => c.id === id);
    const target = newestChat(active.data.chats);
    if (open || !target) return;
    const q = new URLSearchParams(search.toString());
    q.set("project", project);
    router.push(`/chat/${target.id}?${q.toString()}`);
  }, [active.data, id, project, router, search]);
  const create = useCreateChat();
  // An empty filter on a persona other than the default offers to start a conversation with it.
  const picked = filter.value !== "default" && !all ? filter.personas.find((p) => p.id === filter.value) : undefined;
  const startWith = (personaId: string) =>
    create.mutate({ project, persona_id: personaId }, { onSuccess: (c) => router.push(`/chat/${c.id}?project=${encodeURIComponent(project)}`) });
  // Archiving or deleting the open conversation goes back to /chat, which resumes the newest one.
  const leave = (chatId: string) => chatId === id && router.push(`/chat?project=${encodeURIComponent(project)}`);
  const [query, setQuery] = useState("");
  const deferredQuery = useDeferredValue(query);
  return (
    <aside
      aria-label="Chat history"
      className="flex w-72 flex-none flex-col gap-3 border-r border-border-subtle p-3 max-md:max-h-64 max-md:w-full max-md:border-b max-md:border-r-0"
    >
      <PersonaFilter filter={{ ...filter, set: pickPersona }} />
      {newChat}
      <TextInput aria-label="Find in chats" placeholder="Find in chats" value={query} onChange={(e) => setQuery(e.target.value)} onKeyDown={(e) => e.key === "Escape" && setQuery("")} />
      {!query && <Segmented
        label="Show conversations"
        small
        value={view}
        onChange={setView}
        options={[
          { value: "active", label: "Active" },
          { value: "archived", label: "Archived" },
        ]}
      />}
      <nav aria-label="Chats" className="min-h-0 flex-1 overflow-y-auto">
        {query ? (
          <ChatSearchResults query={deferredQuery} onPick={() => undefined} />
        ) : chats.isError ? (
          <Banner tone="blocked" title="Couldn’t load conversations">{errorText(chats.error)}</Banner>
        ) : !list ? (
          <div aria-busy="true" className="flex flex-col gap-2">
            {[80, 60, 70].map((w) => (
              <Skeleton key={w} className="h-5" style={{ width: `${w}%` }} />
            ))}
          </div>
        ) : list.length === 0 ? (
          view === "active" && picked ? (
            <div className="flex flex-col items-start gap-1 px-2 py-4 text-[13px]">
              <p className="text-fg-secondary">No conversations with {picked.name} yet.</p>
              <button type="button" className="text-accent hover:underline disabled:opacity-50" disabled={!project || create.isPending} onClick={() => startWith(picked.id)}>
                Start one with {picked.name} →
              </button>
              {create.isError && <p role="alert" className="text-xs text-pill-blocked-fg">{errorText(create.error)}</p>}
            </div>
          ) : (
            <p className="px-2 py-4 text-[13px] text-fg-secondary">
              {view === "archived" ? "No archived conversations." : "No conversations yet."}
            </p>
          )
        ) : view === "archived" ? (
          <>
            <ul className="flex flex-col gap-px">
              {list.map((c) => (
                <ArchivedRow key={c.id} chat={c} project={project} active={c.id === id} all={all} onDeleted={() => leave(c.id)} />
              ))}
            </ul>
            <p className="px-2 py-3 text-xs text-fg-tertiary">Archived conversations are read-only. Restore one to continue it.</p>
          </>
        ) : (
          <ul className="flex flex-col gap-px">
            {list.map((c) => (
              <ChatRow
                key={c.id}
                chat={c}
                project={project}
                active={c.id === id}
                all={all}
                onLeave={() => leave(c.id)}
              />
            ))}
          </ul>
        )}
      </nav>
    </aside>
  );
}

/**
 * The Personal Assistant (replaces Chat): one conversation surface with no agent
 * picker and a history sidebar (rename, archive, restore, delete) beside it.
 * /chat opens the newest conversation, or the first-time welcome; "New
 * conversation" starts afresh. Memory and Personas open as right-hand panels from the main menu.
 */
export function ChatScreen({ id }: { id?: string }) {
  const router = useRouter();
  const search = useSearchParams();
  const current = useChat(id);
  // An open chat carries its project; the index resolves ?project=, then the remembered one.
  const scope = useProjectScope("required", { skip: !!id });
  const project = id
    ? (current.data?.project ?? search.get("project") ?? "")
    : scope.status === "ready" ? (scope.project?.key ?? "") : "";
  const memory = useMemoryPanel();
  const personaPanel = usePersonaPanel();
  const canvas = useCanvasPanel();
  const turnRunning = current.data?.state === "running";
  const create = useCreateChat();
  const personas = usePersonas();
  const health = useHarnessHealth();
  const noHarness = !!health && !health.ready;
  // /chat resumes the newest conversation, unless a "Start chat" link brought context to attach.
  const attaching = search.has("attach");
  // Resume the newest conversation with any persona, whatever the sidebar shows.
  const chats = useChats(project, false, "all");
  const newest = newestChat(chats.data?.chats);
  useEffect(() => {
    if (id || attaching || !newest) return;
    const q = new URLSearchParams(search.toString());
    q.set("project", project);
    router.replace(`/chat/${newest.id}?${q.toString()}`);
  }, [id, attaching, newest, project, router, search]);

  // The sidebar's persona picker (shared through storage) decides who a new conversation is with.
  const filter = usePersonaFilter();
  function newConversation() {
    // New conversations use the persona picked in the sidebar, or the default when it shows
    // the default or all personas; the composer can switch it before the first message.
    const persona_id = filter.value !== "default" && filter.value !== "all" ? filter.value : personas.data?.default_persona_id;
    // An empty current conversation with that persona is already new.
    if (id && current.data?.message_count === 0 && current.data.persona?.id === persona_id) return;
    create.mutate({ project, ...(persona_id ? { persona_id } : {}) }, { onSuccess: (c) => router.push(`/chat/${c.id}?project=${encodeURIComponent(project)}`) });
  }
  // Sits under the sidebar's persona picker and follows it.
  const newChat = (
    <Button variant="secondary" icon="plus" className="w-full justify-center" disabled={!project || noHarness || create.isPending} onClick={newConversation}>
      New conversation
    </Button>
  );
  const resolving = !id && !attaching && (!chats.data || !!newest);
  return (
    <div className="flex h-dvh flex-col">
      {/* The assistant reads as personal, not per project: no project crumb. */}
      <TopBar crumbs={[{ label: "Assistant" }]} />
      {!id && scope.status !== "ready" ? (
        <ProjectScopeState base="/chat" scope={scope} />
      ) : (
        <div className="relative flex min-h-0 flex-1 max-md:flex-col">
          <HistorySidebar project={project} id={id} newChat={newChat} />
          <div className="flex min-w-0 flex-1 flex-col">
            {(noHarness || create.isError) && (
              <div className="flex flex-col gap-1 px-6 pt-3">
                {noHarness && <NoHarnessNote health={health} />}
                {create.isError && <p role="alert" className="text-xs text-pill-blocked-fg">{errorText(create.error)}</p>}
              </div>
            )}
            {id ? (
              <Conversation key={id} id={id} onNew={newConversation} actions={<CanvasButton chatId={id} live={turnRunning} />} />
            ) : (
              <>
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
          {id && canvas.open ? <CanvasPanel chatId={id} live={turnRunning} readOnly={!!current.data?.archived} agent={current.data?.persona?.name ?? "Assistant"} /> : personaPanel.open ? <PersonaPanel /> : memory.open && <MemoryPanel />}
        </div>
      )}
    </div>
  );
}

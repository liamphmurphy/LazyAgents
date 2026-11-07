"use client";

import Link from "next/link";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { useEffect, useRef, useState, type FormEvent } from "react";
import type { Schemas } from "@/lib/api/client";
import { useBootstrap, useChat, useChats, useCreateChat, useDeleteChat, useStartChat, useUpdateChat } from "@/lib/api/hooks";
import { chatTitle } from "@/lib/chat-ui";
import { cx } from "@/lib/cx";
import { parseAttach, removeChip, toContextRefs, type Chip } from "@/lib/chat-context";
import { Button, IconButton } from "@/components/ui/button";
import { Segmented } from "@/components/ui/controls";
import { Banner, Skeleton, TextArea, TextInput } from "@/components/ui/primitives";
import { TopBar } from "@/components/shell/topbar";
import { ProjectScopeState, projectCrumbs, useProjectScope } from "@/components/shell/project-scope";
import { NoHarnessNote, useHarnessHealth } from "@/components/domain/setup";
import { Conversation } from "./conversation";
import { AttachButton, AttachmentChips, DropOverlay, useAttachments, useFileDrop } from "./attachments";
import { ChipList } from "./context-chips";
import { RestoreChat } from "./restore";
import { MemoryButton, MemoryPanel, useMemoryPanel } from "./memory-panel";

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
  const [text, setText] = useState("");
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
      { project, message: { body, ...(context_refs ? { context_refs } : {}), ...(files.ids.length ? { attachment_ids: files.ids } : {}) } },
      { onSuccess: (c) => router.push(`/chat/${c.id}?project=${encodeURIComponent(project)}`) },
    );
  }
  return (
    <div className="relative flex min-h-0 flex-1 flex-col" {...drop.props}>
      <DropOverlay over={drop.over} />
      <div className="flex flex-1 flex-col items-center justify-center gap-3 overflow-y-auto px-6 py-8 text-center">
        <span aria-hidden className="grid size-12 place-items-center rounded-full bg-accent/15 text-xl font-semibold text-accent">
          A
        </span>
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
        <AttachmentChips attachments={files} />
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
        <div className="flex items-center gap-2">
          <AttachButton onFiles={files.add} disabled={locked} />
          <span className="flex-1" />
          <Button type="submit" variant="primary" disabled={locked || files.pending || !text.trim()}>
            {start.isPending ? "Starting…" : "Send"}
          </Button>
        </div>
      </form>
    </div>
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
function ArchivedRow({ chat, active, project, onDeleted }: { chat: Chat; active: boolean; project: string; onDeleted: () => void }) {
  return (
    <li className={cx("flex flex-wrap items-center gap-1 rounded-md pr-1", active ? "bg-selected" : "hover:bg-hover")}>
      <Link
        href={`/chat/${chat.id}?project=${encodeURIComponent(project)}&view=archived`}
        aria-current={active ? "page" : undefined}
        className="min-w-0 flex-1 truncate px-2 py-1.5 text-[13px]"
      >
        {chatTitle(chat)}
      </Link>
      <RestoreChat chat={chat} compact />
      <DeleteChat chat={chat} onDeleted={onDeleted} />
    </li>
  );
}

function ChatRow({ chat, active, project, onLeave }: { chat: Chat; active: boolean; project: string; onLeave: () => void }) {
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
        <div className={cx("group flex flex-wrap items-center gap-1 rounded-md pr-1", active ? "bg-selected" : "hover:bg-hover")}>
          <Link
            href={`/chat/${chat.id}?project=${encodeURIComponent(project)}`}
            aria-current={active ? "page" : undefined}
            className="flex min-w-0 flex-1 items-center gap-2 px-2 py-1.5 text-[13px]"
          >
            {chat.state === "running" && <span aria-hidden className="size-1.5 flex-none rounded-full bg-state-done" />}
            <span className="truncate">{label}</span>
            {chat.state === "running" && <span className="sr-only"> (running)</span>}
            {chat.state === "failed" && <span className="text-xs text-pill-blocked-fg">Failed</span>}
          </Link>
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
      )}
      {update.isError && (
        <p role="alert" className="px-2 pb-1 text-xs text-pill-blocked-fg">
          {errorText(update.error)}
        </p>
      )}
    </li>
  );
}

/** The conversation history: Active/Archived (kept in ?view= so it survives opening one), with rename, archive and restore. */
function HistorySidebar({ project, id }: { project: string; id?: string }) {
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
  const active = useChats(project);
  const archived = useChats(view === "archived" ? project : "", true);
  const chats = view === "archived" ? archived : active;
  const list = chats.data?.chats;
  // Archiving or deleting the open conversation goes back to /chat, which resumes the newest one.
  const leave = (chatId: string) => chatId === id && router.push(`/chat?project=${encodeURIComponent(project)}`);
  return (
    <aside
      aria-label="Chat history"
      className="flex w-72 flex-none flex-col gap-3 border-r border-border-subtle p-3 max-md:max-h-64 max-md:w-full max-md:border-b max-md:border-r-0"
    >
      <Segmented
        label="Show conversations"
        small
        value={view}
        onChange={setView}
        options={[
          { value: "active", label: "Active" },
          { value: "archived", label: "Archived" },
        ]}
      />
      <nav aria-label="Chats" className="min-h-0 flex-1 overflow-y-auto">
        {chats.isError ? (
          <Banner tone="blocked" title="Couldn’t load conversations">{errorText(chats.error)}</Banner>
        ) : !list ? (
          <div aria-busy="true" className="flex flex-col gap-2">
            {[80, 60, 70].map((w) => (
              <Skeleton key={w} className="h-5" style={{ width: `${w}%` }} />
            ))}
          </div>
        ) : list.length === 0 ? (
          <p className="px-2 py-4 text-[13px] text-fg-secondary">
            {view === "archived" ? "No archived conversations." : "No conversations yet."}
          </p>
        ) : view === "archived" ? (
          <>
            <ul className="flex flex-col gap-px">
              {list.map((c) => (
                <ArchivedRow key={c.id} chat={c} project={project} active={c.id === id} onDeleted={() => leave(c.id)} />
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
 * conversation" starts afresh. Memory opens as a right-hand panel.
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
  const chats = useChats(project);
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
        <div className="relative flex min-h-0 flex-1 max-md:flex-col">
          <HistorySidebar project={project} id={id} />
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

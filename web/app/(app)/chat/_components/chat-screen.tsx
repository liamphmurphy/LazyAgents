"use client";

import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { useState, type FormEvent } from "react";
import type { Schemas } from "@/lib/api/client";
import { useBootstrap, useChat, useChats, useCreateChat, useUpdateChat } from "@/lib/api/hooks";
import { cx } from "@/lib/cx";
import { Button, IconButton } from "@/components/ui/button";
import { Banner, EmptyState, Skeleton, TextInput } from "@/components/ui/primitives";
import { TopBar } from "@/components/shell/topbar";
import { Conversation } from "./conversation";

type Chat = Schemas["ChatSession"];
const errorText = (e: unknown) => (e instanceof Error ? e.message : "Something went wrong. Please try again.");

function ChatRow({ chat, active, project, onArchived }: { chat: Chat; active: boolean; project: string; onArchived: () => void }) {
  const update = useUpdateChat();
  const [editing, setEditing] = useState(false);
  const [title, setTitle] = useState(chat.title);

  function save(e: FormEvent) {
    e.preventDefault();
    const t = title.trim();
    if (!t || t === chat.title) return setEditing(false);
    update.mutate({ id: chat.id, patch: { revision: chat.revision, title: t } }, { onSuccess: () => setEditing(false) });
  }
  return (
    <li className="flex flex-col">
      {editing ? (
        <form onSubmit={save} className="flex items-center gap-1 px-1 py-1">
          <TextInput
            aria-label="Chat title"
            value={title}
            autoFocus
            onChange={(e) => setTitle(e.target.value)}
            onKeyDown={(e) => e.key === "Escape" && setEditing(false)}
          />
          <IconButton icon="check" label="Save title" type="submit" />
          <IconButton icon="x" label="Cancel rename" onClick={() => setEditing(false)} />
        </form>
      ) : (
        <div className={cx("group flex items-center gap-1 rounded-md pr-1", active ? "bg-selected" : "hover:bg-hover")}>
          <Link
            href={`/chat/${chat.id}?project=${encodeURIComponent(project)}`}
            aria-current={active ? "page" : undefined}
            className="flex min-w-0 flex-1 items-center gap-2 px-2 py-1.5 text-[13px]"
          >
            {chat.state === "running" && <span aria-hidden className="size-1.5 flex-none rounded-full bg-state-done" />}
            <span className="truncate">{chat.title}</span>
            {chat.state === "running" && <span className="sr-only"> (running)</span>}
            {chat.state === "failed" && <span className="text-xs text-pill-blocked-fg">Failed</span>}
          </Link>
          <IconButton icon="edit" label={`Rename ${chat.title}`} onClick={() => { setTitle(chat.title); setEditing(true); }} />
          <IconButton
            icon="folder"
            label={`Archive ${chat.title}`}
            disabled={update.isPending}
            onClick={() =>
              update.mutate({ id: chat.id, patch: { revision: chat.revision, archived: true } }, { onSuccess: onArchived })
            }
          />
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

export function ChatScreen({ id }: { id?: string }) {
  const router = useRouter();
  const search = useSearchParams();
  const boot = useBootstrap();
  const current = useChat(id);
  const projects = boot.data?.projects.filter((p) => !p.archived) ?? [];
  const project = current.data?.project ?? search.get("project") ?? projects[0]?.key ?? "";
  const chats = useChats(project);
  const create = useCreateChat();

  function newChat() {
    create.mutate({ project }, { onSuccess: (c) => router.push(`/chat/${c.id}?project=${encodeURIComponent(project)}`) });
  }
  const list = chats.data?.chats;
  return (
    <div className="flex h-dvh flex-col">
      <TopBar crumbs={[{ label: "Chat" }]} />
      <div className="flex min-h-0 flex-1 max-md:flex-col">
        <aside
          aria-label="Chat history"
          className="flex w-72 flex-none flex-col gap-3 border-r border-border-subtle p-3 max-md:max-h-64 max-md:w-full max-md:border-b max-md:border-r-0"
        >
          <div className="flex items-center gap-2">
            <select
              aria-label="Project"
              value={project}
              onChange={(e) => router.push(`/chat?project=${encodeURIComponent(e.target.value)}`)}
              className="h-[30px] min-w-0 flex-1 rounded-md border border-border-strong bg-surface px-2 text-[13px] text-fg"
            >
              {projects.map((p) => (
                <option key={p.key} value={p.key}>{p.name}</option>
              ))}
            </select>
            <Button variant="primary" icon="plus" disabled={!project || create.isPending} onClick={newChat}>
              New chat
            </Button>
          </div>
          {create.isError && (
            <p role="alert" className="text-xs text-pill-blocked-fg">{errorText(create.error)}</p>
          )}
          <nav aria-label="Chats" className="min-h-0 flex-1 overflow-y-auto">
            {chats.isError ? (
              <Banner tone="blocked" title="Couldn’t load chats">{errorText(chats.error)}</Banner>
            ) : !list ? (
              <div aria-busy="true" className="flex flex-col gap-2">
                {[80, 60, 70].map((w) => (
                  <Skeleton key={w} className="h-5" style={{ width: `${w}%` }} />
                ))}
              </div>
            ) : list.length === 0 ? (
              <p className="px-2 py-4 text-[13px] text-fg-secondary">No chats yet. Start one with New chat.</p>
            ) : (
              <ul className="flex flex-col gap-px">
                {list.map((c) => (
                  <ChatRow
                    key={c.id}
                    chat={c}
                    project={project}
                    active={c.id === id}
                    onArchived={() => c.id === id && router.push(`/chat?project=${encodeURIComponent(project)}`)}
                  />
                ))}
              </ul>
            )}
          </nav>
        </aside>
        {id ? (
          <Conversation key={id} id={id} />
        ) : (
          <div className="flex flex-1 items-center justify-center">
            <EmptyState
              icon="msg"
              title="Chat with your project"
              actions={
                <Button variant="primary" icon="plus" disabled={!project || create.isPending} onClick={newChat}>
                  New chat
                </Button>
              }
            >
              Ask questions grounded in the project’s wiki, artifacts and tickets. Switch model any time.
            </EmptyState>
          </div>
        )}
      </div>
    </div>
  );
}

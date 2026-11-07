"use client";

import Link from "next/link";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { useState, type FormEvent } from "react";
import type { Schemas } from "@/lib/api/client";
import { useBootstrap, useChat, useChats, useCreateChat, useUpdateChat } from "@/lib/api/hooks";
import { cx } from "@/lib/cx";
import { Button, IconButton } from "@/components/ui/button";
import { Segmented } from "@/components/ui/controls";
import { Banner, EmptyState, Skeleton, TextInput } from "@/components/ui/primitives";
import { TopBar } from "@/components/shell/topbar";
import { ProjectScopeState, projectCrumbs, useProjectScope } from "@/components/shell/project-scope";
import { Conversation } from "./conversation";
import { RestoreChat } from "./restore";
import { NoHarnessNote, useHarnessHealth } from "@/components/domain/setup";

type Chat = Schemas["ChatSession"];
const errorText = (e: unknown) => (e instanceof Error ? e.message : "Something went wrong. Please try again.");

/** An archived chat: open it read-only, or restore it to the active list. */
function ArchivedRow({ chat, active, project }: { chat: Chat; active: boolean; project: string }) {
  return (
    <li className={cx("flex items-center gap-1 rounded-md pr-1", active ? "bg-selected" : "hover:bg-hover")}>
      <Link
        href={`/chat/${chat.id}?project=${encodeURIComponent(project)}&view=archived`}
        aria-current={active ? "page" : undefined}
        className="min-w-0 flex-1 truncate px-2 py-1.5 text-[13px]"
      >
        {chat.title}
      </Link>
      <RestoreChat chat={chat} compact />
    </li>
  );
}

function ChatRow({ chat, active, project, onArchived }: { chat: Chat; active: boolean; project: string; onArchived: () => void }) {
  const update = useUpdateChat();
  const [editing, setEditing] = useState(false);
  const [title, setTitle] = useState(chat.title);

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
              update.mutateAsync({ id: chat.id, patch: { revision: chat.revision, archived: true } }).then(onArchived, () => undefined)
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
  // An open chat carries its project; the index resolves ?project=, then the remembered one.
  const scope = useProjectScope("required", { skip: !!id });
  const project = id
    ? (current.data?.project ?? search.get("project") ?? "")
    : scope.status === "ready" ? (scope.project?.key ?? "") : "";
  const scoped = boot.data?.projects.find((p) => p.key === project);
  // The filter lives in the URL so it survives opening a chat (a new route).
  const pathname = usePathname();
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
  const create = useCreateChat();
  // Nothing can answer while no harness is available: say so before a chat is created.
  const health = useHarnessHealth();
  const noHarness = !!health && !health.ready;

  function newChat() {
    // /chat?attach=… (from a Start chat link) carries over to the new chat.
    const q = new URLSearchParams({ project });
    for (const v of search.getAll("attach")) q.append("attach", v);
    const t = search.get("attach_title");
    if (t) q.set("attach_title", t);
    // Every chat is with the Personal Assistant: there is no agent to pick.
    create.mutate({ project }, { onSuccess: (c) => router.push(`/chat/${c.id}?${q.toString()}`) });
  }
  const list = chats.data?.chats;
  return (
    <div className="flex h-dvh flex-col">
      <TopBar crumbs={projectCrumbs(scoped, "Chat")} />
      {!id && scope.status !== "ready" ? (
        <ProjectScopeState base="/chat" scope={scope} />
      ) : (
      <div className="flex min-h-0 flex-1 max-md:flex-col">
        <aside
          aria-label="Chat history"
          className="flex w-72 flex-none flex-col gap-3 border-r border-border-subtle p-3 max-md:max-h-64 max-md:w-full max-md:border-b max-md:border-r-0"
        >
          <div className="flex items-center gap-2">
            <Button variant="primary" icon="plus" className="flex-1 justify-center" disabled={!project || noHarness || create.isPending} onClick={newChat}>
              New chat
            </Button>
          </div>
          {noHarness && <NoHarnessNote health={health} />}
          {create.isError && (
            <p role="alert" className="text-xs text-pill-blocked-fg">{errorText(create.error)}</p>
          )}
          <Segmented
            label="Show chats"
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
              <Banner tone="blocked" title="Couldn’t load chats">{errorText(chats.error)}</Banner>
            ) : !list ? (
              <div aria-busy="true" className="flex flex-col gap-2">
                {[80, 60, 70].map((w) => (
                  <Skeleton key={w} className="h-5" style={{ width: `${w}%` }} />
                ))}
              </div>
            ) : list.length === 0 ? (
              <p className="px-2 py-4 text-[13px] text-fg-secondary">
                {view === "archived" ? "No archived chats." : "No chats yet. Start one with New chat."}
              </p>
            ) : view === "archived" ? (
              <>
                <ul className="flex flex-col gap-px">
                  {list.map((c) => (
                    <ArchivedRow key={c.id} chat={c} project={project} active={c.id === id} />
                  ))}
                </ul>
                <p className="px-2 py-3 text-xs text-fg-tertiary">Archived chats are read-only. Restore one to continue it.</p>
              </>
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
                <Button variant="primary" icon="plus" disabled={!project || noHarness || create.isPending} onClick={newChat}>
                  New chat
                </Button>
              }
            >
              Ask questions grounded in the project’s wiki, artifacts and tickets. Switch model any time.
            </EmptyState>
          </div>
        )}
      </div>
      )}
    </div>
  );
}

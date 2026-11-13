"use client";

import { useRouter } from "next/navigation";
import { useCallback, useEffect, useId, useRef, useState } from "react";
import { Conversation, type ConversationContext } from "@/app/(app)/chat/_components/conversation";
import { PersonaAvatar, personaLook } from "@/app/(app)/chat/_components/persona";
import { Dock, type DockTab } from "@/components/shell/editor-layout";
import { Button } from "@/components/ui/button";
import { Icon } from "@/components/ui/icon";
import { Skeleton } from "@/components/ui/primitives";
import { isTyping } from "@/components/shell/shortcuts";
import { provideChatRefs, useChat, useCreateChat } from "@/lib/api/hooks";
import { fetchPage, useAttachChat, usePage, type Page } from "@/lib/api/knowledge-hooks";
import type { Schemas } from "@/lib/api/client";
import { refKey } from "@/lib/chat-context";
import { dockReducer, dockShortcut, loadDock, saveDock, type DockAction, type DockState } from "@/lib/editor-layout";
import { useKnowledgeChat } from "./chat-context";

const APP = "knowledge";
const TABS = ["chat"];
/** The chat for screens that aren't a page (Home, Graph, Connect). A page's own chat is the one it's attached to. */
const GENERAL_CHAT_KEY = "vesper.knowledge.chat";

/**
 * The persona chat on the right of every Knowledge screen, on the shared dock: it folds to a rail and starts
 * folded. On a page it is that page's chat; elsewhere one general Knowledge chat.
 */
export function KnowledgeDock({ pageId }: { pageId?: string }) {
  const { request } = useKnowledgeChat();
  const uid = useId();
  const page = usePage(pageId).data;
  const [general, setGeneral] = useState<string | null>(null);
  const [state, setState] = useState<DockState>({ open: false, tab: "chat", hidden: false });
  const dispatch = useCallback((a: DockAction) => setState((s) => dockReducer(s, a)), []);
  const [loaded, setLoaded] = useState(false);
  const [focusChat, setFocusChat] = useState(0);

  // Read after mount so the server render matches the first client render.
  const restored = useRef(false);
  useEffect(() => {
    if (restored.current) return;
    restored.current = true;
    setState(loadDock(window.localStorage, APP, TABS, false));
    setGeneral(window.localStorage.getItem(GENERAL_CHAT_KEY));
    setLoaded(true);
  }, []);
  useEffect(() => {
    if (loaded) saveDock(window.localStorage, APP, state);
  }, [state, loaded]);
  const [seenRequest, setSeenRequest] = useState(request);
  if (request !== seenRequest) {
    setSeenRequest(request);
    dispatch({ type: "open", tab: "chat" });
  }

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const action = dockShortcut(e, isTyping(e.target));
      if (action !== "toggle" && action !== "chat") return;
      e.preventDefault();
      if (action === "toggle") dispatch({ type: "toggle" });
      else {
        dispatch({ type: "open", tab: "chat" });
        setFocusChat((n) => n + 1);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [dispatch]);

  // ⌘J: once the panel has rendered, focus its composer.
  useEffect(() => {
    if (!focusChat) return;
    const frame = requestAnimationFrame(() => document.querySelector<HTMLElement>(`[data-dock-panel="${CSS.escape(`${uid}:chat`)}"] textarea, [data-dock-panel="${CSS.escape(`${uid}:chat`)}"] [contenteditable='true']`)?.focus());
    return () => cancelAnimationFrame(frame);
  }, [focusChat, uid]);

  const chatId = pageId ? (page?.chat_id ?? undefined) : (general ?? undefined);
  useEffect(() => {
    if (chatId && pageId) return provideChatRefs(chatId, () => [{ kind: "page", id: pageId }]);
  }, [chatId, pageId]);
  const chat = useChat(chatId).data;
  const running = chat?.state === "running";
  const shown = state.open && !state.hidden;

  // A reply that arrived while the chat was out of sight is unread; one there on load, or that arrives while it shows, is read.
  const lastReply = chat?.last_message_at;
  const [seen, setSeen] = useState<{ chat?: string; at?: string }>({});
  if (lastReply && (seen.chat !== chatId || (shown && seen.at !== lastReply))) setSeen({ chat: chatId, at: lastReply });
  const unread = !shown && !!lastReply && seen.chat === chatId && seen.at !== lastReply && !running;

  const tabs: DockTab[] = [
    {
      id: "chat",
      label: "Chat",
      icon: "msg",
      avatar: <PersonaAvatar persona={personaLook(chat?.speaker ?? chat?.persona)} size={16} />,
      busy: running,
      unread,
      content: (
        <ChatBody
          key={pageId ?? "general"}
          chatId={chatId}
          page={pageId ? page : undefined}
          loadingPage={!!pageId && !page}
          onGeneral={(id) => {
            window.localStorage.setItem(GENERAL_CHAT_KEY, id);
            setGeneral(id);
          }}
        />
      ),
    },
  ];
  // Hidden below md: the page sidebar and explorer already fill a phone.
  return (
    <div className="hidden md:contents">
      <Dock uid={uid} tabs={tabs} state={state} dispatch={dispatch} panels={shown ? undefined : null} />
    </div>
  );
}

function ChatBody({ chatId, page, loadingPage, onGeneral }: { chatId?: string; page?: Page; loadingPage: boolean; onGeneral: (id: string) => void }) {
  const router = useRouter();
  const create = useCreateChat();
  const attach = useAttachChat(page?.id ?? "");
  const { attachments, request, remove, readDraft, claimRequest } = useKnowledgeChat();
  const [created, setCreated] = useState<string>();
  const id = created ?? chatId;
  const host = useRef<HTMLDivElement>(null);
  const selectChat = async (next: string) => {
    setCreated(next);
    if (page) await attach.mutateAsync(next);
    else onGeneral(next);
  };
  const start = () => {
    create.mutate(
      { title: page?.title ?? "Knowledge", hidden: true },
      { onSuccess: (c) => { void selectChat(c.id).catch(() => { /* attach.error keeps the chat available with Retry. */ }); } },
    );
  };

  // Only an explicit Ask action creates an empty conversation; opening the rail does not.
  useEffect(() => {
    if (!request || loadingPage || !claimRequest(request)) return;
    if (!id && !create.isPending) start();
  });
  useEffect(() => {
    if (!request || !id) return;
    const frame = requestAnimationFrame(() => host.current?.querySelector<HTMLElement>("textarea")?.focus());
    return () => cancelAnimationFrame(frame);
  }, [request, id]);

  const refs: Schemas["ChatContextRef"][] = [];
  for (const p of [...(page ? [page] : []), ...attachments]) {
    if (!refs.some((r) => r.id === p.id)) refs.push({ kind: "page", id: p.id });
  }
  const quote = (text: string) => text.split("\n").map((line) => `> ${line}`).join("\n");
  const label = (p: { path: string | null; title: string }) => `[[${p.path ?? p.title}]]`;
  const passages = attachments.filter((a) => a.quote !== undefined).map((a) => `Selected passage from ${label(a)}:\n${quote(a.quote!)}`);
  const contextBody = (body: string, snapshots: string[] = []) => {
    const viewing = page ? [`Currently viewing Knowledge page ${label(page)}.`] : [];
    const blocks = [...viewing, ...passages, ...snapshots];
    // The server excludes this source material when interpreting persona mentions and hand-off limits.
    const suffix = `\n\n<!-- vesper:knowledge-context -->\n${blocks.join("\n\n")}`;
    return blocks.length && !body.endsWith(suffix) ? `${body}${suffix}` : body;
  };
  const context: ConversationContext = {
    hidden: true,
    composer: (
      <div className="flex flex-wrap gap-1.5 text-xs text-fg-secondary" aria-label="Knowledge context">
        {page && <span className="max-w-full truncate rounded-md border border-border bg-surface px-2 py-1" title={page.path ?? page.title}>Viewing: {page.title}</span>}
        {attachments.map((a) => (
          <span key={a.key} className="inline-flex max-w-full items-center gap-1 rounded-md border border-border bg-surface px-2 py-1" title={a.quote ?? a.path ?? a.title}>
            <Icon name={a.quote === undefined ? "wiki" : "msg"} size={12} />
            <span className="truncate">{a.title}{a.quote === undefined ? "" : " · selection"}</span>
            <button aria-label={`Remove ${a.quote === undefined ? a.title : `selection from ${a.title}`}`} onClick={() => remove([a.key])} className="grid size-4 flex-none place-items-center rounded hover:bg-hover"><Icon name="x" size={11} /></button>
          </span>
        ))}
      </div>
    ),
    prepare: async (input) => {
      const draft = page && readDraft(page.id);
      const unsaved = page && draft !== undefined && draft.trim() !== (page.body_md ?? "").trim()
        ? [`Current editor text of ${label(page)} (includes unsaved edits; use this over the saved page):\n${quote(draft)}`] : [];
      const all = [...(input.context_refs ?? [])];
      for (const ref of refs) if (!all.some((r) => refKey(r) === refKey(ref))) all.push(ref);
      return { ...input, body: contextBody(input.body, unsaved), ...(all.length ? { context_refs: all } : {}) };
    },
    prepareText: async (body) => {
      // These endpoints accept text only. Freeze their page context when queued, before navigation or edits.
      const snapshots = await Promise.all(refs.map(async (r) => {
        const draft = readDraft(r.id!);
        const p = await fetchPage(r.id!);
        const text = draft ?? p.body_md ?? "";
        return `Page snapshot of ${label(p)}:\n${quote(text)}`;
      }));
      return contextBody(body, snapshots);
    },
    onSent: () => remove(attachments.map((a) => a.key)),
  };

  if (loadingPage) return <div className="p-4"><Skeleton className="h-4 w-2/3" /></div>;
  if (id) return (
    <div ref={host} className="flex min-h-0 flex-1 flex-col">
      {attach.error && <p role="alert" className="p-2 text-xs text-state-blocked">Couldn’t link this chat to the page. <button className="underline" onClick={() => attach.mutate(id)}>Retry</button></p>}
      <Conversation key={id} id={id} onNew={() => router.push("/chat")} onReplace={selectChat} context={context} />
    </div>
  );
  return (
    <div className="flex flex-1 flex-col items-center justify-center gap-3 p-6 text-center">
      <Icon name="msg" size={22} className="text-fg-tertiary" />
      {context.composer}
      <p className="text-[13px] text-fg-secondary">
        {page ? "Talk to a persona about this page. It can read it, link it and edit it with you." : "Talk to a persona about your notes. It can search, link and write pages with you."}
      </p>
      <Button variant="primary" disabled={create.isPending} onClick={start}>
        {page ? "Start a chat about this page" : "Start a chat"}
      </Button>
      {create.error && <p className="text-xs text-state-blocked">{create.error.message}</p>}
    </div>
  );
}

"use client";

import { useRouter } from "next/navigation";
import { useCallback, useEffect, useId, useRef, useState } from "react";
import { Conversation } from "@/app/(app)/chat/_components/conversation";
import { PersonaAvatar, personaLook } from "@/app/(app)/chat/_components/persona";
import { Dock, type DockTab } from "@/components/shell/editor-layout";
import { Button } from "@/components/ui/button";
import { Icon } from "@/components/ui/icon";
import { Skeleton } from "@/components/ui/primitives";
import { isTyping } from "@/components/shell/shortcuts";
import { useChat, useCreateChat } from "@/lib/api/hooks";
import { useAttachChat, usePage, type Page } from "@/lib/api/knowledge-hooks";
import { dockReducer, dockShortcut, loadDock, saveDock, type DockAction, type DockState } from "@/lib/editor-layout";

const APP = "knowledge";
const TABS = ["chat"];
/** The chat for screens that aren't a page (Home, Graph, Connect). A page's own chat is the one it's attached to. */
const GENERAL_CHAT_KEY = "vesper.knowledge.chat";

/**
 * The persona chat on the right of every Knowledge screen, on the shared dock: it folds to a rail and starts
 * folded. On a page it is that page's chat; elsewhere one general Knowledge chat.
 */
export function KnowledgeDock({ pageId }: { pageId?: string }) {
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
  if (loadingPage) return <div className="p-4"><Skeleton className="h-4 w-2/3" /></div>;
  if (chatId) return <Conversation key={chatId} id={chatId} onNew={() => router.push("/chat")} />;
  const start = () => {
    const body = page
      ? `I'm working on the Knowledge page [[${page.path ?? page.title}]]. Read it with read_page and help me with it.`
      : "I'm in Knowledge, my notes and documents. Help me find, connect and write pages: search_knowledge before answering.";
    create.mutate(
      { title: page?.title ?? "Knowledge", hidden: true, message: { body } },
      {
        onSuccess: (c) => {
          if (page) attach.mutate(c.id);
          else onGeneral(c.id);
        },
      },
    );
  };
  return (
    <div className="flex flex-1 flex-col items-center justify-center gap-3 p-6 text-center">
      <Icon name="msg" size={22} className="text-fg-tertiary" />
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

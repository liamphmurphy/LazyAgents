"use client";

import { createContext, useCallback, useContext, useMemo, useRef, useState, type ReactNode } from "react";

export type PageContext = { id: string; title: string; path: string | null };
export type PageAttachment = PageContext & { key: string; quote?: string };

function useContextState() {
  const [attachments, setAttachments] = useState<PageAttachment[]>([]);
  const [request, setRequest] = useState(0);
  const handled = useRef(0);
  const drafts = useRef(new Map<string, () => string | undefined>());
  const ask = useCallback((page: PageContext, quote?: string) => {
    const attachment = { id: page.id, title: page.title, path: page.path, quote, key: crypto.randomUUID() };
    setAttachments((old) => old.some((a) => a.id === page.id && a.quote === quote) ? old : [...old, attachment]);
    setRequest((n) => n + 1);
  }, []);
  const remove = useCallback((keys: string[]) => setAttachments((old) => old.filter((a) => !keys.includes(a.key))), []);
  const registerDraft = useCallback((id: string, read: () => string | undefined) => {
    drafts.current.set(id, read);
    return () => { if (drafts.current.get(id) === read) drafts.current.delete(id); };
  }, []);
  const readDraft = useCallback((id: string) => drafts.current.get(id)?.(), []);
  const claimRequest = useCallback((n: number) => {
    if (n <= handled.current) return false;
    handled.current = n;
    return true;
  }, []);
  return useMemo(() => ({ attachments, request, ask, remove, registerDraft, readDraft, claimRequest }), [attachments, request, ask, remove, registerDraft, readDraft, claimRequest]);
}

const Context = createContext<ReturnType<typeof useContextState> | null>(null);

export function KnowledgeChatProvider({ children }: { children: ReactNode }) {
  return <Context.Provider value={useContextState()}>{children}</Context.Provider>;
}

export function useKnowledgeChat() {
  const value = useContext(Context);
  if (!value) throw new Error("useKnowledgeChat outside KnowledgeChatProvider");
  return value;
}

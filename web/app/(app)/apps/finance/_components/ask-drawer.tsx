"use client";

import { useState, type FormEvent } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api, unwrap } from "@/lib/api/client";
import { chatKeys } from "@/lib/api/hooks";
import { useSpaceChats } from "@/lib/api/space-hooks";
import { AppDrawer } from "@/components/domain/app-drawer";
import { Button } from "@/components/ui/button";
import { Icon } from "@/components/ui/icon";
import { Skeleton } from "@/components/ui/primitives";
import { Conversation } from "@/app/(app)/chat/_components/conversation";

const errorText = (e: unknown) => (e instanceof Error ? e.message : "Something went wrong. Please try again.");

export const ASK_SUGGESTIONS = ["Why was last month higher than usual?", "What subscriptions could I cut?", "Am I on track to save more this year?", "Where does most of my money go?"];

/**
 * The Ask drawer's conversation: the Finance space's newest chat (so history and the space's
 * memory carry over), until the person starts a new one. ask() sends to it, or starts it.
 */
export function useFinanceAsk(spaceId: string | undefined) {
  const chats = useSpaceChats(spaceId);
  const qc = useQueryClient();
  // undefined follows the newest chat; null is a fresh start with no chat yet.
  const [own, setOwn] = useState<string | null | undefined>(undefined);
  const chatId = own === undefined ? chats.data?.chats[0]?.id : (own ?? undefined);
  const send = useMutation({
    mutationFn: async (body: string) => {
      let id = chatId;
      if (!id) id = (await unwrap(api.POST("/chats", { body: { space_id: spaceId! } }))).id;
      await unwrap(api.POST("/chats/{id}/messages", { params: { path: { id } }, body: { body } }));
      return id;
    },
    onSuccess: (id) => setOwn(id),
    onSettled: () => void qc.invalidateQueries({ queryKey: chatKeys.all }),
  });
  return {
    chatId,
    loading: !!spaceId && chats.isLoading,
    ask: (q: string) => send.mutate(q),
    asking: send.isPending,
    error: send.error,
    startFresh: () => {
      send.reset();
      setOwn(null);
    },
  };
}

export function AskDrawer({ open, onOpenChange, ask }: { open: boolean; onOpenChange: (o: boolean) => void; ask: ReturnType<typeof useFinanceAsk> }) {
  return (
    <AppDrawer
      open={open}
      onOpenChange={onOpenChange}
      icon="dollar"
      title="Ask Finance"
      subtitle="Answers come from your imported transactions."
      actions={ask.chatId && <Button variant="secondary" onClick={ask.startFresh}>New chat</Button>}
      footer={<><Icon name="lock" size={12} />The finance tools only answer here. Other chats can&rsquo;t read your transactions.</>}
    >
      {ask.loading ? (
        <div aria-busy="true" className="flex flex-col gap-3 p-5">
          <Skeleton className="h-5 w-2/3" />
          <Skeleton className="h-14 w-full" />
        </div>
      ) : ask.chatId ? (
        <div className="flex min-h-0 flex-1 flex-col">
          <Conversation key={ask.chatId} id={ask.chatId} onNew={ask.startFresh} />
        </div>
      ) : (
        <Starter ask={ask} />
      )}
    </AppDrawer>
  );
}

/** Before the first question: suggestions, and a box to ask anything. */
function Starter({ ask }: { ask: ReturnType<typeof useFinanceAsk> }) {
  const [text, setText] = useState("");
  function submit(e: FormEvent) {
    e.preventDefault();
    const q = text.trim();
    if (q && !ask.asking) ask.ask(q);
  }
  return (
    <div className="flex min-h-0 flex-1 flex-col justify-end gap-3 overflow-y-auto p-4">
      <p className="text-[13px] text-fg-secondary">Ask about your spending, subscriptions, savings or a single transaction. Tell me a category is wrong and I&rsquo;ll fix it and remember the merchant.</p>
      <ul className="flex flex-col gap-[6px]" aria-label="Suggestions">
        {ASK_SUGGESTIONS.map((s, i) => (
          <li key={s} className="motion-rise" style={{ animationDelay: `${i * 50}ms` }}>
            <button
              type="button"
              disabled={ask.asking}
              onClick={() => ask.ask(s)}
              className="w-full rounded-lg border border-border-strong px-[10px] py-[7px] text-left text-[13px] text-fg-2 hover:border-border-hover hover:bg-hover disabled:opacity-50"
            >
              {s}
            </button>
          </li>
        ))}
      </ul>
      <form onSubmit={submit} className="flex gap-[6px] rounded-xl border border-border-strong bg-surface p-1 pl-3 focus-within:border-accent">
        <input aria-label="Ask about your money" className="flex-1 bg-transparent text-[13px] text-fg outline-none placeholder:text-fg-tertiary" placeholder="Ask about your money…" value={text} disabled={ask.asking} onChange={(e) => setText(e.target.value)} />
        <Button type="submit" variant="primary" disabled={!text.trim() || ask.asking}>
          {ask.asking ? "Asking…" : "Ask"}
        </Button>
      </form>
      {ask.error && (
        <p role="alert" className="text-[12px] text-pill-blocked-fg">
          {errorText(ask.error)}
        </p>
      )}
    </div>
  );
}

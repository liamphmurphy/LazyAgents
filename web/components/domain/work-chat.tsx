"use client";

import { useCallback, useEffect, useState } from "react";
import { Conversation, type ConversationContext, type DockWelcome } from "@/app/(app)/chat/_components/conversation";
import { DockChatSwitcher } from "@/components/domain/dock-chat-switcher";
import { Button } from "@/components/ui/button";
import { Icon } from "@/components/ui/icon";
import { useAddDockChat, useDockChats, type DockScope } from "@/lib/api/dock-hooks";
import { ApiError, type Schemas } from "@/lib/api/client";
import { useChat, useCreateChat } from "@/lib/api/hooks";
import { withWorkContext, workSubjectLabel, type WorkSubject } from "@/lib/work-context";

/** One list of work chats, shared by every run and ticket, so the conversation follows you between them. */
const SCOPE: DockScope = "app:work";
/** The chat the panel opened last, in any run or ticket. */
const CURRENT_KEY = "vesper.work.chat";

const chip = "inline-flex h-6 max-w-full items-center gap-1.5 rounded-md border px-2 text-12";

/** Whom the next message goes to and what it carries: the persona, and the item in the main pane. */
function Chips({ persona, subject }: { persona?: string; subject: WorkSubject }) {
  return (
    <div className="flex flex-wrap items-center gap-1.5" aria-label="Message context" data-testid="work-chat-context">
      <span className="text-12 text-fg-tertiary">To</span>
      <span data-testid="work-chat-to" className={`${chip} border-accent/35 bg-accent-soft font-medium text-accent`}>{persona ?? "your persona"}</span>
      <span data-testid="work-chat-subject" className={`${chip} border-border bg-surface text-fg-secondary`} title={workSubjectLabel(subject)}>
        <Icon name={subject.kind === "run" ? "play" : "tickets"} size={12} className="flex-none text-fg-tertiary" />
        <span className="truncate">Viewing: {workSubjectLabel(subject)}</span>
      </span>
    </div>
  );
}

function welcomeFor(subject: WorkSubject): DockWelcome {
  const what = subject.kind === "run" ? "this run" : "this ticket";
  return {
    where: subject.kind === "run" ? "beside the run" : `beside ${subject.key}`,
    intro: `I can read ${what}, its ticket and its history, and talk it through with you. Replies to the agent go in the main pane.`,
    label: subject.kind === "run" ? "For this run" : "For this ticket",
    starters:
      subject.kind === "run"
        ? [
            { icon: "search", title: "What is it doing?", prompt: "Summarise what this run has done so far and what it's doing now." },
            { icon: "shield", title: "Is it safe to approve?", prompt: "Is it safe to approve what this run is asking for? What could go wrong?" },
            { icon: "check", title: "Check the criteria", prompt: "Which of this ticket's acceptance criteria has the run met, and which are still open?" },
          ]
        : [
            { icon: "search", title: "Where does it stand?", prompt: "Summarise where this ticket stands: its runs, what's done and what's left." },
            { icon: "edit", title: "Tighten the criteria", prompt: "Suggest clearer acceptance criteria for this ticket." },
          ],
  };
}

/**
 * The persona chat in a run's or ticket's side panel: the dock always talks to your persona, while
 * the main pane's composer talks to what's in it (the run's agent). One chat follows you between
 * items; every message says which item the person was looking at.
 */
export function WorkChat({ subject }: { subject: WorkSubject }) {
  const [current, setCurrent] = useState<string | null>(null);
  const [loaded, setLoaded] = useState(false);
  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect -- reading browser-only storage after hydration
    setCurrent(window.localStorage.getItem(CURRENT_KEY));
    setLoaded(true);
  }, []);
  const remember = useCallback((id: string) => {
    window.localStorage.setItem(CURRENT_KEY, id);
    setCurrent(id);
  }, []);

  // A remembered chat can be gone (deleted, or an id from another instance): treat it as none.
  const remembered = useChat(current ?? undefined);
  const gone = remembered.error instanceof ApiError && remembered.error.status === 404;
  const id = gone ? undefined : (current ?? undefined);
  const chat = gone ? undefined : remembered.data;

  const list = useDockChats(SCOPE);
  const add = useAddDockChat();
  const create = useCreateChat();
  const select = (next: string) => {
    remember(next);
    add.mutate({ scope: SCOPE, chatId: next });
  };
  const start = () => create.mutate({ title: "Work", hidden: true }, { onSuccess: (c) => select(c.id) });
  const chats = list.data?.chats ?? [];
  const switcher = (
    <DockChatSwitcher
      current={chats.find((c) => c.id === id)}
      sections={[{ label: "Work chats", chats }]}
      newLabel="New chat"
      onSelect={(next) => { if (next !== id) select(next); }}
      onNew={start}
      creating={create.isPending}
      inline={!!id}
    />
  );

  const persona = (chat?.speaker ?? chat?.persona)?.name;
  const context: ConversationContext = {
    hidden: true,
    composer: <Chips persona={persona} subject={subject} />,
    prepare: async (input: Schemas["NewChatMessage"]) => {
      const refs = [...(input.context_refs ?? [])];
      const key = subject.kind === "ticket" ? subject.key : subject.ticketKey;
      if (key && !refs.some((r) => r.kind === "ticket" && r.key === key)) refs.push({ kind: "ticket", key });
      return { ...input, body: withWorkContext(input.body, subject), ...(refs.length ? { context_refs: refs } : {}) };
    },
    prepareText: async (body) => withWorkContext(body, subject),
    onSent: () => {},
    welcome: welcomeFor(subject),
  };

  if (!loaded) return null;
  if (id)
    return (
      <div className="flex min-h-0 flex-1 flex-col" data-testid="work-chat">
        <Conversation key={id} id={id} onNew={start} onReplace={async (next) => select(next)} context={context} actions={switcher} />
      </div>
    );
  return (
    <div className="flex min-h-0 flex-1 flex-col" data-testid="work-chat">
      {chats.length > 0 && switcher}
      <div className="flex flex-1 flex-col items-center justify-center gap-3 p-6 text-center">
        <Icon name="msg" size={22} className="text-fg-tertiary" />
        <Chips persona={persona} subject={subject} />
        <p className="text-13 text-fg-secondary">
          Talk it through with a persona. It can read the {subject.kind} and its ticket; replies to the agent stay in the main pane.
        </p>
        <Button variant="primary" disabled={create.isPending} onClick={start}>Start a chat</Button>
        {create.error && <p role="alert" className="text-12 text-state-blocked">{create.error.message}</p>}
      </div>
    </div>
  );
}

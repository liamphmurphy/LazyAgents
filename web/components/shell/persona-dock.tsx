"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useEffect, useRef, useState, useSyncExternalStore } from "react";
import { PersonaAvatar } from "@/app/(app)/chat/_components/persona";
import type { Schemas } from "@/lib/api/client";
import { useMarkChatRead, usePersonaDock } from "@/lib/api/hooks";
import { DOCK_MAX_PERSONAS, DOCK_STATE_LABEL, dockEntries, dockSummary, viewedChatId } from "@/lib/persona-dock";

type Entry = Schemas["PersonaDockEntry"];
type Chat = Schemas["PersonaDockChat"];

/** Clears bottom bars and floating buttons (bulk bar, jump-to-latest). */
const DOCK_BOTTOM = 80;

function subscribeVisible(fn: () => void) {
  document.addEventListener("visibilitychange", fn);
  return () => document.removeEventListener("visibilitychange", fn);
}

/** Lifts the dock above a composer anchored to the bottom of the screen. Composers grow and mount late, so it re-measures. */
function useDockBottom(): number {
  const [bottom, setBottom] = useState(DOCK_BOTTOM);
  useEffect(() => {
    const measure = () => {
      const r = document.querySelector("[data-dock-avoid]")?.getBoundingClientRect();
      setBottom(r && r.bottom > window.innerHeight - 120 ? Math.max(DOCK_BOTTOM, Math.ceil(window.innerHeight - r.top + 12)) : DOCK_BOTTOM);
    };
    measure();
    const t = setInterval(measure, 500);
    window.addEventListener("resize", measure);
    return () => {
      clearInterval(t);
      window.removeEventListener("resize", measure);
    };
  }, []);
  return bottom;
}

function describe(e: Entry): string {
  return `${e.persona.name}: ${DOCK_STATE_LABEL[e.state]}${e.unread ? `, ${e.unread} unread` : ""}`;
}

function ChatRow({ chat, onPick }: { chat: Chat; onPick: () => void }) {
  return (
    <Link href={`/chat/${chat.id}`} onClick={onPick} className="block rounded-md px-2 py-1.5 text-[12.5px] hover:bg-surface-raised focus-visible:outline-2 focus-visible:outline-accent">
      <span className="flex items-baseline justify-between gap-2">
        <span className="truncate font-medium text-fg">{chat.title || "Untitled chat"}</span>
        <span className={`flex-none ${chat.state === "needs_you" ? "font-medium text-pill-human-fg" : "text-fg-2"}`}>{DOCK_STATE_LABEL[chat.state]}</span>
      </span>
      <span className="block truncate text-fg-2">{chat.state === "working" ? "Working on a reply…" : chat.last_line}</span>
    </Link>
  );
}

function PersonaButton({ entry, expanded, onClick }: { entry: Entry; expanded: boolean; onClick: () => void }) {
  return (
    <button type="button" aria-label={describe(entry)} aria-expanded={entry.chats.length > 1 ? expanded : undefined} title={describe(entry)} onClick={onClick}
      className={`relative rounded-full focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent ${entry.state === "working" ? "motion-safe:animate-pulse" : ""}`}>
      <PersonaAvatar persona={entry.persona} size={36} />
      {entry.state === "needs_you" ? (
        <span aria-hidden className="absolute -right-1 -top-1 grid size-4 place-items-center rounded-full bg-pill-human-fg text-[10px] font-semibold text-bg">!</span>
      ) : entry.state === "replied" ? (
        <span aria-hidden className="absolute -right-1 -top-1 grid h-4 min-w-4 place-items-center rounded-full bg-fg px-0.5 text-[10px] font-semibold text-bg">{entry.unread}</span>
      ) : null}
    </button>
  );
}

/** Which personas are working, replied or need the person, on every screen. Chat personas only. */
export function PersonaDock() {
  const router = useRouter();
  const pathname = usePathname();
  const dock = usePersonaDock();
  const mark = useMarkChatRead();
  const bottom = useDockBottom();
  const [open, setOpen] = useState<string | null>(null); // a persona id, or "all" for the narrow badge
  const root = useRef<HTMLDivElement>(null);
  const visible = useSyncExternalStore(subscribeVisible, () => document.visibilityState === "visible", () => false);
  const viewed = visible ? viewedChatId(pathname) : null;
  const entries = dockEntries<Chat, Entry>(dock.data?.personas ?? [], viewed);
  const shown = entries.slice(0, DOCK_MAX_PERSONAS);

  // Opening a chat reads it, and so does a reply arriving while it's on screen.
  const unreadViewed = dock.data?.personas.flatMap((e) => e.chats).find((c) => c.id === viewed && c.unread > 0);
  const { mutate } = mark;
  const seq = unreadViewed?.last_seq;
  useEffect(() => {
    if (viewed && seq) mutate({ id: viewed, seq });
  }, [viewed, seq, mutate]);

  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && setOpen(null);
    const onDown = (e: MouseEvent) => {
      if (!root.current?.contains(e.target as Node)) setOpen(null);
    };
    document.addEventListener("keydown", onKey);
    document.addEventListener("mousedown", onDown);
    return () => {
      document.removeEventListener("keydown", onKey);
      document.removeEventListener("mousedown", onDown);
    };
  }, [open]);

  if (shown.length === 0) return null;
  const pick = (e: Entry) => {
    if (e.chats.length === 1) {
      setOpen(null);
      router.push(`/chat/${e.chats[0].id}`);
    } else setOpen(open === e.persona.id ? null : e.persona.id);
  };
  const openEntries = open === "all" ? shown : shown.filter((e) => e.persona.id === open);
  return (
    <div ref={root} role="region" aria-label="Persona activity" style={{ bottom }} className="fixed right-4 z-30 flex flex-col items-end gap-2">
      {openEntries.length > 0 && (
        <div className="max-h-80 w-72 overflow-y-auto rounded-lg border border-border-float bg-surface-raised p-1.5 shadow-lg">
          {openEntries.map((e) => (
            <div key={e.persona.id}>
              <p className="px-2 py-1 text-[13px] font-medium text-fg">{e.persona.name}</p>
              {e.chats.map((c) => <ChatRow key={c.id} chat={c} onPick={() => setOpen(null)} />)}
            </div>
          ))}
        </div>
      )}
      <div className="hidden gap-2 rounded-full border border-border-float bg-surface-raised p-1.5 shadow-lg min-[800px]:flex">
        {shown.map((e) => <PersonaButton key={e.persona.id} entry={e} expanded={open === e.persona.id} onClick={() => pick(e)} />)}
      </div>
      <button type="button" aria-expanded={open === "all"} onClick={() => setOpen(open === "all" ? null : "all")}
        className="rounded-full border border-border-float bg-surface-raised px-3 py-2 text-[13px] font-medium text-fg shadow-lg focus-visible:outline-2 focus-visible:outline-accent min-[800px]:hidden">
        {dockSummary(entries)}
      </button>
    </div>
  );
}

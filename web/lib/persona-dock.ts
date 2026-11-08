// Pure helpers for the persona dock (LAZYAGENTS-90). No React or fetch here so node --test can cover them.

export type DockState = "working" | "replied" | "needs_you";
export type DockChat = { id: string; title: string; state: DockState; unread: number; waiting_approval: boolean; last_seq: number; last_line: string };
export type DockEntry<C extends DockChat = DockChat> = { state: DockState; unread: number; chats: C[] };

/** The dock shows this many personas: the most recent ones to ping. */
export const DOCK_MAX_PERSONAS = 3;

export const DOCK_STATE_LABEL: Record<DockState, string> = { working: "Working", replied: "Replied", needs_you: "Needs you" };

const RANK: Record<DockState, number> = { working: 0, replied: 1, needs_you: 2 };

/** The chat id in an app path, for /chat/<id>. */
export function viewedChatId(pathname: string): string | null {
  return /^\/chat\/([^/]+)/.exec(pathname)?.[1] ?? null;
}

/** The chat on screen never counts as unread. It stays only for what reading can't clear. */
function withoutViewed<C extends DockChat, E extends DockEntry<C>>(entry: E, viewed: string | null): E | null {
  const chats = entry.chats.flatMap((c) => {
    if (c.id !== viewed) return [c];
    if (c.waiting_approval) return [{ ...c, state: "needs_you" as const, unread: 0 }];
    return c.state === "working" ? [{ ...c, unread: 0 }] : [];
  });
  if (chats.length === 0) return null;
  const state = chats.reduce<DockState>((s, c) => (RANK[c.state] > RANK[s] ? c.state : s), "working");
  return { ...entry, chats, state, unread: chats.reduce((n, c) => n + c.unread, 0) };
}

/** Applies the viewed chat. Input is newest first with each persona once, as the server sends it. */
export function dockEntries<C extends DockChat, E extends DockEntry<C>>(entries: E[], viewed: string | null): E[] {
  return entries.flatMap((e) => withoutViewed<C, E>(e, viewed) ?? []);
}

/** The narrow-screen badge's text, covering every active persona. */
export function dockSummary(entries: DockEntry[]): string {
  const needs = entries.filter((e) => e.state === "needs_you").length;
  const unread = entries.reduce((n, e) => n + e.unread, 0);
  const working = entries.filter((e) => e.state === "working").length;
  const parts = [needs ? `${needs} needs you` : "", unread ? `${unread} unread` : "", working ? `${working} working` : ""].filter(Boolean);
  return parts.join(" · ");
}

// Pure helpers for the activity orb: the one floating element showing which
// harnesses are working, how much plan allowance is left, and which personas
// finished. No React or fetch here so node --test can cover them.

import type { DockState } from "./persona-dock.ts";

export type UsageWindow = { id: string; kind: "session" | "weekly" | "other"; label: string; used_percent: number; resets_at?: string };
export type HarnessActivity = {
  harness: string;
  label: string;
  running: number;
  limited_until?: string;
  plan?: string;
  usage_available: boolean;
  usage_checked_at?: string;
  windows: UsageWindow[];
};

export const CLAUDE = "claude-code";
export const CODEX = "codex";

/** Running and not out of allowance: a limited harness's runs have moved elsewhere. */
export function harnessWorking(h: HarnessActivity | undefined): boolean {
  return !!h && h.running > 0 && !h.limited_until;
}

export type Glyph = "both" | "claude" | "codex" | "idle";

/** The orb's centre: Claude's robot, Codex's comets, both, or an idle dot. */
export function orbGlyph(harnesses: HarnessActivity[]): Glyph {
  const a = harnessWorking(harnesses.find((h) => h.harness === CLAUDE));
  const b = harnessWorking(harnesses.find((h) => h.harness === CODEX));
  return a && b ? "both" : a ? "claude" : b ? "codex" : "idle";
}

/** The session window's used percentage for a ring: 100 while limited, null when unread. */
export function sessionPercent(h: HarnessActivity | undefined): number | null {
  if (!h) return null;
  if (h.limited_until) return 100;
  if (!h.usage_available) return null;
  const w = h.windows.find((x) => x.kind === "session") ?? h.windows[0];
  return w ? Math.round(w.used_percent) : null;
}

export function weeklyWindow(h: HarnessActivity): UsageWindow | undefined {
  return h.windows.find((w) => w.kind === "weekly");
}

type BadgeEntry = { state: DockState; unread: number };
export type Badge = { kind: "needs" } | { kind: "unread"; count: number } | null;

/** Amber "!" when a persona needs the person, otherwise the unread count. */
export function orbBadge(entries: BadgeEntry[]): Badge {
  if (entries.some((e) => e.state === "needs_you")) return { kind: "needs" };
  const count = entries.reduce((n, e) => n + e.unread, 0);
  return count > 0 ? { kind: "unread", count } : null;
}

/** Dimmed when there's nothing to look at. */
export function orbQuiet(harnesses: HarnessActivity[], entries: BadgeEntry[]): boolean {
  return orbGlyph(harnesses) === "idle" && !entries.some((e) => e.state === "working") && orbBadge(entries) === null && !harnesses.some((h) => h.limited_until);
}

export function clockTime(iso: string): string {
  return new Date(iso).toLocaleTimeString([], { hour: "numeric", minute: "2-digit" });
}

/** "9:20 PM" today, otherwise "Thu 8:00 AM". */
export function resetLabel(iso: string, now: Date = new Date()): string {
  const d = new Date(iso);
  const sameDay = d.toDateString() === now.toDateString();
  return sameDay ? clockTime(iso) : `${d.toLocaleDateString([], { weekday: "short" })} ${clockTime(iso)}`;
}

/** The line under a harness's usage bar. */
export function usageSub(h: HarnessActivity, all: HarnessActivity[], now: Date = new Date()): string {
  if (h.limited_until) {
    const other = all.find((x) => x.harness !== h.harness && !x.limited_until);
    return `Limited until ${resetLabel(h.limited_until, now)}${other ? ` · falls back to ${other.label}` : ""}`;
  }
  if (!h.usage_available) return h.usage_checked_at ? "No plan limits to show (API key or signed out)" : "Not read yet";
  const session = h.windows.find((w) => w.kind === "session");
  const weekly = weeklyWindow(h);
  const parts = [session ? (session.resets_at ? `5h · resets ${resetLabel(session.resets_at, now)}` : "5h") : "", weekly ? `weekly ${Math.round(weekly.used_percent)}%` : ""];
  return parts.filter(Boolean).join(" · ");
}

/** "checked 40s ago", from the newest reading. */
export function checkedAgo(harnesses: HarnessActivity[], now: number = Date.now()): string {
  const at = harnesses.map((h) => (h.usage_checked_at ? Date.parse(h.usage_checked_at) : 0)).reduce((a, b) => Math.max(a, b), 0);
  if (!at) return "not checked yet";
  const s = Math.max(0, Math.round((now - at) / 1000));
  if (s < 60) return `checked ${s}s ago`;
  const m = Math.round(s / 60);
  return m < 60 ? `checked ${m}m ago` : `checked ${Math.round(m / 60)}h ago`;
}

type Chat = { id: string; state: DockState; unread: number };
type Entry<C extends Chat> = { persona: { id?: string }; chats: C[] };
export type Finish<C extends Chat, E extends Entry<C>> = { entry: E; chat: C; kind: "done" | "needs" };

/** Each chat's state, to compare polls. */
export function chatStates<C extends Chat>(entries: Entry<C>[]): Map<string, DockState> {
  return new Map(entries.flatMap((e) => e.chats.map((c) => [c.id, c.state] as const)));
}

/**
 * Chats that finished between two polls: a working chat that replied, or any
 * chat that now needs the person. A chat that wasn't on the dock and arrives
 * with a reply finished too fast to be seen working. `before` is null on the
 * first poll, which never announces anything.
 */
export function finishes<C extends Chat, E extends Entry<C>>(before: Map<string, DockState> | null, entries: E[]): Finish<C, E>[] {
  if (!before) return [];
  const out: Finish<C, E>[] = [];
  for (const entry of entries) {
    for (const chat of entry.chats) {
      const was = before.get(chat.id);
      if (chat.state === "needs_you" && was !== "needs_you") out.push({ entry, chat, kind: "needs" });
      else if (chat.state === "replied" && (was === "working" || (was === undefined && chat.unread > 0))) out.push({ entry, chat, kind: "done" });
    }
  }
  return out;
}

/** The orb's accessible name. */
export function orbLabel(names: string[], entries: BadgeEntry[]): string {
  const needs = entries.filter((e) => e.state === "needs_you").length;
  const unread = entries.reduce((n, e) => n + e.unread, 0);
  return `Activity: ${names.length ? `${names.join(", ")} working` : "nothing running"}${needs ? `, ${needs} need you` : ""}${unread ? `, ${unread} unread` : ""}`;
}

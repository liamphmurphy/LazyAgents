/**
 * The compact shell's bottom tab bar (DESIGN.md §3 Responsive): which tabs it
 * has, which one a path belongs to, and which screens hide it. Pure so the
 * rules are unit-tested; the bar itself is components/shell/tab-bar.tsx.
 */
import type { IconName } from "@/components/ui/icon";
import { isActive, type NavBadge } from "./project-scope.ts";

export type Tab = { key: "chat" | "inbox" | "runs" | "tickets" | "more"; href: string; label: string; icon: IconName; badge?: NavBadge };

export const TABS: Tab[] = [
  { key: "chat", href: "/chat", label: "Chat", icon: "msg" },
  { key: "inbox", href: "/inbox", label: "Inbox", icon: "inbox", badge: "inbox" },
  { key: "runs", href: "/runs", label: "Runs", icon: "play", badge: "runs" },
  { key: "tickets", href: "/tickets", label: "Tickets", icon: "tickets" },
  { key: "more", href: "/more", label: "More", icon: "dots" },
];

/** The tab a path is under. Pages reached from More (apps, tools, settings) keep More current. */
export function currentTab(pathname: string): Tab["key"] {
  if (pathname === "/") return "chat";
  return TABS.find((t) => t.key !== "more" && isActive(pathname, t.href))?.key ?? "more";
}

/** Detail screens (a run, a ticket, the ticket composer) take the whole screen and have their own way back. */
export function hidesTabBar(pathname: string): boolean {
  return /^\/(runs|tickets)\/[^/]+/.test(pathname);
}

const BADGE_WORD: Partial<Record<NavBadge, string>> = { inbox: "unread", runs: "running" };

/** A badge shown only as a dot or number has to reach screen readers through the link's name. */
export function badgeLabel(label: string, badge: NavBadge | undefined, counts: Record<NavBadge, number>): string {
  const n = badge ? counts[badge] : 0;
  const word = badge && BADGE_WORD[badge];
  return n > 0 && word ? `${label} ${n} ${word}` : label;
}

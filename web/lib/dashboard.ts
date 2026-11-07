/**
 * Dashboard (LAZYAGENTS-47): which runs, chats, blockers and epics each panel
 * shows. Pure so the rules are unit tested without a browser.
 */
import { groupItems } from "./inbox.ts";

type RunLike = { id: string; purpose: string; state: string; plan: ReadonlyArray<{ position: number; text: string; state: string }> };
type CardLike = { key: string; status: string; unmet_dependencies: number; priority: string; updated_at: string };
type EpicLike = { status: string; health: string; points_total: number; points_done: number; tickets_open: number };
type InboxLike = { id: string; kind: string; created_at: string; resolved: boolean; read: boolean };

/**
 * Live runs: everything in flight except chat runs waiting between turns. The
 * chat ADR says a chat parked for the person's next message is not live work,
 * so it must not inflate the count; a chat run that is responding still is.
 */
export function liveRuns<T extends RunLike>(runs: readonly T[]): T[] {
  return runs.filter((r) => !(r.purpose === "chat" && r.state === "waiting"));
}

/** "Implement API · step 2 of 4"; plan positions are 1-based. */
export function currentStep(plan: RunLike["plan"]): { text: string; position: number; total: number } | undefined {
  if (!plan.length) return undefined;
  const step = plan.find((s) => s.state === "current") ?? plan.find((s) => s.state === "needs_human" || s.state === "failed");
  return step && { text: step.text, position: step.position, total: plan.length };
}

/** The top of the Inbox, in its canonical urgency order (most urgent group, longest wait first). */
export function topInbox<T extends InboxLike>(items: readonly T[], n = 5): { items: T[]; open: number } {
  const open = groupItems(items, "open", "oldest").flatMap((g) => g.items);
  return { items: open.slice(0, n), open: open.length };
}

const PRIORITY_RANK: Record<string, number> = { urgent: 0, high: 1, medium: 2, low: 3, none: 4 };

/** Open tickets with unfinished blockers, most urgent first, then most recently changed. */
export function blockedTickets<T extends CardLike>(cards: readonly T[]): T[] {
  return cards
    .filter((c) => c.unmet_dependencies > 0 && c.status !== "done" && c.status !== "canceled")
    .sort((a, b) => (PRIORITY_RANK[a.priority] ?? 9) - (PRIORITY_RANK[b.priority] ?? 9) || b.updated_at.localeCompare(a.updated_at) || a.key.localeCompare(b.key));
}

/** Open epics flagged at risk or off track, off track first. */
export function epicsAtRisk<T extends EpicLike>(epics: readonly T[]): T[] {
  return epics
    .filter((e) => e.status === "open" && (e.health === "at_risk" || e.health === "off_track"))
    .sort((a, b) => (a.health === b.health ? 0 : a.health === "off_track" ? -1 : 1));
}

/**
 * Tickets done per UTC day over the last `days` days ending today (UTC),
 * zero-filled. The server's daily counts already exclude canceled tickets.
 */
export function throughput(daily: ReadonlyArray<{ date: string; count: number }>, now: Date, days = 14): Array<{ date: string; count: number }> {
  const counts = new Map(daily.map((d) => [d.date, d.count]));
  const today = Date.UTC(now.getUTCFullYear(), now.getUTCMonth(), now.getUTCDate());
  return Array.from({ length: days }, (_, i) => {
    const date = new Date(today - (days - 1 - i) * 86_400_000).toISOString().slice(0, 10);
    return { date, count: counts.get(date) ?? 0 };
  });
}

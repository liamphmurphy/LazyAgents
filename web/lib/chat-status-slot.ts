/**
 * The chat's status slot: the banners and trays above the composer share one
 * place that shows the most important item in full and the rest behind "+N more".
 */

/**
 * What goes first, highest first. Ordered by what the person must do before
 * anything else can happen:
 * 1. blocking: nothing can be sent until it is settled (a required commit-email pick, an unavailable model).
 * 2. needs-you: work elsewhere is waiting on the person (a ticket run waiting on them, a workflow gate or Start card).
 * 3. failed-turn: the last reply failed, and Retry is the obvious next step.
 * 4. error: a send or action just failed; it is also announced, so it can sit lower.
 * 5. live: things going on their own (running ticket runs and workflows, queued follow-ups); nothing to do yet.
 */
export const STATUS_PRIORITY = ["blocking", "needs-you", "failed-turn", "error", "live"] as const;

export type StatusKind = (typeof STATUS_PRIORITY)[number];

export interface StatusEntry {
  key: string;
  kind: StatusKind;
  /** The person is using it (an open picker, focus inside it): it never collapses out from under them. */
  pinned?: boolean;
}

/** Items in priority order; items of the same kind keep the order they were given. */
export function orderStatus<T extends StatusEntry>(items: T[]): T[] {
  return items
    .map((item, i) => ({ item, i }))
    .sort((a, b) => STATUS_PRIORITY.indexOf(a.item.kind) - STATUS_PRIORITY.indexOf(b.item.kind) || a.i - b.i)
    .map(({ item }) => item);
}

/**
 * Which items show in full. Collapsed: the top item, anything that blocks
 * sending, and anything pinned. The rest are behind "+N more" (hidden).
 */
export function splitStatus<T extends StatusEntry>(items: T[], expanded: boolean): { shown: T[]; hidden: T[] } {
  const ordered = orderStatus(items);
  if (expanded) return { shown: ordered, hidden: [] };
  const shown: T[] = [];
  const hidden: T[] = [];
  ordered.forEach((item, i) => (i === 0 || item.kind === "blocking" || item.pinned ? shown : hidden).push(item));
  return { shown, hidden };
}

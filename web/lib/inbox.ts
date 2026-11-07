/**
 * Unified Inbox (LAZYAGENTS-46): grouping, ordering and focus rules, kept
 * pure so they are unit tested without a browser.
 */

export type InboxKind =
  | "approval"
  | "question"
  | "blocked"
  | "stalled"
  | "mockup"
  | "planning"
  | "review"
  | "dispatch"
  | "mention"
  | "suggestion";

type Item = { id: string; kind: string; created_at: string; resolved: boolean; read: boolean };

/** Groups in urgency order. Failed and stalled runs share one group. */
export const GROUPS: ReadonlyArray<{ key: string; label: string; kinds: readonly string[] }> = [
  { key: "approval", label: "Approvals", kinds: ["approval"] },
  { key: "question", label: "Questions", kinds: ["question"] },
  { key: "runs", label: "Failed & stalled runs", kinds: ["blocked", "stalled"] },
  { key: "mockup", label: "Mockups", kinds: ["mockup"] },
  { key: "planning", label: "Proposals", kinds: ["planning"] },
  { key: "review", label: "Reviews", kinds: ["review"] },
  { key: "dispatch", label: "Dispatch", kinds: ["dispatch"] },
  { key: "mention", label: "Mentions", kinds: ["mention"] },
  { key: "suggestion", label: "Wiki edits", kinds: ["suggestion"] },
];

/** Row pill label and tone: "blocked" means something broke, the rest need a person. */
export const KIND: Record<string, { label: string; tone: "human" | "blocked" | "info" }> = {
  approval: { label: "Approval", tone: "human" },
  question: { label: "Question", tone: "human" },
  blocked: { label: "Failed", tone: "blocked" },
  stalled: { label: "Stalled", tone: "blocked" },
  mockup: { label: "Mockup", tone: "human" },
  planning: { label: "Proposal", tone: "human" },
  review: { label: "Review", tone: "info" },
  dispatch: { label: "Dispatch", tone: "info" },
  mention: { label: "Mention", tone: "info" },
  suggestion: { label: "Wiki edit", tone: "info" },
};

export type View = "open" | "done";
export type Sort = "oldest" | "newest";

/** Open: still waiting and not marked done since it last changed. */
export function isOpen(i: Item): boolean {
  return !i.resolved && !i.read;
}

/** Visible groups (empty ones dropped), each sorted by wait time. */
export function groupItems<T extends Item>(items: readonly T[], view: View, sort: Sort) {
  const shown = items.filter((i) => (view === "open" ? isOpen(i) : !isOpen(i)));
  const dir = sort === "oldest" ? 1 : -1;
  return GROUPS.map((g) => ({
    ...g,
    items: shown
      .filter((i) => g.kinds.includes(i.kind))
      .sort((a, b) => dir * (Date.parse(a.created_at) - Date.parse(b.created_at)) || a.id.localeCompare(b.id)),
  })).filter((g) => g.items.length > 0);
}

/**
 * Which item to select after the list changes. The selection stays put while
 * it is listed; when it leaves (resolved, done), focus takes the same slot:
 * the next item, or the previous one when it was last.
 */
export function nextSelection(ids: readonly string[], selected: string | undefined, lastIndex: number): string | undefined {
  if (selected && ids.includes(selected)) return selected;
  if (!ids.length) return undefined;
  return ids[Math.min(Math.max(lastIndex, 0), ids.length - 1)];
}

/** Move by one with wrap-around, for j/k. */
export function step(ids: readonly string[], selected: string | undefined, by: 1 | -1): string | undefined {
  if (!ids.length) return undefined;
  const at = selected ? ids.indexOf(selected) : -1;
  if (at < 0) return ids[by === 1 ? 0 : ids.length - 1];
  return ids[(at + by + ids.length) % ids.length];
}

/** "42m", "1h 5m", "3d": how long the item has waited. */
export function waited(iso: string, now = Date.now()): string {
  const m = Math.max(0, Math.floor((now - Date.parse(iso)) / 60_000));
  if (m < 1) return "now";
  if (m < 60) return `${m}m`;
  const h = Math.floor(m / 60);
  if (h < 24) return m % 60 ? `${h}h ${m % 60}m` : `${h}h`;
  return `${Math.floor(h / 24)}d`;
}

/** Waits over an hour are shown in amber. */
export function longWait(iso: string, now = Date.now()): boolean {
  return now - Date.parse(iso) >= 3_600_000;
}

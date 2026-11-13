// What the menu shows on a space's row: which of its chats are working or
// waiting on the person, and where a click on the row's badge should go.

export type SpaceChat = {
  id: string;
  space_id: string;
  title: string;
  state: "working" | "replied" | "needs_you";
  persona?: { name: string };
  started_at?: string;
};

export type SpaceActivity = {
  chats: SpaceChat[];
  /** The most recently started working chat: where the jump button goes. */
  latestWorking?: SpaceChat;
  /** Someone is waiting on the person here, which outranks plain "working". */
  needsYou: boolean;
};

/** Groups the dock's space chats by space. The server sends them in the order the flyout lists them. */
export function groupSpaceActivity(chats: SpaceChat[] | undefined): Map<string, SpaceActivity> {
  const out = new Map<string, SpaceActivity>();
  for (const c of chats ?? []) {
    const a = out.get(c.space_id) ?? { chats: [], needsYou: false };
    a.chats.push(c);
    if (c.state === "needs_you") a.needsYou = true;
    if (c.state === "working" && !a.latestWorking) a.latestWorking = c;
    out.set(c.space_id, a);
  }
  return out;
}

/** One chat: the badge opens it directly. Several: the badge opens the flyout. */
export function badgeTarget(a: SpaceActivity): { kind: "chat"; chat: SpaceChat } | { kind: "flyout" } {
  return a.chats.length === 1 ? { kind: "chat", chat: a.chats[0] } : { kind: "flyout" };
}

/** The jump button only adds something when the badge opens a flyout and a chat is working. */
export function jumpTarget(a: SpaceActivity): SpaceChat | undefined {
  return a.chats.length > 1 ? a.latestWorking : undefined;
}

export function activityLabel(name: string, a: SpaceActivity): string {
  const working = a.chats.filter((c) => c.state === "working").length;
  const waiting = a.chats.length - working;
  const parts = [working ? `${working} working` : "", waiting ? `${waiting} waiting on you` : ""].filter(Boolean);
  return `${name}: ${parts.join(", ")}`;
}

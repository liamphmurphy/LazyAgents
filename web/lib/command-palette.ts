// Pure logic for the ⌘K command palette (LAZYAGENTS-43). Kept free of React so
// `npm run test:unit` covers it; the component is components/shell/command-menu.tsx.

export type PaletteGroup =
  | "Top result"
  | "Needs you"
  | "Running"
  | "Continue"
  | "Recent"
  | "Jump to"
  | "Tickets"
  | "Pages"
  | "Artifacts"
  | "Chats"
  | "Agents"
  | "Epics"
  | "Runs"
  | "Workflows"
  | "Personas"
  | "Memories"
  | "Library"
  | "Actions"
  | "Switch project"
  | "Go to";

/** Display order of groups; search hits are re-sorted into it. */
export const GROUP_ORDER: PaletteGroup[] = [
  "Top result",
  "Needs you",
  "Running",
  "Continue",
  "Recent",
  "Jump to",
  "Tickets",
  "Pages",
  "Artifacts",
  "Chats",
  "Agents",
  "Epics",
  "Runs",
  "Workflows",
  "Personas",
  "Memories",
  "Library",
  "Actions",
  "Switch project",
  "Go to",
];

/** A navigable result, as stored in the recent list. */
export type PaletteLink = {
  id: string;
  title: string;
  meta?: string;
  href: string;
  kind: string;
  status?: string;
};

type TargetLike = {
  kind: string;
  id?: string;
  title: string;
  artifact_type?: string;
  path?: string;
  display_id?: string;
  href?: string;
  status?: string;
};

/** A ticket key typed on its own ("lazyagents-39 " → "LAZYAGENTS-39"), or a bare number in `project`. */
export function ticketKeyFrom(query: string, project?: string): string | undefined {
  const text = query.trim();
  if (project && /^\d+$/.test(text)) return `${project}-${text}`;
  const m = text.match(/^([A-Za-z][A-Za-z0-9]*-\d+)$/);
  return m ? m[1].toUpperCase() : undefined;
}

const FILTER_GROUPS: Record<string, PaletteGroup> = {
  ticket: "Tickets",
  page: "Pages",
  artifact: "Artifacts",
  chat: "Chats",
  agent: "Agents",
  epic: "Epics",
  run: "Runs",
  workflow: "Workflows",
  persona: "Personas",
  memory: "Memories",
  library: "Library",
};

export type PaletteFilters = { text: string; groups: PaletteGroup[]; status?: string; tokens: string[] };

/** Splits `is:ticket status:done deploy` into filters and search text; unknown tokens stay text. */
export function parseFilters(query: string): PaletteFilters {
  const out: PaletteFilters = { text: "", groups: [], tokens: [] };
  const words: string[] = [];
  for (const word of query.trim().split(/\s+/).filter(Boolean)) {
    const lower = word.toLowerCase();
    const kind = lower.match(/^(?:is|in):(.+)$/)?.[1];
    const status = lower.match(/^status:(.+)$/)?.[1];
    if (kind && FILTER_GROUPS[kind]) {
      out.groups.push(FILTER_GROUPS[kind]);
      out.tokens.push(word);
    } else if (status) {
      out.status = status;
      out.tokens.push(word);
    } else words.push(word);
  }
  out.text = words.join(" ");
  return out;
}

/** Whether a hit passes the parsed filters. */
export function passesFilters(hit: { group: PaletteGroup; status?: string }, f: PaletteFilters): boolean {
  return (!f.groups.length || f.groups.includes(hit.group)) && (!f.status || hit.status === f.status);
}

/** The query with one filter token taken out. */
export function removeToken(query: string, token: string): string {
  return query.trim().split(/\s+/).filter((w) => w !== token).join(" ");
}

/** Cuts a server snippet («match») into plain and marked runs. */
export function snippetParts(snippet: string): { text: string; match: boolean }[] {
  return snippet
    .split(/(«[^»]*»)/)
    .filter(Boolean)
    .map((s) => (s.startsWith("«") && s.endsWith("»") ? { text: s.slice(1, -1), match: true } : { text: s, match: false }));
}

/** Cuts text around every query word, case-insensitively. */
export function matchParts(text: string, query: string): { text: string; match: boolean }[] {
  const words = query.split(/\s+/).filter(Boolean).map((w) => w.replace(/[.*+?^${}()|[\]\\]/g, "\\$&"));
  if (!words.length) return [{ text, match: false }];
  const re = new RegExp(`(${words.join("|")})`, "gi");
  return text.split(re).filter(Boolean).map((s) => ({ text: s, match: words.some((w) => new RegExp(`^${w}$`, "i").test(s)) }));
}

/** Lifts the best-scoring hit into "Top result" unless grouping already put it first. */
export function promoteTop<T extends { group: PaletteGroup; score?: number }>(sorted: T[]): T[] {
  let best = 0;
  sorted.forEach((item, i) => {
    if ((item.score ?? 0) > (sorted[best].score ?? 0)) best = i;
  });
  if (best === 0) return sorted;
  return [{ ...sorted[best], group: "Top result" }, ...sorted.filter((_, i) => i !== best)];
}

/** The palette group a search hit belongs to; wiki pages are notes. */
export function groupOf(t: TargetLike): PaletteGroup {
  switch (t.kind) {
    case "ticket":
      return "Tickets";
    case "chat":
      return "Chats";
    case "agent":
      return "Agents";
    case "epic":
      return "Epics";
    case "run":
      return "Runs";
    case "workflow":
      return "Workflows";
    case "persona":
      return "Personas";
    case "memory":
      return "Memories";
    case "library":
      return "Library";
    default:
      return t.artifact_type === "note" ? "Pages" : "Artifacts";
  }
}

/** A search hit as a palette link; undefined when it has nowhere to go. */
export function linkFromTarget(t: TargetLike): PaletteLink | undefined {
  if (!t.href) return undefined;
  const meta =
    t.kind === "chat"
      ? "chat"
      : t.kind === "artifact" && t.artifact_type === "note"
        ? "wiki page"
        : (t.display_id ?? t.artifact_type ?? t.kind);
  return {
    id: t.id ?? t.href,
    title: t.kind === "artifact" && t.artifact_type === "note" && t.path ? t.path : t.title,
    meta,
    href: t.href,
    kind: groupOf(t),
    ...(t.status ? { status: t.status } : {}),
  };
}

/** Stable sort into GROUP_ORDER, keeping the server's ranking inside a group. */
export function sortByGroup<T extends { group: PaletteGroup }>(items: T[]): T[] {
  return items
    .map((item, i) => ({ item, i }))
    .sort((a, b) => GROUP_ORDER.indexOf(a.item.group) - GROUP_ORDER.indexOf(b.item.group) || a.i - b.i)
    .map((x) => x.item);
}

/** Every word of the query appears in the text (case-insensitive); empty matches all. */
export function matchesWords(text: string, query: string): boolean {
  const hay = text.toLowerCase();
  return query
    .toLowerCase()
    .split(/\s+/)
    .filter(Boolean)
    .every((w) => hay.includes(w));
}

export const MAX_RECENT = 6;

/** Most recent first, deduped by href, capped. */
export function pushRecent(list: PaletteLink[], item: PaletteLink, max = MAX_RECENT): PaletteLink[] {
  return [item, ...list.filter((r) => r.href !== item.href)].slice(0, max);
}

/** Reads the stored recent list, dropping anything malformed. */
export function parseRecent(raw: string | null): PaletteLink[] {
  if (!raw) return [];
  try {
    const v: unknown = JSON.parse(raw);
    if (!Array.isArray(v)) return [];
    return v
      .filter(
        (r): r is PaletteLink =>
          !!r &&
          typeof r === "object" &&
          typeof (r as PaletteLink).href === "string" &&
          (r as PaletteLink).href.startsWith("/") &&
          typeof (r as PaletteLink).title === "string" &&
          typeof (r as PaletteLink).id === "string",
      )
      .slice(0, MAX_RECENT);
  } catch {
    return [];
  }
}

/** What the current page is about, for "New chat with … attached" and "Go to run". */
export type PageContext =
  | { kind: "ticket"; key: string }
  | { kind: "artifact"; id: string }
  | { kind: "wiki"; path: string }
  | undefined;

export function pageContext(pathname: string): PageContext {
  const ticket = pathname.match(/^\/tickets\/([A-Z][A-Z0-9]*-\d+)$/);
  if (ticket) return { kind: "ticket", key: ticket[1] };
  const artifact = pathname.match(/^\/artifacts\/([0-9a-f-]{36})$/i);
  if (artifact) return { kind: "artifact", id: artifact[1] };
  const wiki = pathname.match(/^\/wiki\/(.+)$/);
  if (wiki) {
    const path = wiki[1]
      .split("/")
      .map((s) => {
        try {
          return decodeURIComponent(s);
        } catch {
          return s;
        }
      })
      .join("/");
    return { kind: "wiki", path };
  }
  return undefined;
}

/** The project key a ticket key belongs to. */
export function projectOfKey(key: string): string {
  return key.replace(/-\d+$/, "");
}

/** `?` reference (keep in sync with the handlers in app-shell.tsx and ticket-screen.tsx). */
export const SHORTCUTS: { section: string; items: { label: string; keys: string[] }[] }[] = [
  {
    section: "Global",
    items: [
      { label: "Open command palette", keys: ["⌘/Ctrl", "K"] },
      { label: "New ticket", keys: ["C"] },
      { label: "Show this reference", keys: ["?"] },
    ],
  },
  {
    section: "Palette",
    items: [
      { label: "Move / open / close", keys: ["↑", "↓", "↵", "Esc"] },
      { label: "Toggle project scope · commands only", keys: ["Tab", ">"] },
      { label: "Open in a new tab", keys: ["⌘/Ctrl", "↵"] },
    ],
  },
  {
    section: "Ticket",
    items: [
      { label: "Next / previous ticket", keys: ["J", "K"] },
      { label: "Edit the ticket", keys: ["E"] },
      { label: "Save description / comment", keys: ["⌘/Ctrl", "↵"] },
    ],
  },
  {
    section: "Needs you tray",
    items: [
      { label: "Next / previous item", keys: ["J", "K"] },
      { label: "Open the item’s page", keys: ["↵"] },
      { label: "Switch between Needs you and Updates", keys: ["Tab"] },
      { label: "Approve the selected approval", keys: ["A"] },
      { label: "Deny the selected approval (with an optional reason)", keys: ["D"] },
      { label: "Close the tray, or leave the reply box", keys: ["Esc"] },
    ],
  },
  {
    section: "Inbox",
    items: [
      { label: "Next / previous item", keys: ["J", "K"] },
      { label: "Open the item’s page", keys: ["↵"] },
      { label: "Approve (approvals, mockups)", keys: ["A"] },
      { label: "Deny (approvals)", keys: ["D"] },
      { label: "Reply (questions) or focus the note", keys: ["R"] },
      { label: "Mark done: hide until it changes", keys: ["E"] },
      { label: "Send the reply or note", keys: ["⌘/Ctrl", "↵"] },
      { label: "Leave the text box, back to the list", keys: ["Esc"] },
    ],
  },
];

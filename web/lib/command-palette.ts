// Pure logic for the ⌘K command palette (LAZYAGENTS-43). Kept free of React so
// `npm run test:unit` covers it; the component is components/shell/command-menu.tsx.

export type PaletteGroup =
  | "Recent"
  | "Jump to"
  | "Tickets"
  | "Pages"
  | "Artifacts"
  | "Chats"
  | "Agents"
  | "Epics"
  | "Actions"
  | "Switch project"
  | "Go to";

/** Display order of groups; search hits are re-sorted into it. */
export const GROUP_ORDER: PaletteGroup[] = [
  "Recent",
  "Jump to",
  "Tickets",
  "Pages",
  "Artifacts",
  "Chats",
  "Agents",
  "Epics",
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
};

type TargetLike = {
  kind: string;
  id?: string;
  title: string;
  artifact_type?: string;
  path?: string;
  display_id?: string;
  href?: string;
};

/** A ticket key typed on its own ("lazyagents-39 " → "LAZYAGENTS-39"). */
export function ticketKeyFrom(query: string): string | undefined {
  const m = query.trim().match(/^([A-Za-z][A-Za-z0-9]*-\d+)$/);
  return m ? m[1].toUpperCase() : undefined;
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
    ],
  },
  {
    section: "Ticket",
    items: [
      { label: "Next / previous ticket", keys: ["J", "K"] },
      { label: "Save description / comment", keys: ["⌘/Ctrl", "↵"] },
    ],
  },
];

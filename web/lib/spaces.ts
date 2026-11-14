// Spaces: a person's groups of chats with shared instructions, sources and
// memory. Pure helpers for the menu, the space page and its dialogs.

export type SpaceColor = "green" | "amber" | "red" | "blue" | "violet" | "pink" | "teal" | "orange";

export const SPACE_COLORS: { value: SpaceColor; hex: string; label: string }[] = [
  { value: "green", hex: "#34D399", label: "Green" },
  { value: "amber", hex: "#FBBF24", label: "Amber" },
  { value: "red", hex: "#F87171", label: "Red" },
  { value: "blue", hex: "#60A5FA", label: "Blue" },
  { value: "violet", hex: "#A78BFA", label: "Violet" },
  { value: "pink", hex: "#F472B6", label: "Pink" },
  { value: "teal", hex: "#2DD4BF", label: "Teal" },
  { value: "orange", hex: "#FB923C", label: "Orange" },
];

export function spaceHex(color: string | undefined): string {
  return SPACE_COLORS.find((c) => c.value === color)?.hex ?? SPACE_COLORS[0].hex;
}

/** The tile's letter: the name's first letter or digit, upper-cased. */
export function spaceInitial(name: string): string {
  const m = name.match(/[\p{L}\p{N}]/u);
  return m ? m[0].toLocaleUpperCase() : "#";
}

/** Tokens as the sources card shows them: 950, 1.2k, 13k, 13.1k. */
export function formatTokens(n: number, precise = false): string {
  if (n < 1000) return String(n);
  const k = n / 1000;
  if (precise || k < 10) return `${(Math.round(k * 10) / 10).toString()}k`;
  return `${Math.round(k)}k`;
}

/** A file size as a source row shows it. */
export function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${Math.round(n / 1024)} KB`;
  return `${(Math.round((n / (1024 * 1024)) * 10) / 10).toString()} MB`;
}

export type SourceKind = "file" | "text" | "website" | "library" | "link" | "canvas" | "research";

export const SOURCE_KIND_LABEL: Record<SourceKind, string> = {
  file: "File",
  text: "Text",
  website: "Website",
  library: "Library",
  link: "Saved link",
  canvas: "Canvas",
  research: "Research",
};

/** A URL without its scheme, www. or trailing slash: examine.com/creatine. */
export function bareUrl(raw: string): string {
  try {
    const u = new URL(raw);
    return `${u.host.replace(/^www\./, "")}${u.pathname}`.replace(/\/$/, "");
  } catch {
    return raw;
  }
}

type SourceLike = { kind: string; title: string; url?: string; tokens: number; size_bytes: number };

/** The name a source row shows: a page by its address, everything else by title. */
export function sourceName(s: SourceLike): string {
  return s.kind === "website" && s.url ? bareUrl(s.url) : s.title;
}

/** "File · 2.1k tokens", or the size for a file that isn't text. */
export function sourceDetail(s: SourceLike): string {
  const kind = SOURCE_KIND_LABEL[s.kind as SourceKind] ?? "Source";
  return s.tokens > 0 ? `${kind} · ${formatTokens(s.tokens, true)} tokens` : `${kind} · ${formatBytes(s.size_bytes)}`;
}

type ChatLike = { created_at: string; last_message_at?: string };

export type ChatSort = "newest" | "oldest";

/** A space's chats by last activity, newest or oldest first. */
export function sortSpaceChats<T extends ChatLike>(chats: T[], sort: ChatSort): T[] {
  const at = (c: T) => Date.parse(c.last_message_at ?? c.created_at);
  return [...chats].sort((a, b) => (sort === "newest" ? at(b) - at(a) : at(a) - at(b)));
}

/** "Workouts, nutrition and recovery · 12 chats · 4 sources". */
export function spaceSubtitle(s: { description: string; chat_count: number; source_count: number }): string {
  const n = (k: number, one: string) => `${k} ${one}${k === 1 ? "" : "s"}`;
  return [s.description.trim(), n(s.chat_count, "chat"), n(s.source_count, "source")].filter(Boolean).join(" · ");
}

/** The composer's note under a space's new-chat box. */
export function spaceUses(s: { name: string; instructions: string; source_count: number; space_memory: boolean }): string {
  const parts: string[] = [];
  if (s.instructions.trim()) parts.push(`${s.name}’s instructions`);
  if (s.source_count > 0) parts.push(`${s.source_count} source${s.source_count === 1 ? "" : "s"}`);
  if (s.space_memory) parts.push("space memory");
  if (parts.length === 0) return `New chats here start in ${s.name}.`;
  const last = parts.pop();
  return `Uses ${parts.length ? `${parts.join(", ")} and ${last}` : last}`;
}

/** The newest few chats the menu nests under an open space. */
export const NESTED_CHATS = 3;

/** How many chats the space page lists before "Show N more". */
export const SPACE_PAGE_CHATS = 5;

// ---- Work (LAZYAGENTS-213): a space that tracks work has a key, tickets, epics, runs and a
// repository. The server checks the key; this mirrors its rule for the form.

/** 2–10 letters or digits, starting with a letter (upper-cased by the server). */
export const WORK_KEY = /^[A-Za-z][A-Za-z0-9]{1,9}$/;

/**
 * A key to suggest for a space's work: the initials of a several-word name ("Waterdeep
 * campaign" → "WC"), or a one-word name's first letters ("Homelab" → "HOME"). "" when the name
 * has nothing usable.
 */
export function suggestWorkKey(name: string): string {
  const words = name.normalize("NFKD").replace(/[^A-Za-z0-9\s]/g, " ").split(/\s+/).filter(Boolean);
  const first = words.findIndex((w) => /^[A-Za-z]/.test(w));
  if (first < 0) return "";
  const rest = words.slice(first);
  const key = (rest.length > 1 ? rest.map((w) => w[0]).join("") : rest[0].slice(0, 4)).toUpperCase().slice(0, 10);
  return key.length >= 2 ? key : rest[0].slice(0, 4).toUpperCase().padEnd(2, "X");
}

type Listable = { work?: { archived: boolean } };

/** The spaces the menus list: all but those whose work is archived. */
export function listedSpaces<T extends Listable>(spaces: T[]): T[] {
  return spaces.filter((s) => !s.work?.archived);
}

/** The spaces whose work a ticket, epic or run filter offers: those that track work. */
export function workSpaces<T extends Listable>(spaces: T[]): (T & { work: NonNullable<T["work"]> })[] {
  return spaces.filter((s): s is T & { work: NonNullable<T["work"]> } => !!s.work && !s.work.archived);
}

/**
 * Where to manage a space's work (its repository, archiving): that space's page; or, with no
 * project, a new space that tracks work. Replaces Settings › Projects.
 */
export function workSettingsHref(project?: { space_id?: string }): string {
  return project?.space_id ? `/spaces/${project.space_id}` : "/spaces/new?work=1";
}

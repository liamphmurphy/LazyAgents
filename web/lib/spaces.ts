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

export type SourceKind = "file" | "text" | "website" | "library" | "link" | "canvas";

export const SOURCE_KIND_LABEL: Record<SourceKind, string> = {
  file: "File",
  text: "Text",
  website: "Website",
  library: "Library",
  link: "Saved link",
  canvas: "Canvas",
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

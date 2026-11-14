// Starting a deck or design from its start page in Knowledge: whether what was typed is a title
// or a brief for the new item's persona to draft from, and an outline's title and slides.
// Pure, so it's unit-tested in lib/start.test.mjs.

const TICKET_KEY = /\b[A-Z][A-Z0-9]{1,15}-\d{1,7}\b/;

/** Text that reads like a request rather than a title starts the deck and asks its persona to draft it. */
export function isDeckBrief(s: string): boolean {
  const t = s.trim();
  return t.split(/\s+/).length > 8 || /^(make|create|draft|build|write|turn|a deck|slides)\b/i.test(t);
}

/** A request, or a ticket key, starts a blank design and asks its persona to draft it. */
export function isDesignBrief(s: string): boolean {
  const t = s.trim();
  return t.split(/\s+/).length > 6 || TICKET_KEY.test(t) || /^(make|create|draft|design|build|turn|sketch|map|diagram)\b/i.test(t);
}

/** A drafted design's title: the ticket key it names, else empty (Untitled until the persona names it). */
export const designBriefTitle = (s: string) => s.trim().match(TICKET_KEY)?.[0] ?? "";

export const MAX_OUTLINE = 4000;

/**
 * A deck from an outline: with no title typed, the outline's first line (without its bullet)
 * titles the deck and the rest become slides; with one, the whole outline does.
 */
export function splitOutline(outline: string, title: string): { title: string; slides: string } {
  if (title.trim()) return { title: title.trim(), slides: outline.trim() };
  const lines = outline.split("\n");
  const first = lines.findIndex((l) => l.trim());
  if (first < 0) return { title: "", slides: "" };
  return {
    title: lines[first].trim().replace(/^([-*•]|\d+[.)])\s+/, "").slice(0, 200),
    slides: lines.slice(first + 1).join("\n").trim(),
  };
}

// Designs Home (docs/design/designs-home.svg): filters, search and what each
// recent card says. Pure, so it's unit-tested.
import type { Design } from "./schema.ts";

export type Filter = "all" | "mine" | "persona" | "review";
export const FILTERS: { key: Filter; label: string }[] = [
  { key: "all", label: "All" },
  { key: "mine", label: "Mine" },
  { key: "persona", label: "Persona-made" },
  { key: "review", label: "Needs review" },
];

export type Summary = { id: string; title: string; persona_edited: boolean; pending_proposals: number; frames: number; pages: number };

export function filterDesigns<T extends Summary>(list: T[], filter: Filter, query: string): T[] {
  const q = query.trim().toLowerCase();
  return list.filter((d) => {
    if (filter === "mine" && d.persona_edited) return false;
    if (filter === "persona" && !d.persona_edited) return false;
    if (filter === "review" && d.pending_proposals === 0) return false;
    return !q || d.title.toLowerCase().includes(q);
  });
}

/** What the design is, from its body: the card's first detail. */
export function kindOf(d: Design | undefined, frames: number): string {
  if (!d) return frames ? `${frames} ${frames === 1 ? "frame" : "frames"}` : "Design";
  const nodes = d.pages.flatMap((p) => p.children);
  const stickies = nodes.filter((n) => n.type === "sticky").length;
  const diagram = nodes.some((n) => n.type === "section" || n.type === "connector");
  if (!frames && stickies > 1) return "Whiteboard";
  if (!frames && diagram) return "Diagram";
  return frames ? `${frames} ${frames === 1 ? "frame" : "frames"}` : "Empty canvas";
}

/** Tickets and paths the design's frames link to (the card's chips), first few. */
export function linksOf(d: Design | undefined, max = 2): string[] {
  if (!d) return [];
  const out = new Set<string>();
  for (const p of d.pages)
    for (const n of p.children)
      if (n.type === "frame" || n.type === "component") for (const t of [...(n.linked_tickets ?? []), ...(n.linked_paths ?? [])]) out.add(t);
  return [...out].slice(0, max);
}

/** A starting prompt reads as a request (or names a ticket) rather than a title: the persona drafts it. */
export const isBrief = (s: string) => {
  const t = s.trim();
  return t.split(/\s+/).length > 6 || /\b[A-Z][A-Z0-9]{1,15}-\d{1,7}\b/.test(t) || /^(make|create|draft|design|build|turn|sketch|map|diagram)\b/i.test(t);
};

/** The design's title for a brief: a ticket key if it names one, else empty (Untitled until the persona renames it). */
export const briefTitle = (s: string) => s.trim().match(/\b[A-Z][A-Z0-9]{1,15}-\d{1,7}\b/)?.[0] ?? "";

/** The Design card's line in a chat (docs/design/designs-from-chat.svg): "4 frames · 3 links · 1 sticky · Flows". */
export function designCounts(d: Design): string {
  const nodes = d.pages.flatMap((p) => p.children);
  const count = (n: number, one: string, many: string) => `${n} ${n === 1 ? one : many}`;
  const frames = nodes.filter((n) => n.type === "frame").length;
  const stickies = nodes.filter((n) => n.type === "sticky").length;
  const parts = [count(frames, "frame", "frames")];
  if (d.links.length) parts.push(count(d.links.length, "link", "links"));
  if (stickies) parts.push(count(stickies, "sticky", "stickies"));
  if (d.pages.length > 1) parts.push(count(d.pages.length, "page", "pages"));
  else if (d.pages[0]) parts.push(d.pages[0].name);
  return parts.join(" · ");
}

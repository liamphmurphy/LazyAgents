import { fromMarkdown } from "mdast-util-from-markdown";
import { gfmFromMarkdown } from "mdast-util-gfm";
import { gfm } from "micromark-extension-gfm";
import { diffLines } from "./diff.ts";

type Version = { seq: number; author_type: "user" | "agent" };

/**
 * The version a persona's changes are counted from: the person's own last
 * edit, or the last version they finished reviewing, whichever is newer.
 * There's nothing to review while that is the current version.
 */
export function reviewBase(versions: Version[], reviewedSeq = 0): number {
  const ownLast = Math.max(0, ...versions.filter((v) => v.author_type === "user").map((v) => v.seq));
  return Math.max(ownLast, reviewedSeq);
}

/**
 * A document's top-level blocks (paragraphs, headings, whole lists, tables,
 * code), each as its source. One change is one run of changed blocks, so a
 * rewritten section is a handful of decisions, not one per sentence.
 */
export function splitBlocks(md: string): string[] {
  const root = fromMarkdown(md, { extensions: [gfm()], mdastExtensions: [gfmFromMarkdown()] });
  return root.children.flatMap((n) => (n.position ? [md.slice(n.position.start.offset, n.position.end.offset)] : []));
}

export type Segment = { kind: "same"; blocks: string[] } | { kind: "change"; id: number; before: string[]; after: string[] };

/** The blocks both versions share, and the changes between them in order. */
export function reviewSegments(base: string, head: string): Segment[] {
  // diffLines compares lines; JSON keeps each block on one.
  const enc = (blocks: string[]) => blocks.map((b) => JSON.stringify(b)).join("\n");
  const out: Segment[] = [];
  let id = 0;
  for (const l of diffLines(enc(splitBlocks(base)), enc(splitBlocks(head)))) {
    const block = JSON.parse(l.text) as string;
    const last = out.at(-1);
    if (l.op === "same") {
      if (last?.kind === "same") last.blocks.push(block);
      else out.push({ kind: "same", blocks: [block] });
    } else {
      const change = last?.kind === "change" ? last : (out[out.push({ kind: "change", id: id++, before: [], after: [] }) - 1] as Extract<Segment, { kind: "change" }>);
      (l.op === "del" ? change.before : change.after).push(block);
    }
  }
  return out;
}

export type Decision = "accept" | "reject";

/**
 * The document once every change is decided: accepted changes keep the
 * persona's blocks, rejected ones put the earlier blocks back.
 */
export function resolveReview(segments: Segment[], decisions: Record<number, Decision>): string {
  const blocks = segments.flatMap((s) => (s.kind === "same" ? s.blocks : decisions[s.id] === "reject" ? s.before : s.after));
  return blocks.length ? `${blocks.join("\n\n")}\n` : "";
}

/** How a finished review is recorded, for the version note the persona reads. */
export function reviewNote(segments: Segment[], decisions: Record<number, Decision>, persona: string): string {
  const changes = segments.filter((s) => s.kind === "change");
  const kept = changes.filter((s) => decisions[s.id] !== "reject").length;
  if (kept === 0) return `Rejected ${persona}'s ${changes.length === 1 ? "change" : `${changes.length} changes`}`;
  return `Kept ${kept} of ${persona}'s ${changes.length} changes, rejected ${changes.length - kept}`;
}

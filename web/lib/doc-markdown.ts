import { MarkdownManager } from "@tiptap/markdown";
import StarterKit from "@tiptap/starter-kit";
import { TableKit } from "@tiptap/extension-table";
import { TaskItem, TaskList } from "@tiptap/extension-list";
import type { AnyExtension, JSONContent } from "@tiptap/core";
import { fromMarkdown } from "mdast-util-from-markdown";
import { gfmFromMarkdown } from "mdast-util-gfm";
import { gfm } from "micromark-extension-gfm";

/**
 * What a document can hold in the editor. Everything here reads and writes
 * Markdown, so the canvas stays plain text that every persona tool can patch.
 */
export function docExtensions(): AnyExtension[] {
  return [
    StarterKit.configure({ link: { openOnClick: false, autolink: true }, underline: false }),
    TableKit.configure({ table: { resizable: false } }),
    TaskList,
    TaskItem.configure({ nested: true }),
  ];
}

let manager: MarkdownManager | undefined;
const markdown = () => (manager ??= new MarkdownManager({ extensions: docExtensions() }));

/** The editor's document for some Markdown, without an editor. */
export const parseDoc = (md: string): JSONContent => markdown().parse(md);
/** Markdown for an editor document, as the editor would write it. */
export const serializeDoc = (doc: JSONContent): string => markdown().serialize(doc);

type Mdast = { type: string; children?: Mdast[]; [k: string]: unknown };

/** Fields that change with how Markdown is spelled, not what it says. */
const COSMETIC = new Set(["position", "spread", "align", "start"]);

function shape(n: Mdast): unknown {
  const out: Record<string, unknown> = {};
  for (const [k, v] of Object.entries(n)) {
    if (COSMETIC.has(k) || v == null) continue;
    out[k] = k === "children" ? merged(v as Mdast[]).map(shape) : k === "value" && typeof v === "string" ? v.replace(/\s+/g, " ").trim() : v;
  }
  return out;
}

/** Adjacent text nodes join, since escaping can split one in two. */
function merged(children: Mdast[]): Mdast[] {
  const out: Mdast[] = [];
  for (const c of children) {
    const last = out.at(-1);
    if (c.type === "text" && last?.type === "text") out[out.length - 1] = { ...last, value: `${last.value}${c.value}` };
    else out.push(c);
  }
  return out;
}

const tree = (md: string) => fromMarkdown(md, { extensions: [gfm()], mdastExtensions: [gfmFromMarkdown()] }) as unknown as Mdast;

/**
 * Whether the editor can open this Markdown without losing anything: what it
 * would write back has to mean the same, even if spelled differently (`_a_`
 * for `*a*`). Raw HTML and footnotes, which the editor has no place for, fail.
 */
export function editorKeeps(md: string): boolean {
  try {
    return JSON.stringify(shape(tree(md))) === JSON.stringify(shape(tree(serializeDoc(parseDoc(md)))));
  } catch {
    return false;
  }
}

const escape = (s: string) => s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
/** Markdown syntax that can sit inside a selection without showing in it. */
const INLINE = String.raw`(?:[*_~\x60]|\]\([^)\n]*\)|\[)*`;
const BETWEEN = String.raw`(?:\s|[*_~\x60>#|-]|\d+[.)]|\]\([^)\n]*\)|\[|\[[ xX]\])+`;

/**
 * Where text selected in the editor is in its Markdown: the editor shows no
 * syntax, so the search skips over it. With several matches, the one nearest
 * `near` (an offset) wins. Undefined when it isn't there at all.
 */
export function locateQuote(md: string, quote: string, near = 0): { start: number; end: number } | undefined {
  const words = quote.trim().split(/\s+/).filter(Boolean);
  if (!words.length) return undefined;
  const pattern = words.map((w) => Array.from(w, escape).join(INLINE)).join(BETWEEN);
  let best: { start: number; end: number } | undefined;
  for (const m of md.matchAll(new RegExp(pattern, "g"))) {
    const hit = { start: m.index, end: m.index + m[0].length };
    if (!best || Math.abs(hit.start - near) < Math.abs(best.start - near)) best = hit;
  }
  return best;
}

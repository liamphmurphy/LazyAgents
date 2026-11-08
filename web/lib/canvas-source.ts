/**
 * Maps text rendered from Markdown back to its source, so a selection in a
 * canvas can anchor to source offsets. No DOM access.
 */

type Point = { offset?: number };
type HastNode = {
  type: string;
  value?: string;
  tagName?: string;
  properties?: Record<string, unknown>;
  position?: { start: Point; end: Point };
  children?: HastNode[];
};

const isKatex = (n: HastNode) => {
  const c = n.properties?.className;
  return Array.isArray(c) && c.some((v) => typeof v === "string" && v.startsWith("katex"));
};

function wrap(parent: HastNode) {
  parent.children = parent.children?.map((n) => {
    if (n.type === "element" && !isKatex(n)) wrap(n);
    if (n.type !== "text") return n;
    const start = n.position?.start.offset;
    const end = n.position?.end.offset;
    if (start === undefined || end === undefined || end <= start) return n;
    return { type: "element", tagName: "span", properties: { dataSrcStart: start, dataSrcEnd: end }, children: [n] };
  });
}

/**
 * Rehype plugin: wraps each text run that has a source position in a
 * <span data-src-start data-src-end>. Runs after KaTeX, whose output has no
 * place in the source. Fenced code text carries no position, so it stays unwrapped.
 */
export function rehypeSourceSpans() {
  return (tree: HastNode) => wrap(tree);
}

export type Slot = { id: string; offset: number };
export const SLOT_CLASS = "md-slot:";

/**
 * Rehype plugin: after the top-level block holding each slot's source offset,
 * adds an empty div the renderer fills. Keeps inserted content in React's tree.
 */
export function rehypeSlots(slots: Slot[]) {
  return (tree: HastNode) => {
    const blocks = (tree.children ?? []).filter((n) => n.type === "element" && n.position?.end.offset !== undefined);
    if (!blocks.length) return;
    const after = new Map<HastNode, Slot[]>();
    for (const s of slots) {
      const b = blocks.find((n) => n.position!.end.offset! >= s.offset) ?? blocks[blocks.length - 1];
      after.set(b, [...(after.get(b) ?? []), s]);
    }
    tree.children = tree.children!.flatMap((n) => [n, ...(after.get(n) ?? []).map((s) => ({ type: "element", tagName: "div", properties: { className: [SLOT_CLASS + s.id] }, children: [] }))]);
  };
}

const ENTITY = /^&(#\d{1,7}|#[xX][0-9a-fA-F]{1,6}|[A-Za-z][A-Za-z0-9]*);/;
const PUNCTUATION = /[!-/:-@[-`{-~]/;

function entityWidth(entity: string): number {
  const m = /^&#([xX]?)([0-9a-fA-F]+);/.exec(entity);
  return m && parseInt(m[2], m[1] ? 16 : 10) > 0xffff ? 2 : 1;
}

/**
 * For a text run, the source offset (within `src`) of each index of `rendered`,
 * plus one final entry for its end. Accounts for what Markdown removes inside
 * a run: backslash escapes, character references, and line prefixes such as a
 * blockquote's "> " after a soft break, and inline code's backticks.
 */
export function renderedToSource(src: string, rendered: string): number[] {
  // An inline code run's position includes its backticks; its content is literal.
  const ticks = /^`+/.exec(src)?.[0].length ?? 0;
  if (ticks && !rendered.startsWith(src.slice(0, ticks))) {
    const padded = src.length - 2 * ticks > rendered.length && src[ticks] === " " ? 1 : 0;
    return Array.from({ length: rendered.length + 1 }, (_, i) => ticks + padded + i);
  }
  const map: number[] = [];
  let s = 0;
  for (let r = 0; r < rendered.length; ) {
    map.push(Math.min(s, src.length));
    const c = rendered[r];
    if (c === "\n") {
      const nl = src.indexOf("\n", s);
      s = nl < 0 ? s : nl + 1;
      while (src[s] === " " || src[s] === "\t" || src[s] === ">") s++;
      r++;
      continue;
    }
    if (src[s] === "\\" && src[s + 1] === c && PUNCTUATION.test(c)) {
      s += 2;
      r++;
      continue;
    }
    const entity = src[s] === "&" ? ENTITY.exec(src.slice(s, s + 40))?.[0] : undefined;
    if (entity && !rendered.startsWith(entity, r)) {
      s += entity.length;
      r += entityWidth(entity);
      if (entityWidth(entity) === 2) map.push(Math.min(s, src.length));
      continue;
    }
    s++;
    r++;
  }
  map.push(src.length);
  return map;
}

/** The source range of a selection between two points in wrapped text runs. */
export function selectionToSource(
  source: string,
  from: { start: number; end: number; text: string; offset: number },
  to: { start: number; end: number; text: string; offset: number },
): { start: number; end: number } {
  const at = (p: typeof from) => p.start + renderedToSource(source.slice(p.start, p.end), p.text)[p.offset];
  return { start: at(from), end: at(to) };
}

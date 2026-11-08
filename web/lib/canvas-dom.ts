/**
 * Between the DOM and Markdown source for a canvas preview: its text runs are
 * the spans rehypeSourceSpans adds. Pure offset logic is in canvas-source.
 */
import { renderedToSource, selectionToSource } from "./canvas-source";

const runs = (root: HTMLElement) => Array.from(root.querySelectorAll<HTMLElement>("[data-src-start]"));

function run(el: HTMLElement) {
  return { start: Number(el.dataset.srcStart), end: Number(el.dataset.srcEnd), text: el.textContent ?? "" };
}

/** The source range of a browser selection, or null if it covers no mapped text. */
export function rangeToSource(root: HTMLElement, range: Range, source: string): { start: number; end: number } | null {
  const hit = runs(root).filter((el) => range.intersectsNode(el));
  const offsetIn = (el: HTMLElement, node: Node, offset: number, other: number) => (node.parentElement === el && node.nodeType === 3 ? offset : other);
  // A selection ending at the very start of a run, or starting at the very end of one, doesn't touch it.
  while (hit.length && offsetIn(hit[hit.length - 1], range.endContainer, range.endOffset, 1) === 0) hit.pop();
  while (hit.length && offsetIn(hit[0], range.startContainer, range.startOffset, 0) === run(hit[0]).text.length) hit.shift();
  if (!hit.length) return null;
  const first = hit[0];
  const last = hit[hit.length - 1];
  const out = selectionToSource(
    source,
    { ...run(first), offset: offsetIn(first, range.startContainer, range.startOffset, 0) },
    { ...run(last), offset: offsetIn(last, range.endContainer, range.endOffset, run(last).text.length) },
  );
  return out.end > out.start ? out : null;
}

/** First rendered index whose source offset reaches `target`. */
function renderedIndex(map: number[], target: number): number {
  const i = map.findIndex((s) => s >= target);
  return i < 0 ? map.length - 1 : i;
}

/** DOM ranges covering a source range, one per text run it touches. */
export function sourceToRanges(root: HTMLElement, source: string, start: number, end: number): Range[] {
  const out: Range[] = [];
  for (const el of runs(root)) {
    const r = run(el);
    const node = el.firstChild;
    if (r.end <= start || r.start >= end || !node) continue;
    const map = renderedToSource(source.slice(r.start, r.end), r.text);
    const from = renderedIndex(map, start - r.start);
    const to = renderedIndex(map, end - r.start);
    if (to <= from) continue;
    const range = document.createRange();
    range.setStart(node, from);
    range.setEnd(node, to);
    out.push(range);
  }
  return out;
}

/** Paints ranges with a ::highlight() style, leaving the DOM React owns untouched. */
export function setHighlight(name: string, ranges: Range[]) {
  if (typeof CSS === "undefined" || !CSS.highlights) return;
  if (ranges.length) CSS.highlights.set(name, new Highlight(...ranges));
  else CSS.highlights.delete(name);
}

/** The source offset under a viewport point, or null off mapped text. */
export function sourceOffsetAt(x: number, y: number, source: string): number | null {
  let node: Node | null = null;
  let offset = 0;
  if (document.caretPositionFromPoint) {
    const p = document.caretPositionFromPoint(x, y);
    node = p?.offsetNode ?? null;
    offset = p?.offset ?? 0;
  } else {
    const r = document.caretRangeFromPoint?.(x, y);
    node = r?.startContainer ?? null;
    offset = r?.startOffset ?? 0;
  }
  const el = node?.parentElement;
  if (node?.nodeType !== 3 || !el?.dataset.srcStart) return null;
  const r = run(el);
  return r.start + renderedToSource(source.slice(r.start, r.end), r.text)[offset];
}

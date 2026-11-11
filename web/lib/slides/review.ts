/**
 * Reviewing a persona's changes to a deck, and merging concurrent edits.
 * Both work per slide, keyed by stable ids (ADR §3.6–3.7).
 */
import { slideTitle, textOf, type Deck, type Element, type Slide } from "./schema";

/**
 * A value in the server's canonical form for comparing: keys sorted, unset
 * and default fields dropped (false, empty, left alignment, a zero crop),
 * hex colours upper-cased and numbers rounded as the server rounds them. The
 * server normalises what it stores, so comparing raw JSON would see changes
 * that aren't there.
 */
const COLOR_KEYS = new Set(["color", "fill", "stroke", "highlight", "stops"]);
const OPACITY_KEYS = new Set(["fillOpacity", "strokeOpacity", "opacity"]);

function canon(v: unknown, colour = false): unknown {
  if (Array.isArray(v)) return v.map((x) => canon(x, colour));
  if (typeof v === "number") return Math.round(v * 100) / 100;
  // Only colour fields are upper-cased: run text that looks like "#abcdef" is content.
  if (typeof v === "string") return colour && /^#[0-9a-f]{6}$/i.test(v) ? v.toUpperCase() : v;
  if (v && typeof v === "object") {
    const o = v as Record<string, unknown>;
    const out: Record<string, unknown> = {};
    for (const k of Object.keys(o).sort()) {
      const x = o[k];
      if (x === undefined || x === null || x === false || x === "") continue;
      if (k === "align" && x === "left") continue;
      if ((k === "level" || k === "spaceBefore" || k === "rotation" || k === "strokeWidth" || k === "radius") && x === 0) continue;
      if (OPACITY_KEYS.has(k) && x === 1) continue;
      if (k === "dash" && x === "solid") continue;
      if (k === "angle" && x === 360) {
        out[k] = 0;
        continue;
      }
      if (k === "lineSpacing" && (x === 0 || x === 1)) continue;
      if (k === "crop" && x && typeof x === "object" && Object.values(x).every((n) => n === 0)) continue;
      out[k] = canon(x, COLOR_KEYS.has(k) || (k !== "text" && colourMap(o)));
    }
    return out;
  }
  return v;
}

/** A theme's colours map (every value a hex colour). */
const colourMap = (o: Record<string, unknown>) => Object.values(o).length > 0 && Object.values(o).every((x) => typeof x === "string" && /^#[0-9a-f]{6}$/i.test(x));

/** Whether two deck values are the same once both are in canonical form. */
export const same = (a: unknown, b: unknown) => JSON.stringify(canon(a)) === JSON.stringify(canon(b));

export type Mark = "added" | "removed" | "changed";

export type Change = {
  id: number;
  kind: "added" | "removed" | "moved" | "modified" | "theme";
  slideId?: string;
  before?: Slide;
  after?: Slide;
  /** Where the slide is, 1-based, in the version it's shown from. */
  position: number;
  title: string;
  details: string[];
  marksBefore: Record<string, Mark>;
  marksAfter: Record<string, Mark>;
};

export type Decision = "accept" | "reject";

const quote = (s: string) => (s.length > 60 ? `“${s.slice(0, 60)}…”` : `“${s}”`);
const plain = (e: Element) => (textOf(e)?.paragraphs ?? []).map((p) => p.runs.map((r) => r.text).join("")).join(" / ");
const kindName = (e: Element) => (e.type === "text" ? (e.placeholder ? `${e.placeholder} text` : "text box") : e.type === "shape" ? "shape" : e.type);

/** What changed inside one slide, in words, with the elements to outline. */
function slideDiff(a: Slide, b: Slide) {
  const details: string[] = [];
  const marksBefore: Record<string, Mark> = {};
  const marksAfter: Record<string, Mark> = {};
  if (a.layout !== b.layout) details.push(`Layout: ${a.layout} → ${b.layout}`);
  if (!same(a.background, b.background)) details.push("Background colour changed");
  if ((a.notes ?? "") !== (b.notes ?? "")) details.push("Speaker notes changed");
  if (!!a.hidden !== !!b.hidden) details.push(b.hidden ? "Hidden in the presentation" : "Shown in the presentation");
  const before = new Map(a.elements.map((e) => [e.id, e]));
  const after = new Map(b.elements.map((e) => [e.id, e]));
  for (const e of b.elements) {
    const old = before.get(e.id);
    if (!old) {
      details.push(`Added ${kindName(e)}${plain(e) ? ` ${quote(plain(e))}` : ""}`);
      marksAfter[e.id] = "added";
    } else if (!same(old, e)) {
      marksBefore[e.id] = "changed";
      marksAfter[e.id] = "changed";
      if (plain(old) !== plain(e)) details.push(`${kindName(e)[0].toUpperCase()}${kindName(e).slice(1)}: ${quote(plain(old))} → ${quote(plain(e))}`);
      else if (!same(old.frame, e.frame) || old.rotation !== e.rotation) details.push(`Moved or resized ${kindName(e)}`);
      else details.push(`Restyled ${kindName(e)}`);
    }
  }
  for (const e of a.elements)
    if (!after.has(e.id)) {
      details.push(`Removed ${kindName(e)}${plain(e) ? ` ${quote(plain(e))}` : ""}`);
      marksBefore[e.id] = "removed";
    }
  // Stacking order is the elements' order: say so when the kept ones were reordered.
  const kept = (s: Slide, other: Map<string, Element>) => s.elements.filter((e) => other.has(e.id)).map((e) => e.id).join();
  if (kept(a, after) !== kept(b, before)) details.push("Changed the stacking order of elements");
  return { details, marksBefore, marksAfter };
}

/** The common slides whose order changed: everything outside a longest common subsequence. */
function movedIds(baseOrder: string[], headOrder: string[]): Set<string> {
  const common = new Set(baseOrder.filter((id) => headOrder.includes(id)));
  const a = baseOrder.filter((id) => common.has(id));
  const b = headOrder.filter((id) => common.has(id));
  const dp = Array.from({ length: a.length + 1 }, () => new Array<number>(b.length + 1).fill(0));
  for (let i = a.length - 1; i >= 0; i--) for (let j = b.length - 1; j >= 0; j--) dp[i][j] = a[i] === b[j] ? dp[i + 1][j + 1] + 1 : Math.max(dp[i + 1][j], dp[i][j + 1]);
  const keep = new Set<string>();
  for (let i = 0, j = 0; i < a.length && j < b.length; ) {
    if (a[i] === b[j]) {
      keep.add(a[i]);
      i++;
      j++;
    }
    else if (dp[i + 1][j] >= dp[i][j + 1]) i++;
    else j++;
  }
  return new Set(a.filter((id) => !keep.has(id)));
}

/** The changes from base to head, one per changed slide (and one for the theme), in head order. */
export function deckChanges(base: Deck, head: Deck): Change[] {
  const out: Change[] = [];
  let n = 0;
  if (!same(base.theme, head.theme))
    out.push({ id: n++, kind: "theme", position: 0, title: "Theme", details: [`${base.theme.name} → ${head.theme.name}${base.theme.name === head.theme.name ? " (colours or fonts changed)" : ""}`], marksBefore: {}, marksAfter: {} });
  const baseById = new Map(base.slides.map((s) => [s.id, s]));
  const headIds = new Set(head.slides.map((s) => s.id));
  const moved = movedIds(base.slides.map((s) => s.id), head.slides.map((s) => s.id));
  head.slides.forEach((s, i) => {
    const old = baseById.get(s.id);
    if (!old) {
      out.push({ id: n++, kind: "added", slideId: s.id, after: s, position: i + 1, title: slideTitle(s), details: [`New ${s.layout} slide`], marksBefore: {}, marksAfter: {} });
      return;
    }
    const d = same(old, s) ? null : slideDiff(old, s);
    if (d || moved.has(s.id)) {
      const details = [...(moved.has(s.id) ? [`Moved from position ${base.slides.indexOf(old) + 1} to ${i + 1}`] : []), ...(d?.details ?? [])];
      out.push({ id: n++, kind: d ? "modified" : "moved", slideId: s.id, before: old, after: s, position: i + 1, title: slideTitle(s), details, marksBefore: d?.marksBefore ?? {}, marksAfter: d?.marksAfter ?? {} });
    }
  });
  base.slides.forEach((s, i) => {
    if (!headIds.has(s.id)) out.push({ id: n++, kind: "removed", slideId: s.id, before: s, position: i + 1, title: slideTitle(s), details: ["Slide removed"], marksBefore: {}, marksAfter: {} });
  });
  return out;
}

/** Puts slide after the slide that preceded it in `order`, or first. */
function placeAfterPredecessor(slides: Slide[], slide: Slide, order: string[]): Slide[] {
  const rest = slides.filter((s) => s.id !== slide.id);
  const at = order.indexOf(slide.id);
  for (let k = at - 1; k >= 0; k--) {
    const i = rest.findIndex((s) => s.id === order[k]);
    if (i >= 0) return [...rest.slice(0, i + 1), slide, ...rest.slice(i + 1)];
  }
  return [slide, ...rest];
}

/** The deck once every change is decided: accepted ones keep head, rejected ones restore base. */
export function resolveDeck(base: Deck, head: Deck, changes: Change[], decisions: Record<number, Decision>): Deck {
  let slides = [...head.slides];
  let theme = head.theme;
  const baseOrder = base.slides.map((s) => s.id);
  for (const c of changes) {
    if (decisions[c.id] !== "reject") continue;
    if (c.kind === "theme") theme = base.theme;
    else if (c.kind === "added") slides = slides.filter((s) => s.id !== c.slideId);
    else if (c.kind === "removed" && c.before) slides = placeAfterPredecessor(slides, c.before, baseOrder);
    else if (c.kind === "modified" || c.kind === "moved") {
      const restored = c.before!;
      slides = slides.map((s) => (s.id === c.slideId ? restored : s));
      if (c.details.some((d) => d.startsWith("Moved from"))) slides = placeAfterPredecessor(slides, restored, baseOrder);
    }
  }
  if (!slides.length) slides = [base.slides[0]];
  return { ...head, theme, assets: { ...base.assets, ...head.assets }, slides };
}

/** The note a finished review is saved with, for the persona to read. */
export function reviewNote(changes: Change[], decisions: Record<number, Decision>, persona: string): string {
  const kept = changes.filter((c) => decisions[c.id] !== "reject").length;
  if (kept === 0) return `Rejected ${persona}'s ${changes.length === 1 ? "change" : `${changes.length} changes`}`;
  return `Kept ${kept} of ${persona}'s ${changes.length} changes, rejected ${changes.length - kept}`;
}

/**
 * Three-way merge of two edits of the same base, per slide. Returns the
 * merged deck, or null when both sides changed the same slide, the theme or
 * the slide order: the person then chooses.
 */
export function mergeDecks(base: Deck, mine: Deck, theirs: Deck): Deck | null {
  const changedTheme = (d: Deck) => !same(base.theme, d.theme);
  if (changedTheme(mine) && changedTheme(theirs) && !same(mine.theme, theirs.theme)) return null;
  const baseById = new Map(base.slides.map((s) => [s.id, s]));
  const touched = (d: Deck) => {
    const ids = new Set<string>();
    const byId = new Map(d.slides.map((s) => [s.id, s]));
    for (const s of d.slides) if (!same(baseById.get(s.id), s)) ids.add(s.id);
    for (const s of base.slides) if (!byId.has(s.id)) ids.add(s.id);
    return { ids, byId };
  };
  const m = touched(mine);
  const t = touched(theirs);
  for (const id of m.ids) if (t.ids.has(id) && !same(m.byId.get(id), t.byId.get(id))) return null;
  const reordered = (d: Deck) => movedIds(base.slides.map((s) => s.id), d.slides.map((s) => s.id)).size > 0;
  if (reordered(mine) && reordered(theirs)) return null;
  // Start from the side that reordered (or theirs), then apply the other's slide changes.
  const [first, second, secondTouched] = reordered(mine) ? [mine, theirs, t] : [theirs, mine, m];
  let slides = [...first.slides];
  const secondOrder = second.slides.map((s) => s.id);
  for (const id of secondTouched.ids) {
    const s = secondTouched.byId.get(id);
    if (!s) slides = slides.filter((x) => x.id !== id);
    else if (slides.some((x) => x.id === id)) slides = slides.map((x) => (x.id === id ? s : x));
    else slides = placeAfterPredecessor(slides, s, secondOrder);
  }
  return { ...theirs, theme: changedTheme(mine) ? mine.theme : theirs.theme, assets: { ...theirs.assets, ...mine.assets }, slides };
}

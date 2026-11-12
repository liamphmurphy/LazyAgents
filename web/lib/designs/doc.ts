// The editor's working copy (ADR-designs §3.6): the draft, what was last
// saved, undo/redo history, and the rules the Slides review asked for.
// Pure, so the rules are unit-tested: the React side keeps one of these in a
// ref and re-renders on change.
//   - History is only valid against the draft it was taken from: when the
//     server or a merge replaces the draft, it starts again (otherwise undo
//     would quietly revert someone else's changes).
//   - "Unchanged" is decided on the canonical (normalised) JSON, not object
//     identity, so an edit and its undo don't save a version.
import { settle } from "./layout.ts";
import { normalize, canonical } from "./schema.ts";
import type { Design } from "./schema.ts";

export const HISTORY_LIMIT = 100;
/** Edits with the same key within this window are one undo step (a drag, typing, a slider). */
export const COALESCE_MS = 1500;

export type Doc = {
  draft: Design;
  saved: { seq: number; design: Design };
  past: Design[];
  future: Design[];
  /** The last edit's coalescing key and time. */
  key?: string;
  at: number;
};

/**
 * The body as saved: laid out (the layout's boxes are a cache the editor
 * writes on save, ADR §3.2) and normalised. The saved base is always kept in
 * this form, so comparisons and three-way merges see the same cache values
 * the server stores, not stale ones that would look like edits.
 */
export function settled(d: Design): Design {
  const s = settle(d);
  try {
    return normalize(s).design;
  } catch {
    return s;
  }
}

export const open = (seq: number, design: Design): Doc => ({ draft: design, saved: { seq, design: settled(design) }, past: [], future: [], at: 0 });

/** The body as the server would store it; falls back to sorted JSON for a body the validator refuses. */
export function canonicalJson(d: Design): string {
  try {
    return normalize(d).json;
  } catch {
    return canonical(d);
  }
}
export const sameDesign = (a: Design, b: Design) => a === b || canonicalJson(a) === canonicalJson(b);
export const isDirty = (doc: Doc) => !sameDesign(settle(doc.draft), doc.saved.design);

/** An edit: next replaces the draft as one undo step (or joins the last one with the same key). */
export function edit(doc: Doc, next: Design, key?: string, now = Date.now()): Doc {
  if (next === doc.draft) return doc;
  const join = !!key && doc.key === key && now - doc.at < COALESCE_MS;
  const past = join ? doc.past : [...doc.past, doc.draft].slice(-HISTORY_LIMIT);
  return { ...doc, draft: next, past, future: [], key, at: now };
}

export function undo(doc: Doc): Doc {
  const to = doc.past.at(-1);
  if (!to) return doc;
  return { ...doc, draft: to, past: doc.past.slice(0, -1), future: [...doc.future, doc.draft], key: undefined };
}

export function redo(doc: Doc): Doc {
  const to = doc.future.at(-1);
  if (!to) return doc;
  return { ...doc, draft: to, future: doc.future.slice(0, -1), past: [...doc.past, doc.draft], key: undefined };
}

/** A save of `design` landed as seq; the draft (and its history) stay, since later edits build on it. */
export const saved = (doc: Doc, seq: number, design: Design): Doc => ({ ...doc, saved: { seq, design: settled(design) } });

/**
 * A newer version from the server. With nothing unsaved here it replaces the
 * draft and history starts again; with unsaved edits it's ignored (the next
 * save meets it as a 409 and merges).
 */
export function fromServer(doc: Doc, seq: number, design: Design): Doc {
  if (seq <= doc.saved.seq || isDirty(doc)) return doc;
  return open(seq, design);
}

/** A merge after a 409: theirs is now the saved base, the merge the draft, and history starts again. */
export const merged = (doc: Doc, seq: number, theirs: Design, draft: Design): Doc => ({ ...open(seq, theirs), draft });

/** The person chose the latest version over theirs: everything unsaved is dropped. */
export const takeTheirs = (seq: number, theirs: Design): Doc => open(seq, theirs);

/** The person chose to keep their version: it saves on top of the latest one. */
export const keepMine = (doc: Doc, seq: number, theirs: Design): Doc => ({ ...doc, saved: { seq, design: settled(theirs) }, past: [], future: [], key: undefined });

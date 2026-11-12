import test from "node:test";
import assert from "node:assert/strict";
import * as doc from "./designs/doc.ts";
import { find } from "./designs/edit.ts";
import { adoptVersion, applyOps, classify, creates, decide, decideRest, dependencies, resolveBody, undecided } from "./designs/review.ts";
import { normalize } from "./designs/schema.ts";

const text = (id, y, s) => ({ id, type: "text", name: s, x: 0, y, w: 100, h: 20, paragraphs: [{ runs: [{ text: s, size: 16 }] }] });
const rect = (id, x = 0, fill = "#000000") => ({ id, type: "shape", shape: "rect", name: "Button", x, y: 50, w: 40, h: 20, fill });
const base = () =>
  normalize({
    schema: 1, mode: "light", token_sets: {}, assets: {}, links: [],
    pages: [{ id: "p_1", name: "P", children: [{ id: "n_f", type: "frame", name: "Start", x: 0, y: 0, w: 300, h: 400, fill: "#FFFFFF", clip: true, children: [text("n_t1", 0, "One"), text("n_t2", 30, "Two"), rect("n_b")] }] }],
  }).design;
const OPS = [
  { op: "add_node", parent: "n_f", why: "Add a promo code field", node: text("n_new", 100, "Promo code") },
  { op: "set_node", id: "n_b", fields: { fill: "#2F6BFF" }, why: "Stand out" },
  { op: "remove_node", id: "n_t2", why: "Placeholder" },
];

test("each op is added, changed or removed, with a pin box and its why", () => {
  const cs = classify(base(), OPS);
  assert.deepEqual(cs.map((c) => c.kind), ["added", "changed", "removed"]);
  assert.deepEqual(cs.map((c) => c.target), ["n_new", "n_b", "n_t2"]);
  assert.deepEqual(cs.map((c) => c.why), ["Add a promo code field", "Stand out", "Placeholder"]);
  assert.match(cs[0].title, /Promo code/);
  assert.match(cs[2].title, /Two/);
  for (const c of cs) {
    assert.equal(c.page, "p_1");
    assert.ok(c.box && c.box.w > 0, c.title);
  }
});

test("applying ops previews After on a copy and leaves the original alone", () => {
  const d = base();
  const after = applyOps(d, OPS);
  assert.ok(find(after, "n_new"));
  assert.equal(find(after, "n_t2"), undefined);
  assert.equal(find(after, "n_b").node.fill, "#2F6BFF");
  assert.equal(find(d, "n_b").node.fill, "#000000");
  // Only the ops kept; one whose target is gone is skipped.
  const some = applyOps(d, [...OPS, { op: "set_node", id: "n_gone", fields: { fill: "#fff" } }], (i) => i !== 2);
  assert.ok(find(some, "n_t2"));
});

test("an op that points at what another creates depends on it", () => {
  const ops = [
    { op: "add_node", parent: "n_f", node: { id: "n_x", type: "frame", x: 0, y: 0, w: 10, h: 10, children: [{ id: "n_y", type: "shape", shape: "rect", x: 0, y: 0, w: 1, h: 1 }] } },
    { op: "set_node", id: "n_y", fields: { fill: "#fff" } },
    { op: "add_node", parent: "n_x", node: { id: "n_z", type: "frame", x: 0, y: 0, w: 1, h: 1, children: [] } },
    { op: "set_node", id: "n_b", fields: { fill: "#fff" } },
  ];
  assert.deepEqual(creates(ops[0]).sort(), ["n_x", "n_y"]);
  assert.deepEqual(dependencies(ops), [[], [0], [0], []]);
});

test("accepting a change accepts what it needs; rejecting one rejects what needs it", () => {
  const deps = [[], [0], [1], []];
  let d = decide(deps, {}, 2, "accepted");
  assert.deepEqual([d[0]?.state, d[1]?.state, d[2]?.state, d[3]], ["accepted", "accepted", "accepted", undefined]);
  d = decide(deps, {}, 0, "rejected", " not now ");
  assert.deepEqual(d[0], { state: "rejected", reason: "not now" });
  assert.deepEqual([d[1]?.state, d[2]?.state, d[3]], ["rejected", "rejected", undefined]);
  // Rest: only the undecided ones.
  const rest = decideRest(deps, decide(deps, {}, 3, "rejected", "no"), 4, "accepted");
  assert.deepEqual(rest[3], { state: "rejected", reason: "no" });
  assert.equal(rest[2].state, "accepted");
  assert.deepEqual(undecided(rest, 4), []);
});

test("Done needs every change decided; the body has one decision per op in order", () => {
  assert.equal(resolveBody({ 0: { state: "accepted" } }, 3), undefined);
  assert.deepEqual(resolveBody({ 0: { state: "accepted" }, 1: { state: "rejected", reason: "why" }, 2: { state: "accepted" } }, 3), {
    decisions: [{ i: 0, state: "accepted" }, { i: 1, state: "rejected", reason: "why" }, { i: 2, state: "accepted" }],
  });
});

// ---- a version from elsewhere never overwrites unsaved edits ----

test("a persona version replaces a clean draft as one undo step", () => {
  const d = base();
  const s = doc.open(1, d);
  const theirs = applyOps(d, [OPS[1]]);
  const r = adoptVersion(s, 2, theirs);
  assert.equal(r.doc.saved.seq, 2);
  assert.equal(find(r.doc.draft, "n_b").node.fill, "#2F6BFF");
  assert.equal(doc.isDirty(r.doc), false);
  assert.equal(r.doc.past.length, 1);
  // Undo puts the old design back as an unsaved edit that saves as the inverting version.
  const u = doc.undo(r.doc);
  assert.equal(find(u.draft, "n_b").node.fill, "#000000");
  assert.equal(doc.isDirty(u), true);
});

test("a version merges with unsaved local edits; neither side is lost", () => {
  const d = base();
  let s = doc.open(1, d);
  // Mine: move the first text. Theirs: recolour the button and drop the second text.
  s = doc.edit(s, { ...d, pages: [{ ...d.pages[0], children: [{ ...d.pages[0].children[0], children: d.pages[0].children[0].children.map((n) => (n.id === "n_t1" ? { ...n, x: 40 } : n)) }] }] });
  const theirs = applyOps(d, [OPS[1], OPS[2]]);
  const r = adoptVersion(s, 2, theirs);
  assert.equal(r.conflict, false);
  assert.equal(find(r.doc.draft, "n_t1").node.x, 40);
  assert.equal(find(r.doc.draft, "n_b").node.fill, "#2F6BFF");
  assert.equal(find(r.doc.draft, "n_t2"), undefined);
  assert.equal(r.doc.saved.seq, 2);
  assert.equal(doc.isDirty(r.doc), true);
  // One undo step takes the arrival back and keeps my edit's draft.
  assert.equal(find(doc.undo(r.doc).draft, "n_t2") !== undefined, true);
});

test("overlapping edits are a conflict and leave the working copy untouched", () => {
  const d = base();
  let s = doc.open(1, d);
  const recolour = (fill) => ({ ...d, pages: [{ ...d.pages[0], children: [{ ...d.pages[0].children[0], children: d.pages[0].children[0].children.map((n) => (n.id === "n_b" ? { ...n, fill } : n)) }] }] });
  s = doc.edit(s, recolour("#FF0000"));
  const r = adoptVersion(s, 2, applyOps(d, [OPS[1]]));
  assert.equal(r.conflict, true);
  assert.equal(r.doc, s);
});

test("an older or identical version changes nothing but the base", () => {
  const d = base();
  const s = doc.open(3, d);
  assert.equal(adoptVersion(s, 2, d).doc, s);
  const same = adoptVersion(s, 4, d);
  assert.equal(same.doc.saved.seq, 4);
  assert.equal(same.doc.draft, s.draft);
});

import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import * as doc from "./designs/doc.ts";
import { duplicateNodes, find, groupNodes, insertNodes, newId, pathFrom, removeNodes, reorder, resizeNode, ungroupNodes } from "./designs/edit.ts";
import { mergeDesigns } from "./designs/merge.ts";
import { normalize } from "./designs/schema.ts";

const read = (p) => readFileSync(new URL(p, import.meta.url), "utf8");
const acceptance = () => normalize(read("../../docs/design/fixtures/acceptance-design.json")).design;
const rect = (id, x = 0, y = 0) => ({ id, type: "shape", shape: "rect", x, y, w: 10, h: 10, fill: "#000000" });
const design = (children) => ({ schema: 1, mode: "light", token_sets: {}, assets: {}, links: [], pages: [{ id: "p_1", name: "P", children }] });
const ids = (d) => d.pages[0].children.map((n) => n.id);

// ---- history (criterion 4) ----

test("undo and redo walk the history; a new edit drops the redo branch", () => {
  const a = design([rect("n_a")]);
  const b = design([rect("n_a", 5)]);
  const c = design([rect("n_a", 9)]);
  let s = doc.open(1, a);
  s = doc.edit(s, b, undefined, 0);
  s = doc.edit(s, c, undefined, 10);
  s = doc.undo(s);
  assert.equal(s.draft, b);
  s = doc.redo(s);
  assert.equal(s.draft, c);
  s = doc.undo(doc.undo(s));
  assert.equal(s.draft, a);
  s = doc.edit(s, b, undefined, 20);
  assert.equal(s.future.length, 0);
});

test("edits with one key within the window are one undo step", () => {
  const a = design([rect("n_a")]);
  let s = doc.open(1, a);
  s = doc.edit(s, design([rect("n_a", 1)]), "drag", 0);
  s = doc.edit(s, design([rect("n_a", 2)]), "drag", 100);
  s = doc.edit(s, design([rect("n_a", 3)]), "drag", 200);
  assert.equal(s.past.length, 1);
  assert.equal(doc.undo(s).draft, a);
  s = doc.edit(s, design([rect("n_a", 4)]), "drag", 200 + doc.COALESCE_MS + 1);
  assert.equal(s.past.length, 2);
});

test("history resets when the server replaces the working copy", () => {
  let s = doc.open(1, design([rect("n_a")]));
  s = doc.edit(s, design([rect("n_a", 5)]), undefined, 0);
  // Unsaved edits: a newer server version is left for the save to merge.
  assert.equal(doc.fromServer(s, 2, design([rect("n_b")])), s);
  s = doc.saved(s, 2, s.draft);
  assert.equal(s.past.length, 1, "saving keeps the history");
  const theirs = design([rect("n_a", 5), rect("n_p")]);
  s = doc.fromServer(s, 3, theirs);
  assert.equal(s.draft, theirs);
  assert.equal(s.past.length, 0);
  assert.equal(doc.undo(s), s, "nothing to undo into the persona's version");
});

test("history resets when a merge replaces the working copy, and on either conflict choice", () => {
  let s = doc.open(1, design([rect("n_a")]));
  s = doc.edit(s, design([rect("n_a", 5)]), undefined, 0);
  const theirs = design([rect("n_a"), rect("n_p")]);
  const m = doc.merged(s, 2, theirs, design([rect("n_a", 5), rect("n_p")]));
  assert.equal(m.past.length + m.future.length, 0);
  assert.equal(m.saved.seq, 2);
  assert.ok(doc.isDirty(m));
  assert.equal(doc.keepMine(s, 2, theirs).past.length, 0);
  assert.equal(doc.takeTheirs(2, theirs).draft, theirs);
});

test("unchanged is decided on canonical JSON, not identity", () => {
  const a = design([rect("n_a")]);
  const s = doc.edit(doc.open(1, a), structuredClone(a), undefined, 0);
  assert.equal(doc.isDirty(s), false);
  const withDefault = design([{ ...rect("n_a"), opacity: 1, visible: true }]);
  assert.equal(doc.sameDesign(a, withDefault), true, "defaults normalise away");
});

// ---- merge (criterion 6) ----

test("different nodes changed on each side merge", () => {
  const base = design([rect("n_a"), rect("n_b")]);
  const mine = design([rect("n_a", 50), rect("n_b")]);
  const theirs = design([rect("n_a"), rect("n_b", 0, 70), rect("n_c")]);
  const r = mergeDesigns(base, mine, theirs);
  assert.deepEqual(r.conflicts, []);
  const [a, b, c] = r.design.pages[0].children;
  assert.deepEqual([a.id, a.x, b.id, b.y, c.id], ["n_a", 50, "n_b", 70, "n_c"]);
});

test("the same node changed differently on both sides is a conflict", () => {
  const base = design([rect("n_a")]);
  const r = mergeDesigns(base, design([rect("n_a", 1)]), design([rect("n_a", 2)]));
  assert.deepEqual(r.conflicts, ["n_a"]);
  assert.equal(r.design, undefined);
});

test("removing a node I didn't touch merges; removing one I edited is a conflict", () => {
  const base = design([rect("n_a"), rect("n_b")]);
  assert.deepEqual(ids(mergeDesigns(base, design([rect("n_a", 3), rect("n_b")]), design([rect("n_a")])).design), ["n_a"]);
  assert.deepEqual(mergeDesigns(base, design([rect("n_a", 3), rect("n_b")]), design([rect("n_b")])).conflicts, ["n_a"]);
});

test("additions on both sides keep both, after their previous siblings", () => {
  const base = design([rect("n_a"), rect("n_b")]);
  const r = mergeDesigns(base, design([rect("n_a"), rect("n_m"), rect("n_b")]), design([rect("n_a"), rect("n_b"), rect("n_t")]));
  assert.deepEqual(ids(r.design), ["n_a", "n_m", "n_b", "n_t"]);
});

test("a child added under a container the other side removed is a conflict", () => {
  const frame = (children) => ({ id: "n_f", type: "frame", x: 0, y: 0, w: 100, h: 100, children });
  const base = design([frame([])]);
  const r = mergeDesigns(base, design([frame([rect("n_k")])]), design([]));
  assert.ok(r.conflicts.length > 0);
});

test("the acceptance design merges with itself unchanged", () => {
  const d = acceptance();
  const r = mergeDesigns(d, d, d);
  assert.equal(normalize(r.design).json, normalize(d).json);
});

// ---- tree edits ----

test("ids are opaque and valid", () => {
  for (let i = 0; i < 50; i++) assert.match(newId("n"), /^n_[a-z2-7]{10}$/);
});

test("duplicate copies with fresh ids above the original; remove drops connectors and links that touch it", () => {
  const d = acceptance();
  const flows = d.pages[0];
  const first = flows.children.find((n) => n.type === "frame");
  const dup = duplicateNodes(d, [first.id]);
  assert.equal(dup.ids.length, 1);
  assert.notEqual(dup.ids[0], first.id);
  assert.equal(normalize(dup.design).design.pages[0].children.length, flows.children.length + 1);
  const linked = d.links[0].to;
  const gone = removeNodes(d, new Set([linked]));
  assert.ok(!gone.links.some((l) => l.from === linked || l.to === linked));
  assert.equal(find(gone, linked), undefined);
  normalize(gone);
});

test("group and ungroup round-trip positions; reorder moves within the list", () => {
  const d = design([rect("n_a", 10, 10), rect("n_b", 40, 30), rect("n_c")]);
  const g = groupNodes(d, ["n_a", "n_b"]);
  const grp = find(g.design, g.id).node;
  assert.deepEqual([grp.x, grp.y, grp.w, grp.h], [10, 10, 40, 30]);
  assert.deepEqual(grp.children.map((c) => [c.x, c.y]), [[0, 0], [30, 20]]);
  const u = ungroupNodes(g.design, [g.id]);
  assert.deepEqual(u.design.pages[0].children.map((n) => [n.id, n.x, n.y]), [["n_a", 10, 10], ["n_b", 40, 30], ["n_c", 0, 0]]);
  assert.deepEqual(ids(reorder(d, new Set(["n_a"]), "front")), ["n_b", "n_c", "n_a"]);
  assert.deepEqual(ids(insertNodes(d, "p_1", [rect("n_z")], 0)), ["n_z", "n_a", "n_b", "n_c"]);
});

test("resizing a frame applies its children's constraints; a path scales its points", () => {
  const f = { id: "n_f", type: "frame", x: 0, y: 0, w: 100, h: 100, children: [{ ...rect("n_k", 80, 80), constraints: { h: "right", v: "bottom" } }] };
  const r = resizeNode(f, { x: 0, y: 0, w: 200, h: 150 });
  assert.deepEqual([r.children[0].x, r.children[0].y], [180, 130]);
  const p = pathFrom([[0, 0], [10, 20]], false);
  const big = resizeNode(p, { x: p.x, y: p.y, w: 20, h: 40 });
  assert.equal(big.d, "M0 0 L20 40");
});

// ---- Home ----
import { filterDesigns, isBrief, briefTitle, kindOf, linksOf } from "./designs/home.ts";

test("Home filters by author, review and search", () => {
  const list = [
    { id: "1", title: "Checkout flow", persona_edited: true, pending_proposals: 3, frames: 5, pages: 1 },
    { id: "2", title: "Runner architecture", persona_edited: false, pending_proposals: 0, frames: 0, pages: 1 },
  ];
  assert.deepEqual(filterDesigns(list, "mine", "").map((d) => d.id), ["2"]);
  assert.deepEqual(filterDesigns(list, "persona", "").map((d) => d.id), ["1"]);
  assert.deepEqual(filterDesigns(list, "review", "").map((d) => d.id), ["1"]);
  assert.deepEqual(filterDesigns(list, "all", "RUNNER").map((d) => d.id), ["2"]);
});

test("Home tells a brief from a title and reads cards from the body", () => {
  assert.equal(isBrief("Checkout"), false);
  assert.equal(isBrief("Turn LAZYAGENTS-150 into a flow"), true);
  assert.equal(briefTitle("Turn LAZYAGENTS-150 into a flow"), "LAZYAGENTS-150");
  const d = acceptance();
  assert.ok(linksOf(d).includes("LAZYAGENTS-150"));
  assert.equal(kindOf(d, 4), "4 frames");
});

// ---- hit testing ----
import { hitPath, pick, dropTarget } from "./designs/hit.ts";
import { makeLayout } from "./designs/layout.ts";
import { reparent } from "./designs/edit.ts";

test("a click picks a top-level frame's child; double-click goes deeper; empty frame area picks the frame", () => {
  const inner = { id: "n_row", type: "frame", x: 10, y: 10, w: 80, h: 40, children: [rect("n_dot", 5, 5)] };
  const d = design([{ id: "n_f", type: "frame", x: 100, y: 100, w: 200, h: 200, fill: "#FFFFFF", children: [inner] }]);
  const { boxes } = makeLayout(d).layoutPage(d.pages[0]);
  const path = hitPath(d.pages[0], boxes, 117, 117);
  assert.deepEqual(path.map((n) => n.id), ["n_f", "n_row", "n_dot"]);
  assert.equal(pick(path, [], "click").id, "n_row");
  assert.equal(pick(path, ["n_row"], "deeper").id, "n_dot");
  assert.equal(pick(path, [], "deep").id, "n_dot");
  const empty = hitPath(d.pages[0], boxes, 250, 250);
  assert.equal(pick(empty, [], "click").id, "n_f");
  assert.equal(dropTarget(d.pages[0], boxes, 115, 115).id, "n_row");
  assert.deepEqual(hitPath(d.pages[0], boxes, 10, 10), []);
});

test("reparenting keeps a node where it is on the page", () => {
  const d = design([{ id: "n_f", type: "frame", x: 100, y: 100, w: 200, h: 200, children: [] }, rect("n_a", 150, 160)]);
  const out = reparent(d, "n_a", "n_f", { x: 150, y: 160 }, { x: 100, y: 100 });
  const f = find(out, "n_a");
  assert.equal(f.parent.id, "n_f");
  assert.deepEqual([f.node.x, f.node.y], [50, 60]);
  assert.equal(reparent(d, "n_f", "n_f", { x: 0, y: 0 }, { x: 0, y: 0 }), d);
});

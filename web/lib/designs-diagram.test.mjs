import test from "node:test";
import assert from "node:assert/strict";
import { connectNodes, find, mapNodes, newSection, newSticky, wrapInSection } from "./designs/edit.ts";
import { makeLayout, tokenOr } from "./designs/layout.ts";
import { connectTarget, hitPath } from "./designs/hit.ts";
import { normalize } from "./designs/schema.ts";

const body = (children, extra = {}) => ({ schema: 1, mode: "light", token_sets: {}, assets: {}, links: [], pages: [{ id: "p_one", name: "One", children }], ...extra });
const design = (children) => normalize(JSON.stringify(body(children))).design;
const shape = (id, x, y) => ({ id, type: "shape", shape: "rect", x, y, w: 100, h: 60 });
const valid = (d) => assert.doesNotThrow(() => normalize(JSON.stringify(d)));

test("a new sticky and its text edit validate and draw the text", () => {
  const s = newSticky(10, 20, "", "Liam");
  let d = design([s]);
  d = mapNodes(d, new Set([s.id]), (n) => ({ ...n, text: "Check copy" }));
  valid(d);
  const ops = makeLayout(d).layoutPage(d.pages[0]).nodes[0].ops;
  assert.ok(ops.some((o) => o.op === "text" && o.text === "Check"));
  assert.ok(ops.some((o) => o.op === "text" && o.text === "Liam"));
});

test("drawing a section takes in the nodes inside it, and moving it moves them", () => {
  const d0 = design([shape("n_a", 100, 100), shape("n_b", 300, 100), shape("n_out", 900, 100), { id: "n_c", type: "connector", from: { node: "n_a" }, to: { node: "n_b" } }]);
  const sec = newSection(d0, { x: 50, y: 50, w: 500, h: 300 });
  const d1 = wrapInSection(d0, "p_one", sec);
  valid(d1);
  const s = d1.pages[0].children[0];
  assert.equal(s.id, sec.id);
  assert.deepEqual(s.children.map((c) => c.id), ["n_a", "n_b", "n_c"], "both ends in, so the connector moves in too");
  assert.deepEqual([s.children[0].x, s.children[0].y], [50, 50], "children are relative to the section");
  assert.deepEqual(d1.pages[0].children.map((c) => c.id), [sec.id, "n_out"]);

  const before = makeLayout(d1).layoutPage(d1.pages[0]);
  const d2 = mapNodes(d1, new Set([sec.id]), (n) => ({ ...n, x: n.x + 200, y: n.y + 40 }));
  const after = makeLayout(d2).layoutPage(d2.pages[0]);
  for (const id of ["n_a", "n_b"]) assert.deepEqual([after.boxes.get(id).x, after.boxes.get(id).y], [before.boxes.get(id).x + 200, before.boxes.get(id).y + 40]);
  const line = (l) => l.nodes[0].ops.find((o) => o.op === "polyline").points;
  assert.deepEqual(line(after), line(before).map(([x, y]) => [x + 200, y + 40]));
  assert.deepEqual(after.boxes.get("n_out"), before.boxes.get("n_out"));
});

test("section colours use the design's tokens when it has them, else hex", () => {
  assert.equal(tokenOr({ mode: "dark", token_sets: {} }, "surface-raised"), "#1B1A27");
  const sets = { t: { name: "t", source: null, modes: { dark: { "surface-raised": { kind: "color", value: "#111111" } } } } };
  assert.equal(tokenOr({ mode: "dark", token_sets: sets, tokens_ref: { set: "t" } }, "surface-raised"), "$surface-raised");
});

test("connectNodes puts the connector in the ends' lowest common container, never a group", () => {
  const sec = { id: "n_s", type: "section", name: "S", x: 0, y: 0, w: 800, h: 400, children: [shape("n_a", 10, 60), { id: "n_g", type: "group", x: 300, y: 60, w: 100, h: 60, children: [shape("n_b", 0, 0)] }] };
  const d = design([sec, shape("n_top", 1000, 0)]);
  const r = connectNodes(d, "n_a", "n_b", { id: "n_new" });
  assert.equal(r.id, "n_new");
  assert.equal(find(r.design, "n_new").parent.id, "n_s");
  valid(r.design);
  const top = connectNodes(d, "n_a", "n_top");
  assert.equal(find(top.design, top.id).parent, null, "across containers: on the page");
  assert.equal(connectNodes(d, "n_a", "n_a"), undefined);
});

test("the connector tool attaches to the node under the pointer, not the section around it", () => {
  const d = design([{ id: "n_s", type: "section", name: "S", x: 0, y: 0, w: 800, h: 400, children: [shape("n_a", 10, 60)] }]);
  const laid = makeLayout(d).layoutPage(d.pages[0]);
  assert.equal(connectTarget(hitPath(d.pages[0], laid.boxes, 50, 80)).id, "n_a");
  assert.equal(connectTarget(hitPath(d.pages[0], laid.boxes, 600, 300)).id, "n_s");
});

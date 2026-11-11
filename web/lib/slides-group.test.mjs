import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import { duplicate } from "./slides/arrange.ts";
import { boundsOf, findDeep, groupElements, mapDeep, removeDeep, scaleGroup, scopeElements, ungroupElements, withFrame } from "./slides/group.ts";
import { layoutSlide } from "./slides/layout.ts";
import { EMU, wrapGroups } from "./slides/pptx-groups.ts";
import { deckChanges } from "./slides/review.ts";

const shape = (id, x, y, w = 100, h = 100, more = {}) => ({ id, type: "shape", shape: "rect", frame: { x, y, w, h }, ...more });
const ids = (els) => els.map((e) => e.id).join(",");
let n = 0;
const fresh = () => `e_new${++n}`;
const fixture = () => JSON.parse(readFileSync(new URL("../../docs/slides/fixtures/acceptance-deck.json", import.meta.url), "utf8"));

test("group then ungroup leaves every child's position, size and rotation as it was", () => {
  const line = { id: "l", type: "line", frame: { x: 500, y: 400, w: -200, h: -100 }, stroke: "text", strokeWidth: 4 };
  const els = [shape("a", 0, 0), shape("b", 300, 50, 120, 80, { rotation: 30 }), shape("c", 900, 900), line];
  const g = groupElements(els, new Set(["a", "b", "l"]), fresh);
  // The group goes where the topmost of them was, holding them in stacking order.
  assert.equal(ids(g.elements), `c,${g.id}`);
  const grp = g.elements[1];
  assert.equal(ids(grp.elements), "a,b,l");
  assert.deepEqual(grp.frame, { x: 0, y: 0, w: 500, h: 400 });
  const back = ungroupElements(g.elements, new Set([g.id]));
  assert.equal(ids(back.elements), "c,a,b,l");
  assert.deepEqual(back.ids, ["a", "b", "l"]);
  for (const e of back.elements) assert.deepEqual(e, els.find((x) => x.id === e.id));
});

test("grouping needs two elements, merges selected groups and drops placeholder roles", () => {
  assert.equal(groupElements([shape("a", 0, 0)], new Set(["a"]), fresh), undefined);
  const g1 = groupElements([shape("a", 0, 0), shape("b", 10, 10)], new Set(["a", "b"]), fresh);
  const title = { id: "t", type: "text", placeholder: "title", frame: { x: 0, y: 500, w: 100, h: 50 }, paragraphs: [{ runs: [{ text: "T", size: 40 }] }] };
  const g2 = groupElements([...g1.elements, title], new Set([g1.id, "t"]), fresh);
  const grp = g2.elements[0];
  assert.equal(g2.elements.length, 1);
  assert.equal(ids(grp.elements), "a,b,t");
  assert.equal(grp.elements[2].placeholder, undefined);
  assert.ok(grp.elements.every((c) => c.type !== "group"));
});

test("resizing a group scales its children with it", () => {
  const g = { id: "g", type: "group", frame: { x: 100, y: 100, w: 400, h: 200 }, elements: [shape("a", 100, 100, 100, 100), shape("b", 300, 200, 200, 100, { rotation: 10 })] };
  const big = scaleGroup(g, { x: 0, y: 0, w: 800, h: 100 });
  assert.deepEqual(big.frame, { x: 0, y: 0, w: 800, h: 100 });
  assert.deepEqual(big.elements[0].frame, { x: 0, y: 0, w: 200, h: 50 });
  assert.deepEqual(big.elements[1].frame, { x: 400, y: 50, w: 400, h: 50 });
  assert.equal(big.elements[1].rotation, 10);
  // Moving is a resize to the same size: children shift and keep their size.
  const moved = withFrame(g, { ...g.frame, x: 150, y: 90 });
  assert.deepEqual(moved.elements.map((c) => c.frame), [{ x: 150, y: 90, w: 100, h: 100 }, { x: 350, y: 190, w: 200, h: 100 }]);
  // Scaling back restores the children.
  assert.deepEqual(scaleGroup(big, g.frame).elements.map((c) => c.frame), g.elements.map((c) => c.frame));
  // Anything else just takes the frame.
  assert.deepEqual(withFrame(shape("x", 0, 0), { x: 1, y: 2, w: 3, h: 4 }).frame, { x: 1, y: 2, w: 3, h: 4 });
});

test("edits reach into groups and refit them; removing dissolves a group left with one element", () => {
  const els = [shape("a", 0, 0), { id: "g", type: "group", frame: { x: 200, y: 0, w: 300, h: 100 }, elements: [shape("b", 200, 0), shape("c", 400, 0)] }];
  assert.equal(findDeep(els, "c").group.id, "g");
  assert.equal(ids(scopeElements(els, "g")), "b,c");
  assert.equal(scopeElements(els, undefined), els);
  assert.equal(mapDeep(els, new Set(["zz"]), (e) => ({ ...e })), els);
  const moved = mapDeep(els, new Set(["c"]), (e) => ({ ...e, frame: { ...e.frame, x: 600 } }));
  assert.deepEqual(moved[1].frame, { x: 200, y: 0, w: 500, h: 100 });
  assert.equal(moved[0], els[0]);
  assert.equal(ids(removeDeep(els, new Set(["b"]))), "a,c");
  assert.equal(ids(removeDeep(els, new Set(["g"]))), "a");
  assert.equal(boundsOf([{ x: 10, y: 100, w: 50, h: -80 }, { x: 0, y: 50, w: 5, h: 5 }]).y, 20);
});

test("duplicating a group gives its children fresh ids", () => {
  const els = [{ id: "g", type: "group", frame: { x: 0, y: 0, w: 300, h: 100 }, elements: [shape("b", 0, 0), shape("c", 200, 0)] }];
  const r = duplicate(els, new Set(["g"]), fresh);
  const copy = r.elements[1];
  assert.notEqual(copy.id, "g");
  assert.ok(copy.elements.every((c) => !["b", "c"].includes(c.id)));
  assert.deepEqual(copy.elements[1].frame, { x: 240, y: 40, w: 100, h: 100 });
});

test("a group's children lay out like loose elements, tagged with the group", () => {
  const deck = fixture();
  const slide = deck.slides.find((s) => s.id === "s_group");
  const measure = (t, _s, _st, size) => t.length * size * 0.5;
  const grouped = layoutSlide(deck, slide, measure);
  const loose = layoutSlide(deck, { ...slide, elements: ungroupElements(slide.elements, new Set(["e_gp1", "e_gp4"])).elements }, measure);
  assert.deepEqual(grouped.ops.map((o) => Object.fromEntries(Object.entries(o).filter(([k]) => k !== "g"))), loose.ops);
  assert.deepEqual([...new Set(grouped.ops.map((o) => o.g).filter(Boolean))], ["e_gp1", "e_gp4"]);
  assert.ok(grouped.ops.filter((o) => o.el === "e_gp7").every((o) => o.rot?.deg === 6 && o.g === "e_gp4"));
});

test("the review diff names edits inside a group and outlines the child", () => {
  const base = fixture();
  const head = structuredClone(base);
  const slide = head.slides.find((s) => s.id === "s_group");
  const g = slide.elements.find((e) => e.id === "e_gp4");
  g.elements[0].text.paragraphs[0].runs[0].text = "Plan it";
  g.elements[2].fill = "accent";
  g.elements.splice(1, 1);
  const [c] = deckChanges(base, head);
  assert.equal(c.slideId, "s_group");
  assert.deepEqual(c.details, ["Changed which elements are in a group (now 2)", "Shape in a group: “Plan” → “Plan it”", "Restyled shape in a group", "Removed line from a group"]);
  assert.deepEqual(c.marksAfter, { e_gp4: "changed", e_gp5: "changed", e_gp7: "changed" });
  assert.equal(c.marksBefore.e_gp6, "removed");

  // Grouping and ungrouping read as such, not as everything added or removed.
  const grouped = structuredClone(base);
  const s2 = grouped.slides.find((s) => s.id === "s_group");
  s2.elements = ungroupElements(s2.elements, new Set(["e_gp1"])).elements;
  assert.deepEqual(deckChanges(base, grouped)[0].details, ["Ungrouped 2 elements"]);
  assert.deepEqual(deckChanges(grouped, base)[0].details, ["Grouped 2 elements"]);
});

test("PPTX: a group's shapes are wrapped in one p:grpSp at the group's place", () => {
  const sp = (id, name) => `<p:sp><p:nvSpPr><p:cNvPr id="${id}" name="${name}"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr><p:spPr/></p:sp>`;
  const pic = (id, name) => `<p:pic><p:nvPicPr><p:cNvPr id="${id}" name="${name}" descr="x"/></p:nvPicPr></p:pic>`;
  const xml = `<p:spTree><p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr>${sp(2, "e_a")}${sp(3, "e_b")}${pic(4, "e_c")}${sp(5, "e_d")}</p:spTree>`;
  const out = wrapGroups(xml, [{ id: "e_g", frame: { x: 144, y: 0, w: 288, h: 72 }, children: ["e_b", "e_c"] }]);
  const open = out.indexOf("<p:grpSp>");
  assert.ok(open > out.indexOf('name="e_a"') && open < out.indexOf('name="e_b"'));
  assert.ok(out.indexOf("</p:grpSp>") > out.indexOf('name="e_c"') && out.indexOf("</p:grpSp>") < out.indexOf('name="e_d"'));
  assert.match(out, /<p:cNvPr id="6" name="e_g"\/>/);
  assert.match(out, new RegExp(`<a:off x="914400" y="0"/><a:ext cx="${288 * EMU}" cy="457200"/><a:chOff x="914400" y="0"/><a:chExt cx="${288 * EMU}" cy="457200"/>`));
  // A child that drew nothing (a missing image) doesn't ungroup the rest.
  assert.ok(wrapGroups(xml, [{ id: "e_g", frame: { x: 0, y: 0, w: 1, h: 1 }, children: ["e_a", "e_b", "e_zz"] }]).includes("<p:grpSp>"));
  // Unchanged when there are no groups, or fewer than two of a group's shapes are there.
  assert.equal(wrapGroups(xml, []), xml);
  assert.equal(wrapGroups(xml, [{ id: "e_g", frame: { x: 0, y: 0, w: 1, h: 1 }, children: ["e_b", "e_zz"] }]), xml);
});

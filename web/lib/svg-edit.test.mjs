import test from "node:test";
import assert from "node:assert/strict";
import { addShape, bringForward, duplicate, findByPath, getText, pathOf, remove, sendBackward, setAttr, setText, translate } from "./svg-edit.ts";
import { parse, ser } from "./test-support/fake-dom.mjs";

const doc = (body) => parse(`<svg xmlns="http://www.w3.org/2000/svg">${body}</svg>`);

test("findByPath and pathOf round-trip", () => {
  const d = doc('<g><rect id="a"/><circle id="b"/></g>');
  assert.equal(findByPath(d, "").localName, "svg");
  assert.equal(findByPath(d, "0.1").getAttribute("id"), "b");
  assert.equal(findByPath(d, "0.5"), null);
  assert.equal(pathOf(d, findByPath(d, "0.1")), "0.1");
  assert.equal(pathOf(d, d.documentElement), "");
});

test("translate prepends, merges, rounds to 0.5 and cleans up", () => {
  const d = doc('<rect/><rect transform="rotate(10)"/><rect transform="translate(10, 5) scale(2)"/>');
  const [a, b, c] = d.documentElement.childNodes;
  translate(a, 10.3, -4.8);
  assert.equal(a.getAttribute("transform"), "translate(10.5 -5)");
  translate(b, 3, 4);
  assert.equal(b.getAttribute("transform"), "translate(3 4) rotate(10)");
  translate(c, -10, -5);
  assert.equal(c.getAttribute("transform"), "scale(2)");
  translate(a, 0.1, 0);
  assert.equal(a.getAttribute("transform"), "translate(10.5 -5)");
});

test("setText edits a text node or the first tspan", () => {
  const d = doc('<text>a</text><text>x<tspan>b</tspan><tspan>c</tspan></text>');
  const [t1, t2] = d.documentElement.childNodes;
  setText(t1, "hello");
  assert.equal(getText(t1), "hello");
  setText(t2, "new");
  assert.equal(ser(t2), "<text>x<tspan>new</tspan><tspan>c</tspan></text>");
});

test("setAttr sets and removes", () => {
  const d = doc("<rect/>");
  const r = d.documentElement.firstChild;
  setAttr(r, "fill", "#fff");
  assert.equal(r.getAttribute("fill"), "#fff");
  setAttr(r, "fill", "");
  assert.equal(r.getAttribute("fill"), null);
});

test("remove takes the element and its indentation; never the root", () => {
  const d = doc("\n  <rect/>\n  <circle/>\n");
  assert.equal(remove(d.documentElement.childNodes[3]), true);
  assert.equal(ser(d.documentElement), '<svg>\n  <rect/>\n</svg>');
  assert.equal(remove(d.documentElement), false);
});

test("duplicate inserts an offset copy after the original without ids", () => {
  const d = doc('<g><rect id="a" x="1"/></g>');
  const g = d.documentElement.firstChild;
  const copy = duplicate(g.firstChild);
  assert.equal(copy.getAttribute("id"), null);
  assert.equal(copy.getAttribute("transform"), "translate(16 16)");
  assert.equal(g.childNodes[1], copy);
});

test("bringForward / sendBackward reorder among element siblings", () => {
  const d = doc('<rect id="1"/><rect id="2"/><rect id="3"/>');
  const ids = () => d.documentElement.childNodes.map((n) => n.getAttribute("id")).join("");
  const first = d.documentElement.childNodes[0];
  assert.equal(bringForward(first), true);
  assert.equal(ids(), "213");
  assert.equal(bringForward(d.documentElement.childNodes[2]), false);
  assert.equal(sendBackward(d.documentElement.childNodes[2]), true);
  assert.equal(ids(), "231");
  assert.equal(sendBackward(d.documentElement.childNodes[0]), false);
});

test("addShape appends wireframe defaults centred on the point", () => {
  const d = doc("\n  <rect/>\n");
  const r = addShape(d, "rect", { x: 100, y: 100 });
  assert.equal(r.getAttribute("x"), "20");
  assert.equal(r.getAttribute("fill"), "#F3F4F6");
  assert.equal(r.getAttribute("stroke"), "#9CA3AF");
  const t = addShape(d, "text", { x: 50, y: 60 });
  assert.equal(t.getAttribute("font-size"), "16");
  assert.equal(getText(t), "Text");
  assert.equal(addShape(d, "line", { x: 0, y: 0 }).getAttribute("x1"), "-80");
  assert.equal(addShape(d, "ellipse", { x: 0, y: 0 }).getAttribute("rx"), "80");
  assert.equal(d.documentElement.lastChild.nodeValue, "\n"); // closing newline stays last
  assert.equal(pathOf(d, r), "3");
});

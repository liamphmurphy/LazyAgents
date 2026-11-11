import assert from "node:assert/strict";
import test from "node:test";
import { THEMES, dashArray, gradientLine, paintCss, resolvePaint } from "./slides/schema.ts";
import { gradFill, replaceFills } from "./slides/pptx-fills.ts";

test("gradients resolve theme stops, so a theme change recolours them", () => {
  const g = { type: "linear", angle: 45, stops: ["accent", "#00ff00"] };
  assert.deepEqual(resolvePaint(THEMES.light, g), { angle: 45, stops: ["#3B6FE0", "#00FF00"] });
  assert.deepEqual(resolvePaint(THEMES.dark, g), { angle: 45, stops: ["#8B9CFF", "#00FF00"] });
  assert.equal(resolvePaint(THEMES.light, "accent"), "#3B6FE0");
  assert.equal(paintCss({ angle: 0, stops: ["#000000", "#FFFFFF"] }), "linear-gradient(90deg, #000000, #FFFFFF)");
});

test("a gradient line spans the box like CSS and PowerPoint", () => {
  assert.deepEqual(gradientLine(0, 10, 20, 100, 50), { x1: 10, y1: 45, x2: 110, y2: 45 });
  const down = gradientLine(90, 0, 0, 100, 50);
  assert.ok(Math.abs(down.y1) < 1e-9 && Math.abs(down.y2 - 50) < 1e-9 && Math.abs(down.x1 - 50) < 1e-9);
  // 45° on a square: corner to corner.
  const d = gradientLine(45, 0, 0, 100, 100);
  assert.ok(Math.abs(d.x1) < 1e-9 && Math.abs(d.y1) < 1e-9 && Math.abs(d.x2 - 100) < 1e-9 && Math.abs(d.y2 - 100) < 1e-9);
});

test("dashes scale with the stroke width", () => {
  assert.deepEqual(dashArray(undefined, 4), []);
  assert.deepEqual(dashArray("solid", 4), []);
  assert.deepEqual(dashArray("dash", 4), [16, 12]);
  assert.deepEqual(dashArray("dashDot", 2), [8, 6, 2, 6]);
});

test("gradFill writes a two-stop a:lin in 60000ths of a degree, with alpha when translucent", () => {
  const x = gradFill({ angle: 90, stops: ["#112233", "#AABBCC"] }, 0.75);
  assert.match(x, /<a:gs pos="0"><a:srgbClr val="112233"><a:alpha val="75000"\/><\/a:srgbClr><\/a:gs>/);
  assert.match(x, /<a:gs pos="100000"><a:srgbClr val="AABBCC">/);
  assert.match(x, /<a:lin ang="5400000" scaled="0"\/>/);
  assert.doesNotMatch(gradFill({ angle: 0, stops: ["#000000", "#FFFFFF"] }), /alpha/);
});

test("replaceFills swaps only the named shape's fill and the background's", () => {
  const sp = (n) => `<p:sp><p:nvSpPr><p:cNvPr id="${n}" name="e_${n}"></p:cNvPr></p:nvSpPr><p:spPr><a:prstGeom prst="rect"/><a:solidFill><a:srgbClr val="00000${n}"/></a:solidFill><a:ln><a:solidFill><a:srgbClr val="FFFFFF"/></a:solidFill></a:ln></p:spPr></p:sp>`;
  const xml = `<p:bg><p:bgPr><a:solidFill><a:srgbClr val="FFFFFF"/></a:solidFill></p:bgPr></p:bg>${sp(1)}${sp(2)}`;
  const out = replaceFills(xml, [{ el: "e_2", xml: "<G2/>" }, { xml: "<BG/>" }]);
  assert.ok(out.includes("<p:bgPr><BG/></p:bgPr>"));
  assert.ok(out.includes('val="000001"'), "e_1 keeps its fill");
  assert.ok(out.includes('<a:prstGeom prst="rect"/><G2/><a:ln><a:solidFill><a:srgbClr val="FFFFFF"/>'), "e_2's fill, not its outline, is replaced");
  assert.throws(() => replaceFills(xml, [{ el: "e_9", xml: "<G/>" }]));
});

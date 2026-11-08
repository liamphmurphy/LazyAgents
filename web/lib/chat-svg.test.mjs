import assert from "node:assert/strict";
import test from "node:test";
import { isSvgFence, svgDataUrl } from "./chat-svg.ts";

test("isSvgFence matches only the svg-image tag", () => {
  assert.equal(isSvgFence("svg-image"), true);
  assert.equal(isSvgFence(" SVG-Image title=Mock"), true);
  assert.equal(isSvgFence("svg"), false);
  assert.equal(isSvgFence(undefined), false);
});

const decoded = (s) => decodeURIComponent(svgDataUrl(s).split(",").slice(1).join(","));

test("svgDataUrl is an encoded svg data URL", () => {
  assert.match(svgDataUrl("<svg/>"), /^data:image\/svg\+xml;charset=utf-8,%3Csvg/);
  assert.equal(svgDataUrl(`<svg xmlns="x"><text>#&"</text></svg>`).includes("#"), false);
});

test("svgDataUrl adds a missing namespace to the root only", () => {
  assert.equal(decoded("  <svg width='1'><svg/></svg>\n"), `<svg xmlns="http://www.w3.org/2000/svg" width='1'><svg/></svg>`);
  assert.equal(decoded(`<svg xmlns="http://www.w3.org/2000/svg"/>`), `<svg xmlns="http://www.w3.org/2000/svg"/>`);
});

import assert from "node:assert/strict";
import test from "node:test";
import { exportSvg, isStepsFence, isSvgFence, isThemedSvg, parseStepsFence, stepCount, stepFrame, svgDataUrl, svgTitle, themeSvg, VIZ_DARK, VIZ_LIGHT } from "./chat-svg.ts";

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

test("svgTitle reads the title for alt text", () => {
  assert.equal(svgTitle("<svg><title> A box </title></svg>"), "A box");
  assert.equal(svgTitle("<svg/>"), undefined);
});

test("themeSvg swaps palette tokens for each theme and leaves unknown ones", () => {
  const src = `<rect fill="var(--viz-panel)" stroke="var( --viz-2 )"/><rect fill="var(--viz-1-soft)"/><rect fill="var(--viz-nope)"/>`;
  assert.equal(isThemedSvg(src), true);
  assert.equal(isThemedSvg(`<rect fill="#fff"/>`), false);
  const dark = themeSvg(src, VIZ_DARK);
  assert.ok(dark.includes(`fill="${VIZ_DARK.panel}"`) && dark.includes(`stroke="${VIZ_DARK.series[1]}"`));
  assert.ok(dark.includes(`fill="${VIZ_DARK.series[0]}26"`));
  assert.ok(dark.includes("var(--viz-nope)"));
  assert.ok(themeSvg(src, VIZ_LIGHT).includes(`fill="${VIZ_LIGHT.panel}"`));
});

test("exportSvg bakes in the light palette and a background", () => {
  const out = exportSvg(`<svg viewBox="0 0 1 1"><rect fill="var(--viz-ink)"/></svg>`);
  assert.ok(out.includes(`xmlns="http://www.w3.org/2000/svg"`));
  assert.ok(out.includes(VIZ_LIGHT.ink) && !out.includes("--viz-"));
  assert.ok(out.includes(`<style>:root{background:${VIZ_LIGHT.surface}}</style>`));
  assert.equal(exportSvg(`<svg xmlns="x"><rect fill="#000"/></svg>`).includes("<style>"), false);
});

const STEPS = `<svg viewBox="0 0 9 9"><g data-step="1"/><g data-step="2-3"/><g data-step="wat"/></svg>\n1. First\n\n2) Second\nThird`;

test("isStepsFence matches only the svg-steps tag", () => {
  assert.equal(isStepsFence("SVG-Steps"), true);
  assert.equal(isStepsFence("svg-image"), false);
});

test("parseStepsFence splits the SVG from its captions", () => {
  const { svg, captions } = parseStepsFence(STEPS);
  assert.ok(svg.endsWith("</svg>"));
  assert.deepEqual(captions, ["First", "Second", "Third"]);
  assert.deepEqual(parseStepsFence("no svg").captions, []);
});

test("stepCount takes captions or the highest step, capped", () => {
  assert.equal(stepCount(`<g data-step="4"/>`, ["a"]), 4);
  assert.equal(stepCount(`<g/>`, ["a", "b"]), 2);
  assert.equal(stepCount(`<g/>`, []), 0);
  assert.equal(stepCount(`<g data-step="99"/>`, []), 12);
});

test("stepFrame hides what isn't in the step and only well-formed values", () => {
  const { svg } = parseStepsFence(STEPS);
  assert.ok(stepFrame(svg, 1).includes(`<style>[data-step="2-3"]{display:none}</style>`));
  assert.ok(stepFrame(svg, 2).includes("<style>") === false);
  assert.ok(stepFrame(svg, 4).includes(`[data-step="2-3"]{display:none}`));
  assert.equal(stepFrame(`<svg><g/></svg>`, 1), `<svg><g/></svg>`);
  assert.ok(stepFrame(`<svg title="a>b"><g data-step="2"/></svg>`, 1).startsWith(`<svg title="a>b"><style>`));
});

test("exportSvg preserves selected custom colors in a portable copy", () => {
  const palette = { ...VIZ_DARK, panel: "#123456", ink: "#abcdef" };
  const out = exportSvg('<svg><rect fill="var(--viz-panel)"/><text fill="var(--viz-ink)">Text</text></svg>', palette);
  assert.ok(out.includes('fill="#123456"') && out.includes('fill="#abcdef"'));
  assert.ok(!out.includes("--viz-"));
});

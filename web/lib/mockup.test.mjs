import test from "node:test";
import assert from "node:assert/strict";
import { blankWireframe, escapeXml, insertIndent, isSvgFile, svgError } from "./mockup.ts";

const fakeParser = (doc) => ({ parseFromString: () => doc });
const ok = { documentElement: { nodeName: "svg" }, getElementsByTagName: () => [] };

test("svgError flags empty, parser errors and non-svg roots", () => {
  assert.equal(svgError("  ", null), "The SVG is empty.");
  assert.equal(svgError("<svg/>", fakeParser(ok)), null);
  assert.equal(svgError("<svg/>", null), null);
  assert.match(svgError("<x/>", fakeParser({ ...ok, documentElement: { nodeName: "html" } })), /root element/);
  const bad = { ...ok, getElementsByTagName: () => [{ textContent: "This page contains the following errors:\nerror on line 1 at column 5\n" }] };
  assert.equal(svgError("<svg", fakeParser(bad)), "Not well-formed SVG: error on line 1 at column 5");
});

test("blankWireframe is 1280x800 and escapes the title", () => {
  const s = blankWireframe('A <b> & "c"');
  assert.match(s, /width="1280" height="800"/);
  assert.ok(s.includes("A &lt;b&gt; &amp; &quot;c&quot;"));
  assert.ok(!s.includes("<script"));
  assert.match(blankWireframe(""), /Untitled mockup/);
  assert.equal(escapeXml("<&>"), "&lt;&amp;&gt;");
});

test("isSvgFile and insertIndent", () => {
  assert.ok(isSvgFile("a.SVG", ""));
  assert.ok(isSvgFile("x", "image/svg+xml"));
  assert.ok(!isSvgFile("a.png", "image/png"));
  assert.deepEqual(insertIndent("ab", 1, 1), { value: "a  b", caret: 3 });
  assert.deepEqual(insertIndent("abcd", 1, 3), { value: "a  d", caret: 3 });
});

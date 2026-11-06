import test from "node:test";
import assert from "node:assert/strict";
import { sanitizeSvg } from "./svg-sanitize.ts";
import { parse } from "./test-support/fake-dom.mjs";

const NS = 'xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink"';
const run = (body, root = "svg") => sanitizeSvg(parse(`<${root} ${NS}>${body}</${root}>`).documentElement, "mk-1-");
const flat = (n, out = []) => (out.push(n), n.children.forEach((c) => flat(c, out)), out);

test("script, style, foreignObject and foreign namespaces are dropped and counted", () => {
  const { tree, skipped } = run(
    '<script>alert(1)</script><style>rect{fill:red}</style><foreignObject><div xmlns="http://www.w3.org/1999/xhtml"><b>x</b></div></foreignObject><h:p xmlns:h="http://www.w3.org/1999/xhtml"/><rect width="1"/>',
  );
  assert.deepEqual(tree.children.map((c) => c.tag), ["rect"]);
  assert.equal(skipped, 1 + 1 + 3 + 1);
});

test("a non-svg root is rejected", () => {
  assert.equal(sanitizeSvg(parse("<svg><rect/></svg>").documentElement, "p-").tree, null); // no namespace
});

test("event handlers, style attributes and unknown attributes are dropped", () => {
  const { tree } = run('<rect onclick="x()" onload="y()" style="fill:red" class="a" data-x="1" width="5" stroke-width="2"/>');
  assert.deepEqual(tree.children[0].attrs, { width: "5", strokeWidth: "2" });
});

test("javascript: values are dropped", () => {
  const { tree } = run('<rect fill="jav&#x61;script:1" stroke="java script:alert(1)" width="3"/>');
  assert.deepEqual(tree.children[0].attrs, { fill: "jav&#x61;script:1", width: "3" }); // not decoded by the fake parser; the spaced one is still caught
});

test("href: only #fragments and base64 raster data are kept", () => {
  const ok = "data:image/png;base64,AAAA";
  const { tree } = run(
    `<use xlink:href="#a"/><image href="https://evil.test/x.png"/><image xlink:href="${ok}"/><image href="data:image/svg+xml;base64,AAAA"/><use href="javascript:alert(1)"/>`,
  );
  assert.deepEqual(tree.children.map((c) => c.attrs.href), ["#mk-1-a", undefined, ok, undefined, undefined]);
});

test("url() values must be exactly url(#id)", () => {
  const { tree } = run('<rect fill="url(https://evil.test/a)"/><rect fill="url(#g)"/><rect stroke="url(#g) red"/><rect fill="url(data:image/png;base64,AA)"/>');
  assert.deepEqual(tree.children.map((c) => c.attrs.fill ?? c.attrs.stroke ?? null), [null, "url(#mk-1-g)", null, null]);
});

test("ids are prefixed and refs rewritten", () => {
  const { tree } = run(
    '<defs><linearGradient id="g"><stop offset="0" stop-color="#fff"/></linearGradient><clipPath id="c"/></defs><rect fill="url(#g)" clip-path="url(#c)" id="r"/><rect id="9bad id"/>',
  );
  const all = flat(tree);
  assert.equal(all.find((n) => n.tag === "linearGradient").attrs.id, "mk-1-g");
  const r = all.filter((n) => n.tag === "rect");
  assert.equal(r[0].attrs.fill, "url(#mk-1-g)");
  assert.equal(r[0].attrs.clipPath, "url(#mk-1-c)");
  assert.equal(r[0].attrs.id, "mk-1-r");
  assert.equal(r[1].attrs.id, undefined);
  assert.equal(all.find((n) => n.tag === "stop").attrs.stopColor, "#fff");
});

test("paths are child indices in the original doc, counting skipped nodes and whitespace", () => {
  const { tree } = run('<!--c--><g><rect/><script/><circle/></g><text>hi <tspan>x</tspan></text>');
  assert.equal(tree.path, "");
  const g = tree.children[0];
  assert.equal(g.path, "1");
  assert.deepEqual(g.children.map((c) => [c.tag, c.path]), [["rect", "1.0"], ["circle", "1.2"]]);
  const text = tree.children[1];
  assert.equal(text.path, "2");
  assert.deepEqual(text.children.map((c) => [c.tag, c.path, c.text]), [["#text", "2.0", "hi "], ["tspan", "2.1", undefined]]);
});

test("character data is only kept inside text-ish elements", () => {
  const { tree } = run("\n  <g>\n  </g>\n");
  assert.equal(tree.children.length, 1);
  assert.equal(tree.children[0].children.length, 0);
});

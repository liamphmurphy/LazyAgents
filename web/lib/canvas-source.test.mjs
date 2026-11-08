import assert from "node:assert/strict";
import test from "node:test";
import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import ReactMarkdown from "react-markdown";
import rehypeKatex from "rehype-katex";
import remarkGfm from "remark-gfm";
import remarkMath from "remark-math";
import { renderedToSource, rehypeSourceSpans, selectionToSource } from "./canvas-source.ts";

const render = (md) =>
  renderToStaticMarkup(
    createElement(ReactMarkdown, { remarkPlugins: [remarkGfm, [remarkMath, { singleDollarTextMath: false }]], rehypePlugins: [rehypeKatex, rehypeSourceSpans] }, md),
  );

const decode = (s) => s.replaceAll("&lt;", "<").replaceAll("&gt;", ">").replaceAll("&quot;", '"').replaceAll("&#x27;", "'").replaceAll("&amp;", "&");

/** Every wrapped run in the output, with its source offsets and rendered text. */
function runs(md) {
  return [...render(md).matchAll(/<span data-src-start="(\d+)" data-src-end="(\d+)">([^<]*)<\/span>/g)].map((m) => ({
    start: Number(m[1]),
    end: Number(m[2]),
    text: decode(m[3]),
  }));
}

test("runs carry offsets into the source through inline formatting", () => {
  const md = "Plain **bold** and `code` end.";
  assert.deepEqual(
    runs(md).map((r) => r.text),
    ["Plain ", "bold", " and ", "code", " end."],
  );
  for (const r of runs(md).filter((r) => r.text !== "code")) assert.equal(md.slice(r.start, r.end), r.text);
});

test("a selection across bold covers the markers in the source", () => {
  const md = "Plain **bold** end.";
  const [plain, bold] = runs(md);
  const range = selectionToSource(md, { ...plain, offset: 2 }, { ...bold, offset: 2 });
  assert.equal(md.slice(range.start, range.end), "ain **bo");
});

test("lists, headings and table cells map back", () => {
  const md = "# Title\n\n- one\n- two\n\n| a | b |\n|---|---|\n| cell | x |\n";
  for (const r of runs(md)) assert.equal(md.slice(r.start, r.end), r.text);
  assert.ok(runs(md).some((r) => r.text === "cell"));
});

test("escapes and character references keep later offsets right", () => {
  const md = String.raw`a \* b &amp; c &#x1F600; d`;
  const [run] = runs(md);
  assert.equal(run.text, "a * b & c \u{1F600} d");
  const map = renderedToSource(md.slice(run.start, run.end), run.text);
  assert.equal(md.slice(map[run.text.indexOf("b")], map[run.text.indexOf("b")] + 1), "b");
  assert.equal(md.slice(map[run.text.indexOf("c")], map[run.text.indexOf("c")] + 1), "c");
  assert.equal(md.slice(map[run.text.indexOf("d")], map[run.text.indexOf("d")] + 1), "d");
});

test("an unknown entity stays literal", () => {
  const src = "x &nope; y";
  const map = renderedToSource(src, src);
  assert.deepEqual(map, [...Array(src.length + 1).keys()]);
});

test("soft breaks skip blockquote prefixes", () => {
  const md = "> first line\n> second line";
  const [run] = runs(md);
  assert.equal(run.text, "first line\nsecond line");
  const i = run.text.indexOf("second");
  const at = run.start + renderedToSource(md.slice(run.start, run.end), run.text)[i];
  assert.equal(md.slice(at, at + 6), "second");
});

test("math and fenced code stay unwrapped and intact", () => {
  const html = render("Before\n\n$$x^2$$\n\n```js\nlet a\n```\n");
  assert.match(html, /katex/);
  assert.match(html, /<code class="language-js">let a\n<\/code>/);
  assert.deepEqual(
    runs("Before\n\n$$x^2$$\n\n```js\nlet a\n```\n").map((r) => r.text),
    ["Before"],
  );
});

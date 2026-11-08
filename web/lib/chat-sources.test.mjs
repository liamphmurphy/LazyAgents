import test from "node:test";
import assert from "node:assert/strict";
import { sourceSplit, splitSourceList, usedWebSources } from "./chat-sources.ts";

const urls = (r) => r.listed.map((l) => l.url);

test("a headed list of URLs leaves the message, in any common shape", () => {
  for (const tail of [
    "Sources:\n- https://a.com/x\n- https://b.com/y",
    "**Sources**\n\n- [A](https://a.com/x)\n- [B](https://b.com/y)",
    "## References\n1. A: https://a.com/x\n2. B — https://b.com/y",
    "---\n\nSources: https://a.com/x, https://b.com/y.",
  ]) {
    const r = splitSourceList(`The answer.\n\n${tail}\n`);
    assert.equal(r.body, "The answer.", tail);
    assert.deepEqual(urls(r), ["https://a.com/x", "https://b.com/y"], tail);
  }
});

test("titles come from link text or the words before the URL", () => {
  const r = splitSourceList("Hi.\n\nSources:\n- [Docs](https://a.com)\n- **Pricing**: https://b.com\n- https://c.com");
  assert.deepEqual(r.listed.map((l) => l.title), ["Docs", "Pricing", ""]);
});

test("anything but a clean headed list stays in the message", () => {
  for (const md of [
    "See https://a.com for more.",
    "Answer.\n\n- https://a.com\n- https://b.com",
    "Answer.\n\nSources:\n- https://a.com\nHope that helps!",
    "Answer.\n\nSources:\n- https://a.com\n- [[Eng/Plan]]",
    "Links are great: read more.",
    "```\nSources:\n- https://a.com\n```",
    "Sources:\n- https://a.com",
  ]) {
    const r = splitSourceList(md);
    assert.equal(r.body, md);
    assert.deepEqual(r.listed, []);
  }
});

const found = (...s) => ({ queries: [], fetched: [], running: false, sources: s });
const a = { title: "A title", url: "https://www.a.com/x/", host: "a.com" };
const b = { title: "B title", url: "https://b.com/y", host: "b.com" };
const c = { title: "C title", url: "https://c.com/z", host: "c.com" };

test("used pages are the listed ones and linked results; the rest are only found", () => {
  const r = usedWebSources("As [c says](https://c.com/z), and https://other.com too.", [{ title: "", url: "https://a.com/x" }], found(a, b, c));
  assert.deepEqual(r.used.map((s) => s.title), ["A title", "C title"]);
  assert.deepEqual(r.more, [b]);
});

test("a listed URL with no search result stands on its own", () => {
  const r = usedWebSources("x", [{ title: "", url: "https://www.a.com/x" }, { title: "Named", url: "https://b.com/y" }], found());
  assert.deepEqual(r.used.map((s) => [s.title, s.host]), [["a.com", "a.com"], ["Named", "b.com"]]);
});

test("the split names both origins only when both are present", () => {
  assert.equal(sourceSplit(3, 3), "3 in Vesper · 3 from the web");
  assert.equal(sourceSplit(2, 0), "in Vesper");
  assert.equal(sourceSplit(0, 2), "from the web");
});

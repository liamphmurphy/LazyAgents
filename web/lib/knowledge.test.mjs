import test from "node:test";
import assert from "node:assert/strict";
import { ancestors, buildTree, closeTab, monthGrid, openTab, outline, pageHref, slug, visibleProperties, wordCount } from "./knowledge.ts";

test("the explorer tree puts folders first, counts pages and lists daily notes newest first", () => {
  const t = buildTree([
    { id: "1", title: "Welcome", path: "Welcome" },
    { id: "2", title: "The old harbour", path: "Campaign/Locations/The old harbour" },
    { id: "3", title: "Watchtower", path: "Campaign/Locations/Watchtower" },
    { id: "4", title: "2026-11-12", path: "Daily/2026-11-12" },
    { id: "5", title: "2026-11-13", path: "Daily/2026-11-13" },
  ]);
  assert.deepEqual(t.folders.map((f) => f.name), ["Campaign", "Daily"]);
  assert.equal(t.count, 5);
  assert.equal(t.folders[0].count, 2);
  assert.deepEqual(t.folders[0].folders[0].pages.map((p) => p.title), ["The old harbour", "Watchtower"]);
  assert.deepEqual(t.folders[1].pages.map((p) => p.title), ["2026-11-13", "2026-11-12"]);
  assert.equal(t.folders[0].folders[0].path, "Campaign/Locations");
  assert.deepEqual(t.pages.map((p) => p.title), ["Welcome"]);
});

test("ancestors opens the folders leading to a page", () => {
  assert.deepEqual(ancestors("Campaign/Locations/The old harbour"), ["Campaign", "Campaign/Locations"]);
  assert.deepEqual(ancestors("Welcome"), []);
  assert.deepEqual(ancestors(null), []);
});

test("slugs match the server's page URLs", () => {
  assert.equal(slug("The old harbour"), "the-old-harbour");
  assert.equal(slug("Café [draft] #2"), "café-draft-2");
  assert.equal(pageHref("abc", "Harbour notes"), "/knowledge/abc/harbour-notes");
});

test("November 2026 starts on a Sunday, so the first Monday-first week has six blanks", () => {
  const g = monthGrid(2026, 10);
  assert.deepEqual(g[0], [null, null, null, null, null, null, 1]);
  assert.equal(g.flat().filter(Boolean).length, 30);
  assert.ok(g.every((w) => w.length === 7));
});

test("tabs open after the active one, refocus instead of duplicating, and close to a neighbour", () => {
  let tabs = openTab([], { id: "a", title: "A" });
  tabs = openTab(tabs, { id: "b", title: "B" }, "a");
  tabs = openTab(tabs, { id: "c", title: "C" }, "a");
  assert.deepEqual(tabs.map((t) => t.id), ["a", "c", "b"]);
  assert.equal(openTab(tabs, { id: "c", title: "C" }), tabs);
  assert.equal(openTab(tabs, { id: "c", title: "Renamed" })[1].title, "Renamed");
  const closed = closeTab(tabs, "c", "c");
  assert.deepEqual(closed.tabs.map((t) => t.id), ["a", "b"]);
  assert.equal(closed.next?.id, "b");
  assert.equal(closeTab(tabs, "a", "c").next, undefined);
});

test("outline skips headings inside code fences", () => {
  assert.deepEqual(outline("# Title\n\n```\n# not a heading\n```\n## Arrival ##\n"), [
    { level: 1, text: "Title", slug: "title" },
    { level: 2, text: "Arrival", slug: "arrival" },
  ]);
});

test("words and visible properties", () => {
  assert.equal(wordCount("The ferry's late.\n\n```\ncode here\n```\nTwice."), 4);
  assert.deepEqual(visibleProperties({ type: "location", query: { tag: "x" }, tags: [], zettel_id: "1", region: "Saltmere" }), [
    ["type", "location"],
    ["region", "Saltmere"],
  ]);
});

test("saving keeps wikilinks, embeds and callouts the editor escaped", async () => {
  const { restoreObsidianSyntax } = await import("./knowledge.ts");
  const { parseDoc, serializeDoc } = await import("./doc-markdown.ts");
  const md = "See [[Locations/Watchtower|the tower]] and [[Page#Arrival]] and [[Session 4#^dawn]].\n\n![[Maps/harbour.png]]\n\n> [!info] Secret\n> The bell.\n\n`[[not a link]]` stays code.";
  const written = serializeDoc(parseDoc(md));
  assert.match(written, /\\\[\\\[/, "the editor escapes brackets; if this fails the restore step may be unneeded");
  const restored = restoreObsidianSyntax(written);
  for (const s of ["[[Locations/Watchtower|the tower]]", "[[Page#Arrival]]", "[[Session 4#^dawn]]", "![[Maps/harbour.png]]", "> [!info] Secret"]) assert.ok(restored.includes(s), `${s} in ${restored}`);
  assert.ok(restored.includes("`[[not a link]]`"));
  assert.equal(restoreObsidianSyntax("a \\[b\\] c"), "a \\[b\\] c");
});

test("the graph layout keeps linked pages closer than unlinked ones, deterministically and fast", async () => {
  const { layoutGraph } = await import("./knowledge-graph.ts");
  const nodes = Array.from({ length: 1500 }, (_, i) => ({ id: `n${i}`, group: `g${i % 5}` }));
  const links = Array.from({ length: 1499 }, (_, i) => ({ source: `n${i}`, target: `n${(i * 7 + 1) % 1500}` }));
  const t0 = performance.now();
  const a = layoutGraph(nodes, links);
  assert.ok(performance.now() - t0 < 4000, "lays out 1,500 pages in a few seconds at most");
  assert.deepEqual(layoutGraph(nodes, links), a);
  const at = new Map(a.map((p) => [p.id, p]));
  const dist = (u, v) => Math.hypot(at.get(u).x - at.get(v).x, at.get(u).y - at.get(v).y);
  const linked = links.slice(0, 200).reduce((s, l) => s + dist(l.source, l.target), 0) / 200;
  const random = Array.from({ length: 200 }, (_, i) => dist(`n${i}`, `n${(i * 13 + 500) % 1500}`)).reduce((s, d) => s + d, 0) / 200;
  assert.ok(linked < random, `linked ${linked.toFixed(0)} < random ${random.toFixed(0)}`);
  assert.ok(a.every((p) => Number.isFinite(p.x) && Number.isFinite(p.y)));
});

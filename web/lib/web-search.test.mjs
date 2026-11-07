import test from "node:test";
import assert from "node:assert/strict";
import { hostOf, searchLinks, webSearches } from "./web-search.ts";

const at = (s) => `2026-11-07T10:00:${String(s).padStart(2, "0")}Z`;
const links = [{ title: "Go 1.24", url: "https://www.go.dev/blog" }, { title: "Dup", url: "https://a.com/x" }];
const result = `Web search results for query: "go"\n\nLinks: ${JSON.stringify(links)}\n\nSummary text.`;

test("web searches read queries, sources and fetched pages in the turn window", () => {
  const events = [
    { seq: 1, type: "tool_call", at: at(1), payload: { id: "old", tool: "WebSearch", input: { query: "old" } } },
    { seq: 2, type: "tool_result", at: at(2), payload: { id: "old", output: result } },
    { seq: 3, type: "tool_call", at: at(5), payload: { id: "a", tool: "WebSearch", input: { query: "go release" } } },
    { seq: 4, type: "tool_result", at: at(6), payload: { id: "a", output: result } },
    { seq: 5, type: "tool_call", at: at(7), payload: { id: "b", tool: "WebFetch", input: { url: "https://a.com/x", prompt: "p" } } },
    { seq: 6, type: "tool_call", at: at(8), payload: { id: "c", tool: "WebFetch", input: { url: "https://b.org/y" } } },
    { seq: 7, type: "tool_call", at: at(9), payload: { id: "d", tool: "web_search", input: { query: "codex q" } } },
  ];
  const r = webSearches(events, at(3), at(30));
  assert.deepEqual(r.queries, ["go release", "codex q"]);
  assert.deepEqual(r.fetched, ["https://a.com/x", "https://b.org/y"]);
  assert.deepEqual(r.sources.map((x) => [x.title, x.host]), [["Go 1.24", "go.dev"], ["Dup", "a.com"], ["b.org", "b.org"]]);
  assert.equal(r.running, false);
});

test("a search without a result is running; no events is empty", () => {
  const events = [{ seq: 1, type: "tool_call", at: at(5), payload: { id: "a", tool: "WebSearch", input: { query: "q" } } }];
  assert.equal(webSearches(events, at(1)).running, true);
  assert.equal(webSearches(events, at(5)).queries.length, 0);
  assert.deepEqual(webSearches(undefined), { queries: [], sources: [], fetched: [], running: false });
});

test("search links tolerate content blocks and bad JSON", () => {
  assert.equal(searchLinks([{ type: "text", text: result }]).length, 2);
  assert.deepEqual(searchLinks("Links: [oops"), []);
  assert.equal(hostOf("not a url"), "not a url");
});

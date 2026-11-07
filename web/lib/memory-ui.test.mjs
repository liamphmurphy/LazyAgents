import test from "node:test";
import assert from "node:assert/strict";
import { memoriesMarkdown, memoryNotices, parseTags, toolOutput } from "./memory-ui.ts";

const at = (s) => `2026-11-07T10:00:${String(s).padStart(2, "0")}Z`;

test("memory notices come from successful memory tool results in the turn", () => {
  const events = [
    { seq: 1, type: "tool_result", at: at(1), payload: { id: "old", tool: "mcp__lazyagents__save_memory", output: '{"id":"m0","action":"created","title":"Old"}' } },
    { seq: 2, type: "tool_call", at: at(5), payload: { id: "a", tool: "save_memory", input: {} } },
    { seq: 3, type: "tool_result", at: at(6), payload: { id: "a", output: '{"id":"m1","action":"created","title":"Works in PT"}' } },
    { seq: 4, type: "tool_result", at: at(7), payload: { id: "b", tool: "lazyagents.save_memory", output: JSON.stringify({ id: "m2", action: "updated", title: "New", previous: { id: "m2", title: "Was", summary: "s", type: "fact" } }) } },
    { seq: 5, type: "tool_result", at: at(8), payload: { id: "c", tool: "mcp__lazyagents__forget_memory", output: '{"forgotten":"m3","title":"Globex"}' } },
    { seq: 6, type: "tool_result", at: at(9), payload: { id: "d", tool: "mcp__lazyagents__save_memory", is_error: true, output: "secret refused" } },
    { seq: 7, type: "tool_result", at: at(10), payload: { id: "e", tool: "mcp__lazyagents__read_page", output: "{}" } },
    { seq: 8, type: "tool_result", at: at(11), payload: { id: "f", tool: "update_memory", output: '{"memory":{"id":"m4","title":"T"}}' } },
  ];
  assert.deepEqual(memoryNotices(events, at(2), at(30)), [
    { key: 3, kind: "saved", id: "m1", title: "Works in PT" },
    { key: 4, kind: "updated", id: "m2", title: "New", previous: { title: "Was", summary: "s", type: "fact" } },
    { key: 5, kind: "forgotten", id: "m3", title: "Globex" },
    { key: 8, kind: "updated", id: "m4", title: "T" },
  ]);
  assert.equal(memoryNotices(events, at(2), at(6)).length, 1);
  assert.equal(memoryNotices(undefined).length, 0);
});

test("tool output unwraps MCP content blocks and rejects non-JSON", () => {
  assert.deepEqual(toolOutput('[{"type":"text","text":"{\\"id\\":\\"x\\"}"}]'), { id: "x" });
  assert.deepEqual(toolOutput({ content: [{ type: "text", text: '{"id":"y"}' }] }), { id: "y" });
  assert.equal(toolOutput("Remembered it"), undefined);
});

test("tags and markdown export", () => {
  assert.deepEqual(parseTags("a, B ,a,,c"), ["a", "b", "c"]);
  const md = memoriesMarkdown(
    [{ id: "1", title: "T", summary: "S", body: "", tags: ["x"], type: "fact", source: "manual", created_at: "c", updated_at: "u", superseded_by: "2" }],
    "now",
  );
  assert.match(md, /## T \(retired\)/);
  assert.match(md, /- Tags: x/);
  assert.match(md, /- Superseded by: 2/);
});

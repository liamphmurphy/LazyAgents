import test from "node:test";
import assert from "node:assert/strict";
import { IMPORT_PROMPT, memoriesMarkdown, memoryNotices, parseMemoryImport, parseTags, toolOutput } from "./memory-ui.ts";

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

test("import prompt asks for the JSON shape the parser reads", () => {
  const r = parseMemoryImport(IMPORT_PROMPT);
  assert.equal(r.entries.length, 1);
  assert.equal(r.entries[0].title, "Prefers metric units");
});

test("import reads a fenced block with chatter around it", () => {
  const reply = 'Sure! Here you go:\n\n```json\n{"memories":[{"title":" Likes tea ","summary":"Drinks\\n  tea","type":"Preference","tags":["Drinks"]}]}\n```\nAnything else?';
  assert.deepEqual(parseMemoryImport(reply), {
    entries: [{ title: "Likes tea", summary: "Drinks tea", body: "", tags: ["drinks"], type: "preference" }],
    skipped: 0,
  });
});

test("import reads a bare array, and an unfenced object amid chatter", () => {
  assert.equal(parseMemoryImport('[{"title":"A"},{"title":"B"}]').entries.length, 2);
  assert.equal(parseMemoryImport('Here: {"memories":[{"title":"A"}]} done').entries.length, 1);
});

test("import takes our own export and skips retired entries", () => {
  const exp = { exported_at: "now", memories: [{ id: "1", title: "Keep", summary: "s", body: "b", tags: ["x"], type: "fact", source: "manual" }, { id: "2", title: "Old", superseded_by: "1" }, 5, { summary: "" }] };
  const r = parseMemoryImport(JSON.stringify(exp));
  assert.deepEqual(r.entries.map((e) => e.title), ["Keep"]);
  assert.equal(r.skipped, 3);
});

test("import normalises tags, types and lengths", () => {
  const long = "x".repeat(500);
  const r = parseMemoryImport(JSON.stringify([{ title: long, summary: long, body: "y".repeat(30000), tags: "#Foo, bar ,foo," + "z".repeat(41), type: "weird" }, { summary: "Only a summary" }]));
  assert.equal(r.entries[0].title.length, 200);
  assert.equal(r.entries[0].summary.length, 300);
  assert.equal(r.entries[0].body.length, 20000);
  assert.deepEqual(r.entries[0].tags, ["foo", "bar"]);
  assert.equal(r.entries[0].type, "other");
  assert.equal(r.entries[1].title, "Only a summary");
});

test("import errors: empty, garbage, no list, too many", () => {
  assert.deepEqual(parseMemoryImport("  \n"), { entries: [], skipped: 0 });
  assert.match(parseMemoryImport("I remember nothing").error, /Couldn't find JSON/);
  assert.match(parseMemoryImport('{"a":1}').error, /no memories list/);
  const many = JSON.stringify(Array.from({ length: 201 }, (_, i) => ({ title: `t${i}` })));
  const r = parseMemoryImport(many);
  assert.match(r.error, /at most 200/);
  assert.deepEqual(r.entries, []);
});

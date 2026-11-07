import test from "node:test";
import assert from "node:assert/strict";
import { diffLines } from "./diff.ts";
import { ancestors, buildTree, diffStats, foldDiff, numberLines, pageHeader, parseCompare } from "./wiki.ts";

test("page header: topic and tags are read and blanked", () => {
  const h = pageHeader("\nTopic: Architecture\nTags: runner, #Purpose, mcp, runner\n\n## When to use\n");
  assert.equal(h.topic, "Architecture");
  assert.deepEqual(h.tags, ["runner", "purpose", "mcp"]);
  assert.equal(h.body, "\n\n\n\n## When to use\n");
  const none = pageHeader("# Title\n\nTags: later lines don't count\n");
  assert.deepEqual(none, { topic: "", tags: [], body: "# Title\n\nTags: later lines don't count\n" });
});

test("tree: folders with recursive counts and folder pages", () => {
  const p = (path) => ({ id: path, path, title: path.split("/").pop() });
  const root = buildTree([p("Patterns/Go/Wrap errors"), p("Patterns/Go/Use pgx"), p("Patterns/API/Allowlist"), p("Payments"), p("Payments/Runbook"), p("Glossary")]);
  const patterns = root.folders.get("patterns");
  assert.equal(patterns.count, 3);
  assert.equal(patterns.folders.get("go").count, 2);
  assert.equal(root.folders.get("payments").page.path, "Payments");
  assert.equal(root.folders.get("payments").count, 2);
  assert.deepEqual(root.pages.map((x) => x.path), ["Glossary"]);
  assert.equal(root.count, 6);
  const mem = buildTree([p("agents/atlas/Memory"), p("agents/atlas/Notes/One")], "agents/");
  assert.equal(mem.folders.get("atlas").path, "agents/atlas");
  assert.equal(mem.folders.get("atlas").folders.get("notes").path, "agents/atlas/Notes");
  assert.equal(mem.folders.get("atlas").count, 2);
  assert.deepEqual(ancestors("Patterns/Go/Wrap errors"), ["patterns", "patterns/go"]);
});

test("diff: numbered, folded with 3 lines of context, counted", () => {
  const a = Array.from({ length: 20 }, (_, i) => `line ${i + 1}`).join("\n");
  const b = a.replace("line 2\n", "line two\n").replace("line 19", "line 19\nline 19b");
  const lines = numberLines(diffLines(a, b));
  assert.deepEqual(diffStats(lines), { added: 2, removed: 1 });
  const folded = foldDiff(lines);
  const skip = folded.find((l) => l.op === "skip");
  assert.equal(skip.count, 11); // lines 6-16: 3 lines of context after line 2 and before line 19b
  assert.equal(skip.lines[0].a, 6);
  const add = folded.find((l) => l.op === "add" && l.text === "line 19b");
  assert.equal(add.b, 20);
  assert.equal(add.a, undefined);
  assert.deepEqual(parseCompare("v2..v3"), ["v2", "v3"]);
  assert.deepEqual(parseCompare("v1.2..v1.3"), ["v1.2", "v1.3"]);
  assert.equal(parseCompare("v2"), null);
});

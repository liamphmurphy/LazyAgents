import test from "node:test";
import assert from "node:assert/strict";
import { changeTotals, filesStartOpen, langFromFence, langFromPath, parseHunkHeader, parsePatch } from "./code-diff.ts";

test("parseHunkHeader reads old and new starts, with or without counts", () => {
  assert.deepEqual(parseHunkHeader("@@ -10,3 +12,4 @@ func x()"), { oldStart: 10, newStart: 12 });
  assert.deepEqual(parseHunkHeader("@@ -1 +1 @@"), { oldStart: 1, newStart: 1 });
  assert.equal(parseHunkHeader("+@@ nope"), null);
});

test("parsePatch numbers lines across hunks", () => {
  const rows = parsePatch("@@ -1,3 +1,3 @@\n a\n-b\n+B\n c\n@@ -20,1 +20,2 @@\n x\n+y\n\\ No newline at end of file\n");
  assert.deepEqual(rows.map((r) => [r.kind, r.oldNo, r.newNo]), [
    ["hunk", undefined, undefined], ["ctx", 1, 1], ["del", 2, undefined], ["add", undefined, 2], ["ctx", 3, 3],
    ["hunk", undefined, undefined], ["ctx", 20, 20], ["add", undefined, 21], ["meta", undefined, undefined],
  ]);
  assert.equal(rows[2].text, "b");
});

test("changeTotals and filesStartOpen", () => {
  assert.deepEqual(changeTotals([{ additions: 2, deletions: 1 }, { additions: 3, deletions: 0 }]), { additions: 5, deletions: 1 });
  assert.deepEqual(changeTotals([]), { additions: 0, deletions: 0 });
  assert.equal(filesStartOpen(3), true);
  assert.equal(filesStartOpen(4), false);
});

test("language lookup by path and fence", () => {
  assert.equal(langFromPath("web/app/x.tsx"), "tsx");
  assert.equal(langFromPath("a/Dockerfile"), "docker");
  assert.equal(langFromPath("Makefile"), "make");
  assert.equal(langFromPath("notes.unknown"), null);
  assert.equal(langFromPath("LICENSE"), null);
  assert.equal(langFromFence("go"), "go");
  assert.equal(langFromFence("Bash title=x"), "shellscript");
  assert.equal(langFromFence(undefined), null);
});

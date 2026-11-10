import assert from "node:assert/strict";
import test from "node:test";
import { inferredIsStale, linesForRequest, mapApiPrompt, parseSpecSource } from "./playground.ts";

test("the gateway's spec source says which document the API tab got", () => {
  assert.deepEqual(parseSpecSource("served:/v3/api-docs"), { kind: "served", path: "/v3/api-docs" });
  assert.deepEqual(parseSpecSource("file:api/openapi.yaml"), { kind: "file", path: "api/openapi.yaml" });
  assert.deepEqual(parseSpecSource("inferred:.lazyagents/playground/openapi.json@9f17116"), { kind: "inferred", path: ".lazyagents/playground/openapi.json", commit: "9f17116" });
  assert.deepEqual(parseSpecSource("inferred:odd@path/x.json@abc"), { kind: "inferred", path: "odd@path/x.json", commit: "abc" });
  for (const h of [null, "", "weird", "inferred:"]) assert.deepEqual(parseSpecSource(h), { kind: "unknown" });
});

test("an inferred document is stale once the checkout moved past the commit it was read from", () => {
  assert.equal(inferredIsStale("9f17116", "9f17116a2b3c"), false, "a short hash of the same commit");
  assert.equal(inferredIsStale("9f17116a2b3c", "9f17116"), false);
  assert.equal(inferredIsStale("9f17116", "4eae576aaaa"), true);
  assert.equal(inferredIsStale("", "4eae576"), false, "unknown: don't claim it's stale");
  assert.equal(inferredIsStale("9f17116", ""), false);
});

test("a request's logs are the lines naming its id when there are any, else lines written while it ran", () => {
  const at = Date.parse("2026-11-10T22:00:00Z");
  const line = (ms, text) => ({ t: new Date(at + ms).toISOString(), source: "api", stream: "stdout", text });
  const lines = [line(-60_000, "earlier req=r-1 started"), line(10, "other request"), line(20, "req=r-1 failed: boom")];
  assert.deepEqual(linesForRequest(lines, "r-1", at, at + 50), { by: "id", lines: [lines[0], lines[2]] });
  assert.deepEqual(linesForRequest(lines, "r-2", at, at + 50), { by: "time", lines: [lines[1], lines[2]] });
  assert.deepEqual(linesForRequest(lines, "", at, at + 50).by, "time");
});

test("the ask to map the API says what to write and where", () => {
  const map = mapApiPrompt({ stale: false, existing: null });
  assert.match(map, /infer an OpenAPI document/i);
  assert.match(map, /x-source/);
  assert.match(map, /set_playground_openapi/);
  const again = mapApiPrompt({ stale: true, existing: ".lazyagents/playground/openapi.json" });
  assert.match(again, /\.lazyagents\/playground\/openapi\.json/);
  assert.match(again, /changed since/i);
});

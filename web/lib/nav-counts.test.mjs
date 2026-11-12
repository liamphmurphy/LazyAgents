import test from "node:test";
import assert from "node:assert/strict";
import { navCounts } from "./nav-counts.ts";

const ws = (name) => ({ id: name, name, updated_at: "2026-01-01T00:00:00Z" });
const allSet = { key_configured: true, secrets: [ws("harness:anthropic"), ws("harness:openai")] };

test("with nothing loaded every count is zero", () => {
  assert.deepEqual(navCounts({}), { inbox: 0, runs: 0, scheduled: 0, wiki: 0, settings: 0 });
});

test("the Needs you badge counts only items to act on, not every unread inbox item", () => {
  assert.equal(navCounts({ bootstrap: { counts: { inbox: 6, needs_you: 4 } } }).inbox, 4);
  assert.equal(navCounts({ bootstrap: { counts: { inbox: 6 } } }).inbox, 0);
});

test("runs comes from the runner's live count", () => {
  assert.equal(navCounts({ runner: { live: 3 } }).runs, 3);
  assert.equal(navCounts({ runner: {} }).runs, 0);
});

test("scheduled counts tasks whose latest run waits for you", () => {
  const scheduled = [
    { enabled: true, runs: [{ state: "waiting" }, { state: "done" }] },
    { enabled: true, runs: [{ state: "done" }, { state: "waiting" }] },
    { enabled: false, runs: [] },
  ];
  assert.equal(navCounts({ scheduled }).scheduled, 1);
});

test("wiki comes from the bootstrap's suggestion count", () => {
  assert.equal(navCounts({ bootstrap: { counts: { inbox: 0, wiki_suggestions: 2 } } }).wiki, 2);
  assert.equal(navCounts({ bootstrap: { counts: { inbox: 0 } } }).wiki, 0);
});

test("settings is 1 when secrets can't be stored or a required connection is missing", () => {
  assert.equal(navCounts({ secrets: allSet }).settings, 0);
  assert.equal(navCounts({ secrets: { ...allSet, key_configured: false } }).settings, 1);
  assert.equal(navCounts({ secrets: { key_configured: true, secrets: [ws("harness:anthropic")] } }).settings, 1);
});

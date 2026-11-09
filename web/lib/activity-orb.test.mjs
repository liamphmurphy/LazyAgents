import test from "node:test";
import assert from "node:assert/strict";
import { checkedAgo, chatStates, finishes, orbBadge, orbGlyph, orbLabel, orbQuiet, resetLabel, sessionPercent, usageSub } from "./activity-orb.ts";

const harness = (h, over = {}) => ({ harness: h, label: h === "codex" ? "Codex" : "Claude", running: 0, usage_available: false, windows: [], ...over });
const session = (used, resets_at) => ({ id: "five_hour", kind: "session", label: "Session", used_percent: used, resets_at });
const weekly = (used) => ({ id: "seven_day", kind: "weekly", label: "Weekly", used_percent: used });
const chat = (id, state, unread = 0) => ({ id, state, unread });
const entry = (pid, ...chats) => ({ persona: { id: pid }, state: chats[0]?.state ?? "working", unread: chats.reduce((n, c) => n + c.unread, 0), chats });

test("the glyph follows which harnesses are working", () => {
  assert.equal(orbGlyph([harness("claude-code"), harness("codex")]), "idle");
  assert.equal(orbGlyph([harness("claude-code", { running: 2 }), harness("codex")]), "claude");
  assert.equal(orbGlyph([harness("claude-code"), harness("codex", { running: 1 })]), "codex");
  assert.equal(orbGlyph([harness("claude-code", { running: 1 }), harness("codex", { running: 1 })]), "both");
  // A limited harness's runs have moved off it.
  assert.equal(orbGlyph([harness("claude-code", { running: 1 }), harness("codex", { running: 1, limited_until: "2026-11-09T21:20:00Z" })]), "claude");
});

test("rings show the session window, full while limited, nothing when unread", () => {
  assert.equal(sessionPercent(harness("claude-code", { usage_available: true, windows: [weekly(61), session(38.4)] })), 38);
  assert.equal(sessionPercent(harness("codex", { limited_until: "2026-11-09T21:20:00Z" })), 100);
  assert.equal(sessionPercent(harness("codex")), null);
  assert.equal(sessionPercent(undefined), null);
});

test("the badge prefers needs-you over unread", () => {
  assert.deepEqual(orbBadge([entry("a", chat("1", "replied", 2)), entry("b", chat("2", "needs_you"))]), { kind: "needs" });
  assert.deepEqual(orbBadge([entry("a", chat("1", "replied", 2)), entry("b", chat("2", "replied", 1))]), { kind: "unread", count: 3 });
  assert.equal(orbBadge([entry("a", chat("1", "working"))]), null);
});

test("quiet only when nothing is going on", () => {
  const idle = [harness("claude-code"), harness("codex")];
  assert.equal(orbQuiet(idle, []), true);
  assert.equal(orbQuiet(idle, [entry("a", chat("1", "working"))]), false);
  assert.equal(orbQuiet(idle, [entry("a", chat("1", "replied", 1))]), false);
  assert.equal(orbQuiet([harness("claude-code"), harness("codex", { limited_until: "2026-11-09T21:20:00Z" })], []), false);
});

test("finishes: working to replied, anything to needs-you, never on the first poll", () => {
  const before = chatStates([entry("a", chat("1", "working")), entry("b", chat("2", "working")), entry("c", chat("3", "needs_you"))]);
  const now = [entry("a", chat("1", "replied", 1)), entry("b", chat("2", "needs_you")), entry("c", chat("3", "needs_you")), entry("d", chat("4", "replied", 1)), entry("e", chat("5", "working"))];
  assert.deepEqual(finishes(before, now).map((f) => [f.chat.id, f.kind]), [["1", "done"], ["2", "needs"], ["4", "done"]]);
  assert.deepEqual(finishes(null, now), []);
  // A reply already announced stays quiet on the next poll.
  assert.deepEqual(finishes(chatStates(now), now), []);
});

test("usage lines", () => {
  const now = new Date("2026-11-09T18:00:00");
  const claude = harness("claude-code", { usage_available: true, usage_checked_at: "2026-11-09T17:59:20", windows: [session(38, "2026-11-09T21:20:00"), weekly(61)] });
  const codex = harness("codex", { limited_until: "2026-11-09T21:20:00" });
  assert.match(usageSub(claude, [claude, codex], now), /^5h · resets 9:20\s?PM · weekly 61%$/);
  assert.match(usageSub(codex, [claude, codex], now), /^Limited until 9:20\s?PM · falls back to Claude$/);
  assert.equal(usageSub(harness("codex"), [], now), "Not read yet");
  assert.equal(usageSub(harness("codex", { usage_checked_at: "2026-11-09T17:00:00" }), [], now), "No plan limits to show (API key or signed out)");
  assert.match(resetLabel("2026-11-12T08:00:00", now), /^Thu 8:00\s?AM$/);
  assert.equal(checkedAgo([claude, codex], now.getTime()), "checked 40s ago");
  assert.equal(checkedAgo([codex], now.getTime()), "not checked yet");
});

test("accessible name", () => {
  assert.equal(orbLabel([], []), "Activity: nothing running");
  assert.equal(orbLabel(["VesperBot", "Nora"], [entry("a", chat("1", "needs_you")), entry("b", chat("2", "replied", 2))]), "Activity: VesperBot, Nora working, 1 need you, 2 unread");
});

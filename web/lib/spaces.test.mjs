import { test } from "node:test";
import assert from "node:assert/strict";
import { bareUrl, formatBytes, formatTokens, sortSpaceChats, sourceDetail, sourceName, spaceHex, spaceInitial, spaceSubtitle, spaceUses } from "./spaces.ts";

test("tile letters and colors", () => {
  assert.equal(spaceInitial("health"), "H");
  assert.equal(spaceInitial("  2026 trip"), "2");
  assert.equal(spaceInitial("ñandú"), "Ñ");
  assert.equal(spaceInitial("!!"), "#");
  assert.equal(spaceHex("blue"), "#60A5FA");
  assert.equal(spaceHex("nope"), "#34D399");
});

test("token and size formatting", () => {
  assert.equal(formatTokens(950), "950");
  assert.equal(formatTokens(2100), "2.1k");
  assert.equal(formatTokens(13100), "13k");
  assert.equal(formatTokens(13100, true), "13.1k");
  assert.equal(formatTokens(40000), "40k");
  assert.equal(formatBytes(512), "512 B");
  assert.equal(formatBytes(340 * 1024), "340 KB");
  assert.equal(formatBytes(2.5 * 1024 * 1024), "2.5 MB");
});

test("source rows", () => {
  assert.equal(bareUrl("https://www.examine.com/creatine/"), "examine.com/creatine");
  assert.equal(bareUrl("not a url"), "not a url");
  const site = { kind: "website", title: "Creatine", url: "https://examine.com/creatine", tokens: 31000, size_bytes: 1 };
  assert.equal(sourceName(site), "examine.com/creatine");
  assert.equal(sourceDetail(site), "Website · 31k tokens");
  assert.equal(sourceDetail({ kind: "file", title: "a.md", tokens: 2100, size_bytes: 1 }), "File · 2.1k tokens");
  assert.equal(sourceDetail({ kind: "file", title: "scan.pdf", tokens: 0, size_bytes: 340 * 1024 }), "File · 340 KB");
  assert.equal(sourceName({ kind: "text", title: "Meal ideas", tokens: 1, size_bytes: 1 }), "Meal ideas");
});

test("space page text", () => {
  assert.equal(spaceSubtitle({ description: "Workouts, nutrition and recovery", chat_count: 12, source_count: 4 }), "Workouts, nutrition and recovery · 12 chats · 4 sources");
  assert.equal(spaceSubtitle({ description: "", chat_count: 1, source_count: 0 }), "1 chat · 0 sources");
  assert.equal(spaceUses({ name: "Health", instructions: "x", source_count: 4, space_memory: true }), "Uses Health’s instructions, 4 sources and space memory");
  assert.equal(spaceUses({ name: "Health", instructions: "", source_count: 1, space_memory: false }), "Uses 1 source");
  assert.equal(spaceUses({ name: "Health", instructions: "", source_count: 0, space_memory: false }), "New chats here start in Health.");
});

test("chat order", () => {
  const chats = [
    { id: "a", created_at: "2026-11-01T00:00:00Z", last_message_at: "2026-11-08T00:00:00Z" },
    { id: "b", created_at: "2026-11-05T00:00:00Z" },
    { id: "c", created_at: "2026-10-01T00:00:00Z", last_message_at: "2026-11-09T00:00:00Z" },
  ];
  assert.deepEqual(sortSpaceChats(chats, "newest").map((c) => c.id), ["c", "a", "b"]);
  assert.deepEqual(sortSpaceChats(chats, "oldest").map((c) => c.id), ["b", "a", "c"]);
});

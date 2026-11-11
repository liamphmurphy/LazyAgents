import assert from "node:assert/strict";
import test from "node:test";
import { asideCount, asidesInWindow, commandSendMode, cycleSendMode, SEND_MODES, sendModeAction, sendModePlaceholder } from "./send-mode.ts";

test("follow-up is the default, so Enter never interrupts unless asked to", () => {
  assert.equal(SEND_MODES[0], "followup");
});

test("Tab cycles the modes both ways and wraps", () => {
  assert.equal(cycleSendMode("followup"), "aside");
  assert.equal(cycleSendMode("aside"), "interrupt");
  assert.equal(cycleSendMode("interrupt"), "followup");
  assert.equal(cycleSendMode("followup", true), "interrupt");
  assert.equal(cycleSendMode("aside", true), "followup");
});

test("the Send button and placeholder say what each mode does", () => {
  assert.equal(sendModeAction("followup"), "Queue");
  assert.equal(sendModeAction("aside"), "Ask aside");
  assert.equal(sendModeAction("interrupt"), "Interrupt");
  assert.match(sendModePlaceholder("aside", "Hackerman"), /Hackerman keeps working/);
  assert.match(sendModePlaceholder("followup", "Hackerman"), /when Hackerman finishes/);
});

test("/aside and /queue pick their mode; other commands don't", () => {
  assert.equal(commandSendMode("aside"), "aside");
  assert.equal(commandSendMode("queue"), "followup");
  assert.equal(commandSendMode("stop"), null);
});

test("asides belong to the turn they were asked in", () => {
  const a = (id, created_at) => ({ id, created_at });
  const asides = [a("before", "2026-11-11T10:00:00Z"), a("first", "2026-11-11T10:05:00Z"), a("edge", "2026-11-11T10:10:00Z"), a("later", "2026-11-11T10:12:00Z")];
  assert.deepEqual(asidesInWindow(asides, "2026-11-11T10:01:00Z", "2026-11-11T10:10:00Z").map((x) => x.id), ["first"]);
  // The running turn has no end yet.
  assert.deepEqual(asidesInWindow(asides, "2026-11-11T10:10:00Z").map((x) => x.id), ["edge", "later"]);
  assert.deepEqual(asidesInWindow(asides, undefined), []);
  assert.deepEqual(asidesInWindow(undefined, "2026-11-11T10:00:00Z"), []);
});

test("asideCount", () => {
  assert.equal(asideCount(1), "1 aside");
  assert.equal(asideCount(3), "3 asides");
});

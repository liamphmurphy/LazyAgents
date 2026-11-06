import test from "node:test";
import assert from "node:assert/strict";
import { snippetParts } from "./snippet.ts";

test("snippet markers become match parts and never leak", () => {
  assert.deepEqual(snippetParts(""), []);
  assert.deepEqual(snippetParts("plain"), [{ text: "plain", match: false }]);
  assert.deepEqual(snippetParts("a «b» c «d»"), [
    { text: "a ", match: false },
    { text: "b", match: true },
    { text: " c ", match: false },
    { text: "d", match: true },
  ]);
  assert.deepEqual(snippetParts("stray « marker"), [{ text: "stray  marker", match: false }]);
});

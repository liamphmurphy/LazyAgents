import test from "node:test";
import assert from "node:assert/strict";
import { defaultLabel, linkify, parseWikiLinks } from "./wikilinks.ts";

test("note paths render as titles without changing their destinations", () => {
  const ref = parseWikiLinks("[[Campaign/Sessions/Session 27#Summary]]")[0];
  assert.equal(defaultLabel(ref), "Session 27 › Summary");
  assert.equal(defaultLabel({ ...ref, display: "Recap" }), "Recap");
  assert.equal(linkify("[[Campaign/Sessions/Session 27]]"), "[Session 27](wikilink:Campaign%2FSessions%2FSession%2027)");
  assert.equal(linkify("`[[Campaign/Note]]`\n```\n[[Campaign/Note]]\n```"), "`[[Campaign/Note]]`\n```\n[[Campaign/Note]]\n```");
});

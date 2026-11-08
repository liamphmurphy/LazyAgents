import assert from "node:assert/strict";
import test from "node:test";
import { canvasFenceIds, isCanvasFence, parseCanvasFence } from "./chat-canvas.ts";

const ID = "01a1bbdd-4937-7baa-af62-b3726025d331";

test("isCanvasFence matches only the canvas tag", () => {
  assert.equal(isCanvasFence("canvas"), true);
  assert.equal(isCanvasFence(" Canvas x=1"), true);
  assert.equal(isCanvasFence("canvases"), false);
  assert.equal(isCanvasFence(undefined), false);
});

test("parseCanvasFence takes one id", () => {
  assert.equal(parseCanvasFence(`\n${ID.toUpperCase()}\n`), ID);
  assert.equal(parseCanvasFence(`${ID}\n${ID}`), null);
  assert.equal(parseCanvasFence("not an id"), null);
});

test("canvasFenceIds finds each card in a message", () => {
  const other = "22222222-2222-4222-8222-222222222222";
  const md = `Done.\n\n\`\`\`canvas\n${ID.toUpperCase()}\n\`\`\`\n\nAnd:\n\n\`\`\`canvas\n${other}\n\`\`\`\n\n\`\`\`js\n${ID}\n\`\`\``;
  assert.deepEqual(canvasFenceIds(md), [ID, other]);
  assert.deepEqual(canvasFenceIds("no cards"), []);
});

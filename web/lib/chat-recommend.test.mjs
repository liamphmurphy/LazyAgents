import test from "node:test";
import assert from "node:assert/strict";
import { groupRecommendFences, isRecommendFence, parseRecommend, videoEmbedUrl } from "./chat-recommend.ts";

test("a recommend fence is a URL then a reason", () => {
  assert.ok(isRecommendFence("recommend"));
  assert.ok(!isRecommendFence("recommended"));
  assert.deepEqual(parseRecommend("https://a.com/x\nWorth\nit.\n"), { url: "https://a.com/x", note: "Worth it." });
  assert.deepEqual(parseRecommend("https://a.com"), { url: "https://a.com", note: "" });
  assert.equal(parseRecommend("javascript:alert(1)"), null);
  assert.equal(parseRecommend("not a link"), null);
});

test("only a well-formed video id becomes a player", () => {
  assert.equal(videoEmbedUrl("dQw4w9WgXcQ"), "https://www.youtube-nocookie.com/embed/dQw4w9WgXcQ?autoplay=1&rel=0");
  assert.equal(videoEmbedUrl("x\"><script>"), null);
  assert.equal(videoEmbedUrl(undefined), null);
});

const card = () => ({ type: "element", tagName: "pre", children: [{ type: "element", tagName: "code", properties: { className: ["language-recommend"] } }] });
const nl = { type: "text", value: "\n" };
const para = { type: "element", tagName: "p" };

test("neighbouring cards share a row; a lone card stays put", () => {
  const tree = { type: "root", children: [para, nl, card(), nl, card(), nl, para, nl, card()] };
  groupRecommendFences()(tree);
  assert.deepEqual(tree.children.map((c) => c.tagName ?? c.type), ["p", "text", "div", "text", "p", "text", "pre"]);
  assert.equal(tree.children[2].children.length, 2);
});

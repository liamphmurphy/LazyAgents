import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { removeNodes } from "./designs/edit.ts";
import { addLink, brokenLinks, follow, goBack, hotspotsOf, linksFrom, play, removeLink, restart, screens, setLink, startScreen, transitionFrames } from "./designs/prototype.ts";
import { normalize } from "./designs/schema.ts";

const read = (p) => readFileSync(new URL(p, import.meta.url), "utf8");
const acceptance = () => normalize(read("../../docs/design/fixtures/acceptance-design.json")).design;

test("links are added, edited and removed, and the body stays valid", () => {
  const d = acceptance();
  assert.deepEqual(screens(d).map((s) => s.name), ["Cart", "Shipping", "Payment", "Confirmation"]);
  // A new link goes to the frame after the hotspot's own.
  const added = addLink(d, "n_ship_btn", "k_new");
  assert.equal(added.design.links.at(-1).to, "n_pay");
  assert.equal(added.design.links.at(-1).trigger, "click");
  let e = setLink(added.design, "k_new", { trigger: "after-delay", to: "n_done", transition: "slide-up" });
  const l = e.links.find((x) => x.id === "k_new");
  assert.equal(l.delay_ms, 800, "after-delay gets a delay");
  assert.equal(l.to, "n_done");
  normalize(e);
  e = setLink(e, "k_new", { trigger: "hover", transition: "instant" });
  assert.equal(e.links.find((x) => x.id === "k_new").delay_ms, undefined);
  assert.equal(e.links.find((x) => x.id === "k_new").duration_ms, undefined);
  assert.equal(setLink(e, "k_new", { trigger: "hover" }), e, "no change keeps the object");
  assert.equal(linksFrom(e, "n_ship_btn").length, 2);
  // Saved in the body: normalising keeps it.
  assert.ok(normalize(e).design.links.some((x) => x.id === "k_new" && x.trigger === "hover"));
  const r = removeLink(e, "k_new");
  assert.equal(r.links.length, 3);
  assert.equal(removeLink(r, "k_new"), r);
});

test("a link to a removed frame is broken: flagged, valid, and skipped by Present", () => {
  const d = acceptance();
  const gone = { ...d, links: d.links.map((l) => (l.id === "k_pay_done" ? { ...l, to: "n_gone" } : l)) };
  assert.deepEqual(brokenLinks(gone).map((l) => l.id), ["k_pay_done"]);
  normalize(gone);
  assert.deepEqual(hotspotsOf(gone, "n_pay").map((l) => l.id), []);
  assert.deepEqual(hotspotsOf(gone, "n_cart").map((l) => l.id), ["k_cart_ship"]);
  // Removing a frame in the editor removes the links that touch it.
  assert.equal(removeNodes(d, new Set(["n_done"])).links.length, 2);
});

test("Present starts from a chosen frame, follows links, goes back and restarts", () => {
  const d = acceptance();
  assert.equal(startScreen(d, "n_ship").id, "n_ship");
  assert.equal(startScreen(d, "n_pay_btn").id, "n_pay", "a node inside a frame starts at its frame");
  assert.equal(startScreen(d, "nope", "p_flows").id, "n_cart");
  let p = play("n_ship");
  p = follow(p, hotspotsOf(d, "n_ship")[0].to);
  assert.equal(p.at, "n_pay");
  p = follow(p, "n_done");
  assert.deepEqual(p.back, ["n_ship", "n_pay"]);
  p = goBack(p);
  assert.equal(p.at, "n_pay");
  const first = goBack(p);
  assert.equal(first.at, "n_ship");
  assert.equal(goBack(first), first, "back stops at the start");
  assert.deepEqual(restart(p), play("n_ship"));
  assert.equal(transitionFrames("instant"), undefined);
  assert.equal(transitionFrames("slide-left").enter[0].transform, "translate(100%, 0%)");
});

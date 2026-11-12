import test from "node:test";
import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { readFileSync } from "node:fs";
import { createComponent, detach, instantiate, listComponents, overridable, setOverride } from "./designs/components.ts";
import { find, mapNode, subtreeIds } from "./designs/edit.ts";
import { inspect } from "./designs/inspect.ts";
import { makeLayout, pageOps } from "./designs/layout.ts";
import { normalize } from "./designs/schema.ts";
import { buildSnapshot } from "./designs/tokens-gen.ts";
import { DUSK, missingAfterSync, syncTokens, tokensChanged } from "./designs/tokens.ts";

const read = (p) => readFileSync(new URL(p, import.meta.url), "utf8");
const acceptance = () => normalize(read("../../docs/design/fixtures/acceptance-design.json")).design;
const texts = (d, pageIdx = 0) => pageOps(makeLayout(d).layoutPage(d.pages[pageIdx])).filter((o) => o.op === "text");
const opsOf = (d, el) => {
  const ids = subtreeIds(find(d, el).node);
  return pageOps(makeLayout(d).layoutPage(d.pages[0])).filter((o) => ids.has(o.el) || o.el?.startsWith(el + "/"));
};

// ---------- components ----------

test("listComponents counts each component's instances", () => {
  assert.deepEqual(listComponents(acceptance()), [{ id: "n_btn", name: "Button / Primary", page: "p_components", instances: 3 }]);
});

test("editing the main updates its instances, keeps their overrides and leaves detached ones alone", () => {
  let d = acceptance();
  d = detach(d, "n_ship_btn");
  assert.equal(find(d, "n_ship_btn").node.type, "frame");
  const before = (id) => opsOf(d, id);
  const shipBefore = JSON.stringify(before("n_ship_btn"));

  // The person recolours the main and renames its label's style: accent → state-done, size 15 → 17.
  d = mapNode(d, "n_btn", (n) => ({ ...n, fill: "$state-done" }));
  d = mapNode(d, "n_btn_label", (n) => ({ ...n, paragraphs: [{ runs: [{ ...n.paragraphs[0].runs[0], size: 17 }] }] }));

  const green = DUSK.modes.dark["state-done"].value;
  for (const [id, label] of [["n_cart_btn", "Checkout"], ["n_pay_btn", "Pay $84.00"]]) {
    const ops = opsOf(d, id);
    assert.ok(ops.some((o) => o.op === "rect" && o.fill === green), `${id} picks up the main's new fill`);
    const t = ops.filter((o) => o.op === "text");
    assert.equal(t.map((o) => o.text).join(" "), label, `${id} keeps its text override`);
    assert.ok(t.every((o) => o.size === 17), `${id} picks up the main's new text size`);
  }
  assert.equal(JSON.stringify(opsOf(d, "n_ship_btn")), shipBefore, "the detached copy doesn't change");
  assert.equal(opsOf(d, "n_ship_btn").find((o) => o.op === "text").text, "Continue", "detach applied the override");
});

test("create, instantiate, override (text, fill, visible) and clear", () => {
  let d = acceptance();
  d = createComponent(d, "n_pay_number", "Field");
  assert.equal(find(d, "n_pay_number").node.type, "component");
  assert.equal(createComponent(d, "n_pay_title"), null, "only frames become components");
  const made = instantiate(d, "n_pay_number", "n_done", 0, 0);
  d = made.design;
  assert.deepEqual(overridable(d, made.id).map((c) => c.id), ["n_pay_number_text"]);
  d = setOverride(d, made.id, "n_pay_number_text", { text: "5555 4444", fill: "$accent" });
  let inst = find(d, made.id).node;
  assert.deepEqual(inst.overrides, { n_pay_number_text: { text: "5555 4444", fill: "$accent" } });
  d = setOverride(d, made.id, "n_pay_number_text", { visible: false });
  assert.equal(find(d, made.id).node.overrides.n_pay_number_text.visible, false);
  d = setOverride(d, made.id, "n_pay_number_text", { text: undefined, fill: undefined, visible: undefined });
  inst = find(d, made.id).node;
  assert.equal(inst.overrides, undefined, "clearing the last override drops the map");
  // The design is still valid, so the server would take it.
  normalize(JSON.stringify(d));
});

// ---------- tokens ----------

test("tokens.generated.json matches styles/tokens.css (npm run gen:design-tokens)", () => {
  const css = read("../styles/tokens.css");
  const fresh = buildSnapshot(css, createHash("sha256").update(css).digest("hex"), read("../app/globals.css"));
  assert.deepEqual(JSON.parse(read("./designs/tokens.generated.json")), JSON.parse(JSON.stringify(fresh)));
  assert.equal(read("../../server/internal/design/tokens.generated.json"), read("./designs/tokens.generated.json"), "the Go copy is the same file");
  assert.equal(Object.keys(DUSK.modes.dark).length, 111);
  assert.equal(Object.keys(DUSK.modes.light).length, 83);
});

test("the snapshot only changes on sync; the badge shows while tokens.css differs and goes after sync", () => {
  const d = acceptance();
  assert.equal(tokensChanged(d), false, "the fixture is in sync");

  // tokens.css changes: accent is recoloured and text-2 is removed.
  const css = read("../styles/tokens.css").replace(/--accent:\s*#[0-9A-Fa-f]+;/, "--accent: #FF0000;").replace(/--text-2:[^;]+;/g, "");
  const next = buildSnapshot(css, createHash("sha256").update(css).digest("hex"), read("../app/globals.css"));
  assert.notEqual(next.hash, DUSK.hash);
  assert.equal(tokensChanged(d, next), true, "the badge shows");
  const oldAccent = d.token_sets["dusk-web"].modes.dark.accent.value;
  const accentFills = (x) => opsOf(x, "n_cart_btn").filter((o) => o.op === "rect").map((o) => o.fill);
  assert.deepEqual(accentFills(d), [oldAccent], "nothing recolours before the sync");

  assert.deepEqual(missingAfterSync(d, next).map((m) => [m.token, m.nodes]), [["text-2", ["n_ship_addr", "n_pay_note"]]]);
  const synced = syncTokens(d, "2026-11-13T00:00:00Z", { "text-2": "$text-secondary" }, next);
  assert.equal(tokensChanged(synced, next), false, "the badge goes after the sync");
  assert.deepEqual(synced.tokens_ref, { set: "dusk-web", source: "web/styles/tokens.css", hash: next.hash, synced_at: "2026-11-13T00:00:00Z" });
  assert.deepEqual(accentFills(synced), ["#FF0000"], "the sync recolours");
  assert.equal(find(synced, "n_ship_addr").node.paragraphs[0].runs[0].color, "$text-secondary", "a lost token takes the chosen replacement");
  normalize(JSON.stringify(synced));

  const kept = syncTokens(d, "2026-11-13T00:00:00Z", {}, next);
  assert.equal(find(kept, "n_ship_addr").node.paragraphs[0].runs[0].color, d.token_sets["dusk-web"].modes.dark["text-2"].value, "or keeps its literal colour");
  normalize(JSON.stringify(kept));
  assert.equal(tokensChanged(d, next), true, "the original design is untouched");
});

// ---------- inspect ----------

test("inspect writes the Payment frame as CSS with token variables", () => {
  const r = inspect(acceptance(), "n_pay", "css");
  assert.deepEqual(r.box, { x: 946, y: 0, w: 393, h: 852 });
  assert.deepEqual(r.spacing, { direction: "vertical", gap: 16, padding: [64, 24, 32, 24], align: "start", justify: "start" });
  for (const line of [
    ".payment {\n  width: 393px;\n  height: 852px;\n  display: flex;\n  flex-direction: column;\n  gap: 16px;\n  padding: 64px 24px 32px 24px;\n  background: var(--bg-panel);\n  overflow: hidden;\n}",
    ".payment__card-number {\n  align-self: stretch;\n  display: flex;\n  flex-direction: row;\n  gap: 8px;\n  padding: 12px 16px 12px 16px;\n  align-items: center;\n  background: var(--surface-raised);\n  border-width: 1px;\n  border-style: solid;\n  border-color: var(--border-strong);\n  border-radius: 7px;\n}",
    ".payment__expiry {\n  flex: 1 1 0;",
    ".payment__note .run-1-2 {\n  /* \"We never store\" */\n  font-weight: 600;\n  color: var(--text-2);\n}",
    "  object-fit: cover;",
  ])
    assert.ok(r.code.includes(line), `CSS has:\n${line}\n---\n${r.code}`);
  assert.deepEqual(r.assets, [{ id: "a_landscape", mime: "image/png", w: 1200, h: 800, alt: "Stylised mountains under a yellow sun" }]);
  const tok = Object.fromEntries(r.tokens.map((t) => [t.name, t.value]));
  assert.equal(tok["bg-panel"], DUSK.modes.dark["bg-panel"].value);
  assert.equal(tok["border-strong"], DUSK.modes.dark["border-strong"].value);
  assert.ok(r.tokens.find((t) => t.name === "accent").used_for.length > 0);
  const title = r.type.find((t) => t.text === "Payment");
  assert.deepEqual(title, { text: "Payment", size: 26, weight: 600, italic: false, line_height: 31.2, color: DUSK.modes.dark.text.value, token: "text" });
});

test("inspect writes Tailwind with the app's colour names, and React with components as props", () => {
  const d = acceptance();
  const tw = inspect(d, "n_pay", "tailwind").code;
  assert.ok(tw.startsWith('<div class="w-[393px] h-[852px] flex flex-col gap-4 pt-16 pr-6 pb-8 pl-6 bg-panel overflow-hidden">'), tw);
  assert.ok(tw.includes('<div class="self-stretch flex flex-row gap-2 px-4 py-3 items-center bg-surface-raised border border-border-strong rounded-[7px]">'));
  assert.ok(tw.includes('<span class="font-semibold text-fg-2">We never store</span>'));
  assert.ok(tw.includes(">Pay $84.00</p>"));

  const react = inspect(d, "n_pay", "react").code;
  assert.ok(react.startsWith("export function Payment({ assets }: { assets: Record<string, string> }) {"), react);
  assert.ok(react.includes('<ButtonPrimary label={"Pay $84.00"} className="self-stretch" />'), react);
  assert.ok(react.includes('src={assets.a_landscape} alt="Card artwork" />'));
  assert.ok(react.includes('export function ButtonPrimary({ label = "Button", className = "" }) {'));
  assert.ok(react.includes("bg-accent rounded-[10px] ${className}`}>"));
  assert.ok(react.includes(">{label}</p>"));

  // A light frame resolves its tokens' values for light.
  const light = mapNode(d, "n_pay", (n) => ({ ...n, mode: "light" }));
  const lr = inspect(light, "n_pay", "css");
  assert.equal(lr.mode, "light");
  assert.equal(lr.tokens.find((t) => t.name === "bg-panel").value, (DUSK.modes.light["bg-panel"] ?? DUSK.modes.dark["bg-panel"]).value);
  assert.throws(() => inspect(d, "n_nope", "css"), /isn't in the design/);
  assert.throws(() => inspect(d, "n_pay", "swift"), /target/);
});

test("React output stays valid for awkward override text, hidden layers and an inspected instance", () => {
  let d = setOverride(acceptance(), "n_pay_btn", "n_btn_label", { text: 'Say "hi"\nnow' });
  d = setOverride(d, "n_cart_btn", "n_btn_label", { visible: false });
  const pay = inspect(d, "n_pay", "react").code;
  assert.ok(pay.includes('<ButtonPrimary label={"Say \\"hi\\"\\nnow"} className="self-stretch" />'), pay);

  const cart = inspect(d, "n_cart", "react").code;
  assert.ok(cart.includes("<ButtonPrimary label={\"Checkout\"} showLabel={false} className=\"self-stretch\" />"), cart);
  assert.ok(cart.includes('export function ButtonPrimary({ label = "Button", showLabel = true, className = "" }) {'), cart);
  assert.ok(cart.includes("{showLabel && ("), cart);

  const inst = inspect(d, "n_pay_btn", "react").code;
  assert.equal(inst.match(/export function ButtonPrimary\(/g).length, 1, inst);
  assert.ok(!/return \(\s*<ButtonPrimary/.test(inst), "the component doesn't render itself");
  assert.ok(inst.includes('// <ButtonPrimary label={"Say \\"hi\\"\\nnow"}'), inst);
});

test("CSS comments can't be closed by run text", () => {
  const d = mapNode(acceptance(), "n_pay_note", (n) => ({ ...n, paragraphs: [{ runs: [n.paragraphs[0].runs[0], { ...n.paragraphs[0].runs[1], text: "a */ b" }] }] }));
  const css = inspect(d, "n_pay", "css").code;
  assert.ok(!css.includes("a */ b"), css);
});

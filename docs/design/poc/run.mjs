// Runs the Designs PoC and writes everything to evidence/.
//   npm ci && python3 -I fetch-fonts.py && node run.mjs
// Needs a Playwright Chromium (PLAYWRIGHT_BROWSERS_PATH or `npx playwright-core install chromium`).
import { chromium } from "playwright-core";
import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { buildMetrics, measurer } from "./metrics.mjs";
import { makeLayout } from "./layout.mjs";
import { largeDesign } from "./large.mjs";
import { importFlowchart, parseFlowchart, MermaidError } from "./mermaid.mjs";
import { panZoom } from "./panzoom.mjs";
import * as P from "./parity.mjs";
import { snapshot } from "./tokens.mjs";
import { CAPS, validate, walk } from "./validate.mjs";

const FX = "../fixtures";
const EV = "evidence";
for (const d of ["parity", "mermaid"]) mkdirSync(`${EV}/${d}`, { recursive: true });
const json = (path, v) => writeFileSync(path, JSON.stringify(v, null, 2) + "\n");
const report = { generated: new Date().toISOString() };

const raw = readFileSync(`${FX}/acceptance-design.json`, "utf8");
const design = JSON.parse(raw);
const mk = measurer(buildMetrics());

// 1. The fixture validates, and the invalid cases are refused.
const mutate = (f) => {
  const d = JSON.parse(raw);
  f(d);
  return validate(d);
};
const find = (d, id) => {
  let hit;
  walk(d, (n) => (n.id === id ? (hit = n) : 0));
  return hit;
};
const INVALID = {
  "unknown field on a frame": (d) => (find(d, "n_cart").bogus = 1),
  "unknown token": (d) => (find(d, "n_cart").fill = "$nope"),
  "duplicate id": (d) => (find(d, "n_ship").id = "n_cart"),
  "link to a node that isn't a top-level frame": (d) => (d.links[0].to = "n_cart_btn"),
  "image asset missing": (d) => (find(d, "n_pay_card").asset = "a_missing"),
  "connector endpoint on another page": (d) => (find(d, "n_c1").to.node = "n_cart"),
  "override outside the component": (d) => (find(d, "n_cart_btn").overrides = { n_cart: { text: "x" } }),
  "font on a run (Inter only)": (d) => (find(d, "n_st_cart").text.paragraphs[0].runs[0].font = "serif"),
  "children on a shape": (d) => (find(d, "n_st_cart").children = []),
  "fill sizing without auto layout": (d) => (find(d, "n_st_cart").sizing = { w: "fill", h: "fixed" }),
  "lower-case hex colour": (d) => (find(d, "n_cart").fill = "#abcdef"),
  "schema 2": (d) => (d.schema = 2),
};
const tokensNow = snapshot("../../../web/styles/tokens.css");
report.validation = {
  fixture: validate(design, raw),
  fixtureBytes: Buffer.byteLength(raw),
  invalidCases: Object.fromEntries(Object.entries(INVALID).map(([k, f]) => [k, mutate(f)[0] ?? "ACCEPTED (bug)"])),
  tokensInSync: tokensNow.hash === design.tokens_ref.hash,
};
const L = makeLayout(design, mk);

// 2. Parity: server PNG (resvg) vs editor (Chromium), 2x.
const browser = await chromium.launch();
report.chromium = browser.version();
const fonts = P.fontCss();
const assets = P.assetUris(design, FX);
report.parity = {};
for (const id of ["n_pay", "n_state"]) {
  const laid = L.layoutNode(id);
  const svg = P.toSvg(laid, { assets });
  writeFileSync(`${EV}/parity/${id}.svg`, P.toSvg(laid, { assets: { a_landscape: "../../../fixtures/landscape.png" } }));
  const server = P.serverPng(P.toSvg(laid, { assets, scale: P.SCALE }));
  const editor = await P.editorPng(browser, svg, laid.box.w, laid.box.h, fonts);
  writeFileSync(`${EV}/parity/${id}-server.png`, server);
  writeFileSync(`${EV}/parity/${id}-editor.png`, editor);
  const d = P.diff(editor, server, `${EV}/parity/${id}-diff.png`);
  // Counting anti-aliased edge pixels too, as a stricter upper bound.
  const strict = P.diff(editor, server, null, { includeAA: true });
  P.sideBySide([editor, server, readFileSync(`${EV}/parity/${id}-diff.png`)], `${EV}/parity/${id}-side-by-side.png`);
  report.parity[id] = { box: laid.box, ops: laid.ops.length, ...d, percent: +(d.ratio * 100).toFixed(4), withAntiAliasing: { differing: strict.differing, percent: +(strict.ratio * 100).toFixed(4) } };
}

// 3. Mermaid import.
let k = 0;
const ids = () => `n_m${String(++k).padStart(3, "0")}`;
const src = readFileSync(`${FX}/checkout.mmd`, "utf8");
const imported = await importFlowchart(src, { measure: mk.measure, ids });
const importDesign = { ...design, pages: [{ id: "p_import", name: "Import", background: "$bg", children: [imported.section] }], links: [] };
json(`${EV}/mermaid/checkout.design.json`, importDesign);
const LM = makeLayout(importDesign, mk);
writeFileSync(`${EV}/mermaid/checkout.png`, P.serverPng(P.toSvg(LM.layoutNode(imported.section.id), { scale: P.SCALE })));
const count = (t) => {
  let n = 0;
  walk(importDesign, (x) => (x.type === t ? n++ : 0));
  return n;
};
const refusals = {};
for (const bad of ["sequenceDiagram\n  A->>B: hi", "flowchart LR\n  A -->", "flowchart LR\n  A[open --> B", "flowchart LR\n  A ~~> B", "flowchart LR\n  subgraph x\n  A --> B", ""]) {
  try {
    parseFlowchart(bad);
    refusals[bad] = "ACCEPTED (bug)";
  } catch (e) {
    refusals[bad] = e instanceof MermaidError ? e.message : `CRASH ${e.message}`;
  }
}
report.mermaid = { input: "fixtures/checkout.mmd", shapes: count("shape"), sections: count("section"), connectors: count("connector"), valid: validate(importDesign), warnings: imported.warnings, refusals };

// 4. Pan/zoom with ~2k nodes.
const big = largeDesign(design);
const bigRaw = JSON.stringify(big);
let nodes = 0;
walk(big, () => nodes++);
const LB = makeLayout(big, mk);
const t0 = performance.now();
const page = LB.layoutPage("p_large");
const layoutMs = performance.now() - t0;
const inner = P.toSvg(page, {}).replace(/^<svg[^>]*>/, "").replace(/<\/svg>$/, "");
report.panzoom = {
  nodes,
  bodyBytes: Buffer.byteLength(bigRaw),
  bodyBytesPretty: Buffer.byteLength(JSON.stringify(big, null, 1)),
  valid: validate(big, bigRaw),
  layoutMs: +layoutMs.toFixed(1),
  ops: page.ops.length,
  viewport: "1440x900 @2x",
  attr: await panZoom(browser, inner, page.box, fonts, { strategy: "attr", background: page.background }),
  css: await panZoom(browser, inner, page.box, fonts, { strategy: "css", background: page.background }),
};
await browser.close();
report.caps = CAPS;

json(`${EV}/report.json`, report);
console.log(JSON.stringify(report, null, 2));

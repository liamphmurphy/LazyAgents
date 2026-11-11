// Builds a whole deck through the app's own rendering and export code
// (web/lib/slides), for docs/slides/acceptance/run.sh:
//   out/slide-NN.svg   what the editor, thumbnails and presentation draw (shown slides)
//   out/slide-NN.png   that SVG drawn by Chromium at 1920 × 1080 with the slide fonts
//   out/deck.pdf       the PDF export
//   out/deck.pptx      the PPTX export
//   out/slides.json    the shown slides' ids, in order
// Usage: node scripts/slides-deck-evidence.mjs <deck.json> <outdir> [--no-png]   (from web/)
import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { resolve } from "node:path";
import { pathToFileURL } from "node:url";
import { FIX, FONT_FILES, WEB, jiti } from "./slides-node-env.mjs";

const [deckPath, out = "out", flag] = process.argv.slice(2);
mkdirSync(out, { recursive: true });

const { deckToPdf, deckToPptx } = await jiti.import(WEB + "lib/slides/export.ts");
const { SlideSvg } = await jiti.import(WEB + "lib/slides/render.tsx");
const { renderToStaticMarkup } = await import("react-dom/server");
const { createElement } = await import("react");

const deck = JSON.parse(readFileSync(resolve(deckPath), "utf8"));
// Fixture assets name their file; the app serves them by id.
const file = (id) => deck.assets[id]?.file ?? id;
const exportUrl = (id) => file(id);
const svgUrl = (id) => pathToFileURL(FIX + file(id)).href;

writeFileSync(`${out}/deck.pdf`, Buffer.from(await (await deckToPdf(deck, exportUrl)).arrayBuffer()));
writeFileSync(`${out}/deck.pptx`, Buffer.from(await (await deckToPptx(deck, exportUrl)).arrayBuffer()));

const shown = deck.slides.filter((s) => !s.hidden);
const svgs = shown.map((slide, i) => {
  const svg = renderToStaticMarkup(createElement(SlideSvg, { deck, slide, assetUrl: svgUrl, fontsReady: true })).replace("<svg ", '<svg xmlns="http://www.w3.org/2000/svg" ');
  writeFileSync(`${out}/slide-${String(i + 1).padStart(2, "0")}.svg`, svg);
  return svg;
});
writeFileSync(`${out}/slides.json`, JSON.stringify(shown.map((s) => s.id)));

if (flag !== "--no-png") {
  const { chromium } = await import("@playwright/test");
  const browser = await chromium.launch();
  const page = await browser.newPage({ viewport: { width: 1920, height: 1080 } });
  const faces = FONT_FILES.map((f) => `@font-face{font-family:"${f.family}";font-weight:${f.weight};font-style:${f.style};src:url("${pathToFileURL(f.path).href}")}`).join("");
  for (const [i, svg] of svgs.entries()) {
    const html = `<!doctype html><style>${faces} html,body{margin:0} svg{display:block;width:1920px;height:1080px;font-kerning:none}</style>${svg}`;
    const htmlPath = resolve(out, "page.html");
    writeFileSync(htmlPath, html);
    await page.goto(pathToFileURL(htmlPath).href);
    await page.evaluate(() => document.fonts.ready);
    await page.evaluate(() => Promise.all([...document.images].map((im) => im.decode?.().catch(() => {}))));
    await page.screenshot({ path: `${out}/slide-${String(i + 1).padStart(2, "0")}.png` });
  }
  await browser.close();
}
console.log(`wrote ${out}: ${shown.length} slides`);

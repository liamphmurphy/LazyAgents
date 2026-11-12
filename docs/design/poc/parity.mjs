// Server PNG vs editor rendering, from one display list.
//   server: layout -> SVG string -> resvg (Inter TTFs passed in, no system fonts)
//   editor: the same SVG inline in Chromium with Inter loaded through @font-face
// Both at 2x. pixelmatch counts the pixels that differ beyond its anti-aliasing
// detection, and writes a diff image.
import { Resvg } from "@resvg/resvg-js";
import pixelmatch from "pixelmatch";
import { PNG } from "pngjs";
import { readFileSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { FACES, fontPath } from "./metrics.mjs";
import { toSvg } from "./svg.mjs";

export const SCALE = 2;

export function assetUris(design, dir) {
  const out = {};
  for (const [id, a] of Object.entries(design.assets)) out[id] = `data:${a.mime};base64,${readFileSync(join(dir, a.file)).toString("base64")}`;
  return out;
}

export function fontCss() {
  const face = (key, file) => {
    const italic = key.endsWith("i");
    const weight = parseInt(key, 10);
    return `@font-face{font-family:"Inter";font-style:${italic ? "italic" : "normal"};font-weight:${weight};src:url(data:font/ttf;base64,${readFileSync(fontPath(file)).toString("base64")}) format("truetype")}`;
  };
  return Object.entries(FACES).map(([k, f]) => face(k, f)).join("");
}

export function serverPng(svg) {
  const r = new Resvg(svg, {
    font: { fontFiles: Object.values(FACES).map(fontPath), loadSystemFonts: false, defaultFontFamily: "Inter" },
    shapeRendering: 2,
    textRendering: 2,
    imageRendering: 0,
  });
  return r.render().asPng();
}

export async function editorPng(browser, svg, w, h, fonts) {
  const page = await browser.newPage({ viewport: { width: Math.ceil(w), height: Math.ceil(h) }, deviceScaleFactor: SCALE });
  await page.setContent(`<!doctype html><html><head><style>${fonts}html,body{margin:0;background:transparent}svg{display:block}</style></head><body>${svg}</body></html>`);
  await page.evaluate(async () => {
    await Promise.all(["400", "500", "600", "700"].map((w) => document.fonts.load(`${w} 16px Inter`)));
    await document.fonts.load("italic 400 16px Inter");
    await document.fonts.ready;
    await Promise.all([...document.images].map((i) => i.decode?.().catch(() => {})));
  });
  const png = await page.locator("svg").first().screenshot({ omitBackground: true });
  await page.close();
  return png;
}

export function diff(aBuf, bBuf, outPath, { includeAA = false } = {}) {
  const a = PNG.sync.read(aBuf), b = PNG.sync.read(bBuf);
  if (a.width !== b.width || a.height !== b.height) throw new Error(`size differs: ${a.width}x${a.height} vs ${b.width}x${b.height}`);
  const out = new PNG({ width: a.width, height: a.height });
  const n = pixelmatch(a.data, b.data, out.data, a.width, a.height, { threshold: 0.1, includeAA });
  if (outPath) writeFileSync(outPath, PNG.sync.write(out));
  return { width: a.width, height: a.height, differing: n, ratio: n / (a.width * a.height) };
}

/** Same images side by side (editor | server | diff), for a human to look at. */
export function sideBySide(bufs, outPath) {
  const imgs = bufs.map((b) => PNG.sync.read(b));
  const gap = 16;
  const W = imgs.reduce((s, i) => s + i.width, 0) + gap * (imgs.length - 1);
  const H = Math.max(...imgs.map((i) => i.height));
  const out = new PNG({ width: W, height: H });
  out.data.fill(255);
  let x0 = 0;
  for (const img of imgs) {
    PNG.bitblt(img, out, 0, 0, img.width, img.height, x0, 0);
    x0 += img.width + gap;
  }
  writeFileSync(outPath, PNG.sync.write(out));
}

export { toSvg };

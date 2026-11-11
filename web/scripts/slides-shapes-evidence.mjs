// Builds the acceptance deck's preset-shapes slide through the app's own
// export code (web/lib/slides), for docs/slides/shapes/compare.sh:
//   out/shapes.svg            what the editor and presentation draw
//   out/shapes.pdf / .pptx    the PDF and PPTX exports
//   out/shapes-bare.*         the same shapes without text, for the pixel metric
// Usage: node scripts/slides-shapes-evidence.mjs <outdir>   (from web/)
import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { createJiti } from "jiti";

const jiti = createJiti(import.meta.url, { jsx: { runtime: "automatic" } });
const WEB = new URL("../", import.meta.url).pathname;
const FIX = new URL("../../docs/slides/fixtures/", import.meta.url).pathname;
const out = process.argv[2] ?? "out";
mkdirSync(out, { recursive: true });

// The exports fetch fonts from /fonts/slides; serve them from public/.
globalThis.fetch = async (url) => {
  const path = String(url).startsWith("/fonts/") ? WEB + "public" + url : FIX + String(url).split("/").pop();
  const b = readFileSync(path);
  return { ok: true, status: 200, arrayBuffer: async () => b.buffer.slice(b.byteOffset, b.byteOffset + b.byteLength) };
};

// The layout measures text on a canvas; give it one that reads advance widths
// from the slide fonts' own tables (no kerning, as in the app).
function advances(file) {
  const b = readFileSync(file), u16 = (o) => b.readUInt16BE(o), tables = {};
  for (let i = 0; i < u16(4); i++) tables[b.toString("latin1", 12 + i * 16, 16 + i * 16)] = b.readUInt32BE(20 + i * 16);
  const upm = u16(tables.head + 18), nh = u16(tables.hhea + 34), hmtx = tables.hmtx, cmap = tables.cmap;
  let sub;
  for (let i = 0; i < u16(cmap + 2); i++) if (u16(cmap + 4 + i * 8) === 3 && u16(cmap + 6 + i * 8) === 1) sub = cmap + b.readUInt32BE(cmap + 8 + i * 8);
  const segs = u16(sub + 6) / 2, ends = sub + 14, starts = ends + segs * 2 + 2, deltas = starts + segs * 2, ranges = deltas + segs * 2;
  const glyph = (c) => {
    for (let i = 0; i < segs; i++) {
      if (c > u16(ends + i * 2)) continue;
      if (c < u16(starts + i * 2)) return 0;
      const ro = u16(ranges + i * 2);
      if (!ro) return (c + u16(deltas + i * 2)) & 0xffff;
      const g = u16(ranges + i * 2 + ro + (c - u16(starts + i * 2)) * 2);
      return g ? (g + u16(deltas + i * 2)) & 0xffff : 0;
    }
    return 0;
  };
  return (text) => [...text].reduce((w, ch) => w + u16(hmtx + Math.min(glyph(ch.codePointAt(0)), nh - 1) * 4), 0) / upm;
}
const FACES = {};
for (const [fam, file] of [["Sans", "LiberationSans"], ["Serif", "LiberationSerif"], ["Mono", "LiberationMono"]])
  for (const [style, suffix] of [["400 normal", "Regular"], ["700 normal", "Bold"], ["400 italic", "Italic"], ["700 italic", "BoldItalic"]])
    FACES[`Vesper Slides ${fam}|${style}`] = advances(`${WEB}public/fonts/slides/${file}-${suffix}.ttf`);
globalThis.document = {
  createElement: () => ({
    getContext: () => {
      let face, px;
      return {
        set font(f) {
          const m = f.match(/^(italic )?(\d+) ([\d.]+)px "(.+)"$/);
          face = FACES[`${m[4]}|${m[2]} ${m[1] ? "italic" : "normal"}`];
          px = +m[3];
        },
        measureText: (t) => ({ width: face(t) * px }),
      };
    },
  }),
};

const { deckToPdf, deckToPptx } = await jiti.import(WEB + "lib/slides/export.ts");
const { SlideSvg } = await jiti.import(WEB + "lib/slides/render.tsx");
const { renderToStaticMarkup } = await import("react-dom/server");
const { createElement } = await import("react");

const full = JSON.parse(readFileSync(FIX + "acceptance-deck.json", "utf8"));
const slide = full.slides.find((s) => s.id === "s_preset_shapes");
const deck = { ...full, assets: {}, slides: [slide] };
const bare = { ...deck, slides: [{ ...slide, elements: slide.elements.filter((e) => e.type === "shape").map((e) => ({ ...e, text: undefined })) }] };

for (const [name, d] of [["shapes", deck], ["shapes-bare", bare]]) {
  writeFileSync(`${out}/${name}.pdf`, Buffer.from(await (await deckToPdf(d, (id) => id)).arrayBuffer()));
  writeFileSync(`${out}/${name}.pptx`, Buffer.from(await (await deckToPptx(d, (id) => id)).arrayBuffer()));
  const svg = renderToStaticMarkup(createElement(SlideSvg, { deck: d, slide: d.slides[0], assetUrl: (id) => id, fontsReady: true }));
  writeFileSync(`${out}/${name}.svg`, svg.replace("<svg ", '<svg xmlns="http://www.w3.org/2000/svg" '));
}
// Frames for the per-shape metric, in slide units.
writeFileSync(`${out}/frames.json`, JSON.stringify(bare.slides[0].elements.map((e) => ({ id: e.id, shape: e.shape, frame: e.frame, rotation: e.rotation ?? 0 }))));
console.log(`wrote ${out}`);

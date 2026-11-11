// Builds the acceptance deck's preset-shapes slide through the app's own
// export code (web/lib/slides), for docs/slides/shapes/compare.sh:
//   out/shapes.svg            what the editor and presentation draw
//   out/shapes.pdf / .pptx    the PDF and PPTX exports
//   out/shapes-bare.*         the same shapes without text, for the pixel metric
// Usage: node scripts/slides-shapes-evidence.mjs <outdir>   (from web/)
import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { FIX, WEB, jiti } from "./slides-node-env.mjs";

const out = process.argv[2] ?? "out";
mkdirSync(out, { recursive: true });

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

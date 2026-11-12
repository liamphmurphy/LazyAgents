// Inter metrics as plain tables, so text measures the same in the browser
// (editor) and in Node (server renderer) without a canvas or a font parser at
// run time. Production: a build step writes web/lib/designs/inter-metrics.json
// from the shipped TTFs, and a test fails when the TTFs and the table drift.
import opentype from "opentype.js";
import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
export const FACES = {
  "400": "Inter-Regular.ttf",
  "500": "Inter-Medium.ttf",
  "600": "Inter-SemiBold.ttf",
  "700": "Inter-Bold.ttf",
  "400i": "Inter-Italic.ttf",
};
export const fontPath = (f) => join(here, "fonts", f);

/** Covers Basic Latin, Latin-1, Latin Extended-A, general punctuation and currency. */
const RANGES = [[0x20, 0x7e], [0xa0, 0x17f], [0x2000, 0x206f], [0x20a0, 0x20bf], [0x2190, 0x21ff], [0x2022, 0x2022]];

export function buildMetrics() {
  const faces = {};
  for (const [key, file] of Object.entries(FACES)) {
    const font = opentype.parse(readFileSync(fontPath(file)).buffer);
    const upm = font.unitsPerEm;
    const adv = {};
    for (const [a, b] of RANGES) for (let cp = a; cp <= b; cp++) {
      const g = font.charToGlyph(String.fromCodePoint(cp));
      if (g && g.index !== 0) adv[cp] = Math.round((g.advanceWidth / upm) * 10000) / 10000;
    }
    faces[key] = {
      adv,
      missing: Math.round((font.glyphs.get(0).advanceWidth / upm) * 10000) / 10000,
      asc: font.tables.hhea.ascender / upm,
      desc: -font.tables.hhea.descender / upm,
      underline: { pos: -font.tables.post.underlinePosition / upm, thick: font.tables.post.underlineThickness / upm },
      strike: { pos: font.tables.os2.yStrikeoutPosition / upm, thick: font.tables.os2.yStrikeoutSize / upm },
    };
  }
  return faces;
}

/** measure(text, weight, italic, size) in design px. No kerning: renderers draw with kerning off. */
export function measurer(faces) {
  const face = (weight, italic) => faces[italic ? "400i" : String(weight)] ?? faces["400"];
  const measure = (text, weight, italic, size) => {
    const f = face(weight, italic);
    let w = 0;
    for (const ch of text) w += f.adv[ch.codePointAt(0)] ?? f.missing;
    return w * size;
  };
  return { measure, face };
}
